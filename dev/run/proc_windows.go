package main

import (
	"os/exec"
	"strconv"
)

// newProcessGroup does nothing on Windows: Ctrl+C reaches every process in
// the console, and stop ends the whole process tree.
func newProcessGroup(*exec.Cmd) {}

// stop ends the service's whole process tree; pnpm runs node in a child
// process.
func stop(cmd *exec.Cmd) {
	pid := strconv.Itoa(cmd.Process.Pid)
	_ = exec.Command("taskkill", "/T", "/F", "/PID", pid).Run() //nolint:gosec // pid of a process this program started
}
