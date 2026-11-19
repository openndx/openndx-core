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
// for the grace period first so they can shut down cleanly.
func stop(p *proc, interrupted bool) {
	kill := func() {
		// Once the service has exited, Windows may reuse its PID, and
		// taskkill would end an unrelated process tree. Until then, Go holds
		// a handle to the process, so the PID still belongs to it.
		if p.exited.Load() {
			return
		}
		pid := strconv.Itoa(p.cmd.Process.Pid)
		_ = exec.Command("taskkill", "/T", "/F", "/PID", pid).Run() //nolint:gosec // pid of a process this program started
	}
	if interrupted {
		time.AfterFunc(gracePeriod, kill)
		return
	}
	kill()
}

// reap does nothing on Windows: the runner waits for each service to exit,
// and stop kills any that are still running when the grace period ends.
func reap([]*proc, time.Time) {}
