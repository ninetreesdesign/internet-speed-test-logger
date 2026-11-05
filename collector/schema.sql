-- Speed Monitor database, v1 2026-11-04. Built by: speedmon-collector db
-- CSVs in data/ and annotations.csv stay the source of truth; a database is rebuilt from them
-- and kept off Dropbox, since a synced live SQLite file can corrupt.
CREATE TABLE computer (computer_id TEXT PRIMARY KEY, name TEXT NOT NULL, os TEXT, notes TEXT);
CREATE TABLE network  (network_id INTEGER PRIMARY KEY, label TEXT NOT NULL UNIQUE, provider TEXT, notes TEXT);
CREATE TABLE tool     (tool_id TEXT PRIMARY KEY, name TEXT NOT NULL, method TEXT, comparable_group TEXT, notes TEXT);
CREATE TABLE speed_test (
  computer_id TEXT NOT NULL REFERENCES computer, epoch INTEGER NOT NULL,
  tool_id TEXT NOT NULL REFERENCES tool, network_id INTEGER REFERENCES network,
  status TEXT NOT NULL CHECK (status IN ('ok','fail','drop')),
  down_mbps REAL, up_mbps REAL, ping_ms REAL, server TEXT, iface TEXT, place TEXT, note TEXT,
  source_file TEXT NOT NULL, PRIMARY KEY (computer_id, epoch, tool_id));
CREATE TABLE connectivity_check (
  computer_id TEXT NOT NULL REFERENCES computer, epoch INTEGER NOT NULL,
  state TEXT NOT NULL, rtt_ms REAL, network_id INTEGER REFERENCES network, is_imported INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (computer_id, epoch));
CREATE TABLE annotation (
  annotation_id TEXT PRIMARY KEY, start_date TEXT NOT NULL, end_date TEXT, scope TEXT,
  kind TEXT, text TEXT NOT NULL, source TEXT, confidence TEXT);
-- Same pattern as other Data Tables: links live in one edges table.
CREATE TABLE edges (source_id TEXT NOT NULL, target_id TEXT NOT NULL, edge_type TEXT NOT NULL, notes TEXT);
CREATE INDEX speed_time ON speed_test(epoch);
CREATE INDEX check_time ON connectivity_check(epoch);

-- Median and low-end (10th percentile) speed, by month and by hour of day, within one tool.
CREATE VIEW speed_ok AS
  SELECT t.*, n.label AS network, strftime('%Y-%m', t.epoch, 'unixepoch', 'localtime') AS month,
         CAST(strftime('%H', t.epoch, 'unixepoch', 'localtime') AS INTEGER) AS hour,
         CAST(strftime('%w', t.epoch, 'unixepoch', 'localtime') AS INTEGER) AS weekday
  FROM speed_test t LEFT JOIN network n USING (network_id) WHERE t.status = 'ok';
CREATE VIEW speed_monthly AS
  WITH r AS (SELECT computer_id, network, tool_id, month, down_mbps, up_mbps,
    ROW_NUMBER() OVER (PARTITION BY computer_id, network, tool_id, month ORDER BY down_mbps) AS i,
    COUNT(*) OVER (PARTITION BY computer_id, network, tool_id, month) AS n FROM speed_ok)
  SELECT computer_id, network, tool_id, month, n AS tests,
    MAX(CASE WHEN i = (n + 1) / 2 THEN down_mbps END) AS down_median,
    MAX(CASE WHEN i = (n + 9) / 10 THEN down_mbps END) AS down_p10,
    ROUND(AVG(up_mbps), 1) AS up_mean
  FROM r GROUP BY computer_id, network, tool_id, month;
CREATE VIEW speed_by_hour AS
  WITH r AS (SELECT computer_id, network, tool_id, hour, down_mbps,
    ROW_NUMBER() OVER (PARTITION BY computer_id, network, tool_id, hour ORDER BY down_mbps) AS i,
    COUNT(*) OVER (PARTITION BY computer_id, network, tool_id, hour) AS n FROM speed_ok)
  SELECT computer_id, network, tool_id, hour, n AS tests, MAX(CASE WHEN i = (n + 1) / 2 THEN down_mbps END) AS down_median
  FROM r GROUP BY computer_id, network, tool_id, hour;
