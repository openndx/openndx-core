//go:build !windows

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// newProcessGroup starts the service in a process group of its own, so stop
// reaches every process it starts: pnpm runs vite, which runs esbuild, and
// pnpm doesn't pass signals on to them.
func newProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// stop sends SIGINT to the service's process group. Signals from the terminal
// (Ctrl+C, or SIGHUP when it closes) reach only the runner's group, not the
// services, so this is how they stop in every case.
func stop(p *proc, _ bool) {
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGINT)
}

// reap waits until every service's process group is empty, then kills what
// is left at the deadline. The service itself may exit while a process it
// started ignores SIGINT; the runner must not exit before that is gone too.
func reap(procs []*proc, deadline time.Time) {
	for _, p := range procs {
		group := -p.cmd.Process.Pid
		for syscall.Kill(group, 0) == nil && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		_ = syscall.Kill(group, syscall.SIGKILL)
	}
}
