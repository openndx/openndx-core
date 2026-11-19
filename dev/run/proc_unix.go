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
// is left after 10 seconds. A terminal's Ctrl+C reaches only the foreground
// group, so this is also how the services stop on Ctrl+C.
func stop(cmd *exec.Cmd) {
	group := -cmd.Process.Pid
	_ = syscall.Kill(group, syscall.SIGINT)
	time.AfterFunc(10*time.Second, func() { _ = syscall.Kill(group, syscall.SIGKILL) })
}
