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

// stop sends SIGINT to the service's process group, then SIGKILL to whatever
// is left after 10 seconds. Signals from the terminal (Ctrl+C, or SIGHUP when
// it closes) reach only the runner's group, not the services, so this is how
// they stop in every case.
func stop(cmd *exec.Cmd, _ bool) {
	group := -cmd.Process.Pid
	_ = syscall.Kill(group, syscall.SIGINT)
	time.AfterFunc(10*time.Second, func() { _ = syscall.Kill(group, syscall.SIGKILL) })
}
