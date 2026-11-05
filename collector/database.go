// Local SQLite database for analysis. Always rebuilt from the CSVs, then swapped in whole, so
// it can be deleted at any time. It lives outside Dropbox: a synced live database can corrupt.
package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var SCHEMA_SQL string

var TOOL_GROUPS = map[string][2]string{
	"M-Lab":            {"NDT7, one connection", "mlab"},
	"Cloudflare":       {"HTTP goodput, repeated transfers", "cloudflare"},
	"Old Python tool":  {"Cloudflare HTTP from speedmon_bg.py", "old_python"},
	"Old browser page": {"Browser test, method varied by version", "old_browser"},
}

func databasePath() string { return filepath.Join(appDir(), "speedmon.db") }

func nullable(s string) any {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return nil
}

func buildDatabase(root, path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	os.Remove(tmp)
	defer os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.Exec(SCHEMA_SQL); err != nil {
		return "", fmt.Errorf("schema: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := loadAllComputers(tx, root); err != nil {
		return "", err
	}
	if err := loadAnnotations(tx, root); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if err := db.Close(); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

func networkID(tx *sql.Tx, ids map[string]int64, label string) (any, error) {
	if label == "" {
		return nil, nil
	}
	if id, ok := ids[label]; ok {
		return id, nil
	}
	res, err := tx.Exec(`INSERT INTO network(label) VALUES (?)`, label)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	ids[label] = id
	return id, err
}

func loadAllComputers(tx *sql.Tx, root string) error {
	dirs, err := os.ReadDir(filepath.Join(root, "data"))
	if err != nil {
		return err
	}
	nets, tools := map[string]int64{}, map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		computer := d.Name()
		dir := filepath.Join(root, "data", computer)
		if _, err := tx.Exec(`INSERT INTO computer(computer_id,name) VALUES (?,?)`, computer, strings.ReplaceAll(computer, "_", " ")); err != nil {
			return err
		}
		if err := loadSpeed(tx, computer, dir, nets, tools); err != nil {
			return err
		}
		if err := loadChecks(tx, computer, "legacy_checks.csv", 1, dir, nets); err != nil {
			return err
		}
		if err := loadChecks(tx, computer, "checks_*.csv", 0, dir, nets); err != nil {
			return err
		}
	}
	return nil
}

func loadSpeed(tx *sql.Tx, computer, dir string, nets map[string]int64, tools map[string]bool) error {
	files, _ := filepath.Glob(filepath.Join(dir, "*speed*.csv"))
	for _, file := range files {
		for _, r := range readRows(file, 10) {
			epoch, err := strconv.ParseFloat(r[0], 64)
			if err != nil {
				continue
			}
			tool := r[2]
			if tool == "" {
				tool = "Unknown tool"
			}
			if !tools[tool] {
				info := TOOL_GROUPS[tool]
				group := info[1]
				if group == "" {
					group = strings.ToLower(strings.ReplaceAll(tool, " ", "_"))
				}
				if _, err := tx.Exec(`INSERT INTO tool(tool_id,name,method,comparable_group) VALUES (?,?,?,?)`, tool, tool, info[0], group); err != nil {
					return err
				}
				tools[tool] = true
			}
			net, err := networkID(tx, nets, r[7])
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO speed_test(computer_id,epoch,tool_id,network_id,status,down_mbps,up_mbps,ping_ms,server,iface,note,source_file)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, computer, int64(epoch), tool, net, r[1], nullable(r[3]), nullable(r[4]), nullable(r[5]), r[6], r[8], r[9], filepath.Base(file)); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadChecks(tx *sql.Tx, computer, pattern string, imported int, dir string, nets map[string]int64) error {
	for _, r := range readRows(filepath.Join(dir, pattern), 4) {
		epoch, err := strconv.ParseFloat(r[0], 64)
		if err != nil {
			continue
		}
		net, err := networkID(tx, nets, r[3])
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO connectivity_check(computer_id,epoch,state,rtt_ms,network_id,is_imported) VALUES (?,?,?,?,?,?)`,
			computer, int64(epoch), r[1], nullable(r[2]), net, imported); err != nil {
			return err
		}
	}
	return nil
}

func loadAnnotations(tx *sql.Tx, root string) error {
	for _, a := range readCSVMaps(filepath.Join(root, "annotations.csv")) {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO annotation VALUES (?,?,?,?,?,?,?,?)`,
			a["id"], a["start"], a["end"], a["scope"], a["kind"], a["text"], a["source"], a["confidence"]); err != nil {
			return err
		}
	}
	return nil
}

// Read-only: the connection refuses writes, so a query cannot alter the database.
func runQuery(path, query string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no database yet; run: speedmon-collector db")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	fmt.Println(strings.Join(cols, "\t"))
	vals, ptrs := make([]any, len(cols)), make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		out := make([]string, len(cols))
		for i, v := range vals {
			if v != nil {
				out[i] = fmt.Sprint(v)
			}
		}
		fmt.Println(strings.Join(out, "\t"))
	}
	return rows.Err()
}

// /// EOF
