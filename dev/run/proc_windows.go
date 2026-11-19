package main

import (
	"os/exec"
	"strconv"
	"time"
)

// newProcessGroup does nothing on Windows: Ctrl+C reaches every process in
// the console, and stop ends the whole process tree.
func newProcessGroup(*exec.Cmd) {}

// stop ends the service's whole process tree; pnpm runs node in a child
// process. After Ctrl+C, which every service has already received, it waits
// 10 seconds first so they can shut down cleanly.
func stop(cmd *exec.Cmd, interrupted bool) {
	pid := strconv.Itoa(cmd.Process.Pid)
	kill := func() {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", pid).Run() //nolint:gosec // pid of a process this program started
	}
	if interrupted {
		time.AfterFunc(10*time.Second, kill)
		return
	}
	kill()
}
