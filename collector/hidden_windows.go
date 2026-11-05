//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// Stops a console window flashing each time a system tool runs.
func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
