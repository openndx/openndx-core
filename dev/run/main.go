// Command run starts OpenNDX services natively for local development and
// prefixes each line of their output with the service name. `make dev` runs it
// from the repository root:
//
//	go run ./dev/run          # every service
//	go run ./dev/run oe pb    # some of them
//
// The Go services are built into bin/ and started from cmd/<svc>, so each
// reads cmd/<svc>/config.yaml. The consent portal runs `pnpm dev` and reads
// portals/apps/consent/public/config.js. Start their dependencies first with
// `make deps`. Ctrl+C, or any service exiting, stops all of them.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type service struct {
	name   string
	dir    string // working directory, relative to the repository root
	config string // file the service needs, relative to dir
	goPkg  string // package to build, or empty for the portal
}

var services = []service{
	{name: "pdp", dir: "cmd/pdp", config: "config.yaml", goPkg: "./cmd/pdp"},
	{name: "ce", dir: "cmd/ce", config: "config.yaml", goPkg: "./cmd/ce"},
	{name: "oe", dir: "cmd/oe", config: "config.yaml", goPkg: "./cmd/oe"},
	{name: "pb", dir: "cmd/pb", config: "config.yaml", goPkg: "./cmd/pb"},
	{name: "portal", dir: "portals/apps/consent", config: "public/config.js"},
}

// out serialises lines from every service so they don't interleave.
var out = struct {
	sync.Mutex
	w io.Writer
}{w: os.Stdout}

func logf(name, format string, args ...any) {
	out.Lock()
	defer out.Unlock()
	_, _ = fmt.Fprintf(out.w, "%-6s | "+format+"\n", append([]any{name}, args...)...)
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	selected, err := selectServices(args)
	if err != nil {
		logf("dev", "%v", err)
		return 2
	}
	if err := checkConfigs(selected); err != nil {
		logf("dev", "%v", err)
		return 1
	}
	if err := build(selected); err != nil {
		logf("dev", "%v", err)
		return 1
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	exited := make(chan string, len(selected))
	var running []*exec.Cmd
	var wg sync.WaitGroup
	for _, s := range selected {
		cmd, err := start(s, exited, &wg)
		if err != nil {
			logf(s.name, "failed to start: %v", err)
			stopAll(running)
			wg.Wait()
			return 1
		}
		running = append(running, cmd)
	}

	code := 0
	select {
	case <-signals:
		logf("dev", "stopping")
	case name := <-exited:
		logf("dev", "%s exited; stopping the others", name)
		code = 1
	}
	stopAll(running)
	wg.Wait()
	return code
}

func selectServices(names []string) ([]service, error) {
	if len(names) == 0 {
		return services, nil
	}
	var selected []service
	for _, name := range names {
		found := false
		for _, s := range services {
			if s.name == name {
				selected = append(selected, s)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown service %q; choose from pdp, ce, oe, pb, portal", name)
		}
	}
	return selected, nil
}

func checkConfigs(selected []service) error {
	var missing []string
	for _, s := range selected {
		path := filepath.Join(s.dir, s.config)
		if _, err := os.Stat(path); err != nil {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s; copy each from the example next to it (see the README in that directory)",
			strings.Join(missing, ", "))
	}
	return nil
}

func binPath(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join("bin", name)
}

func build(selected []service) error {
	for _, s := range selected {
		if s.goPkg == "" {
			continue
		}
		logf(s.name, "building")
		cmd := exec.Command("go", "build", "-o", binPath(s.name), s.goPkg) //nolint:gosec // package and path come from the services table
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", s.name, err)
		}
	}
	return nil
}

func start(s service, exited chan<- string, wg *sync.WaitGroup) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if s.goPkg != "" {
		bin, err := filepath.Abs(binPath(s.name))
		if err != nil {
			return nil, err
		}
		cmd = exec.Command(bin) //nolint:gosec // binary built above from the services table
	} else {
		cmd = exec.Command("pnpm", "dev")
	}
	cmd.Dir = s.dir

	r, w := io.Pipe()
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			logf(s.name, "%s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logf(s.name, "output no longer shown: %v", err)
			// Keep reading, or the service blocks writing to the pipe.
			_, _ = io.Copy(io.Discard, r)
		}
	}()
	go func() {
		defer wg.Done()
		err := cmd.Wait()
		_ = w.Close()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			logf(s.name, "exited")
		case errors.As(err, &exitErr):
			logf(s.name, "exited with code %d", exitErr.ExitCode())
		default:
			logf(s.name, "exited: %v", err)
		}
		exited <- s.name
	}()
	return cmd, nil
}

// stopAll stops every service. Ctrl+C in a terminal already reaches them, so
// this mostly matters when one service exits by itself. Stopping a service
// that has already exited fails harmlessly.
func stopAll(cmds []*exec.Cmd) {
	for _, cmd := range cmds {
		if runtime.GOOS == "windows" {
			// pnpm runs node in a child process; stop the whole tree.
			pid := strconv.Itoa(cmd.Process.Pid)
			_ = exec.Command("taskkill", "/T", "/F", "/PID", pid).Run() //nolint:gosec // pid of a process this program started
			continue
		}
		_ = cmd.Process.Signal(os.Interrupt)
		p := cmd.Process
		time.AfterFunc(10*time.Second, func() { _ = p.Kill() })
	}
}
