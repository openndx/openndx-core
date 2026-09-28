package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
)

// Build metadata, injected at release time by GoReleaser via
// -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// buildVersion returns the version/commit/date for this binary. Values set via
// -ldflags win; otherwise it falls back to the module version and VCS info Go
// embeds itself, so `go install ...@v0.3.0` and `go build` in a checkout still
// report something useful instead of "dev".
func buildVersion() (v, c, d string) {
	v, c, d = version, commit, date
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, c, d
	}
	if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if c == "none" {
				c = s.Value
			}
		case "vcs.time":
			if d == "unknown" {
				d = s.Value
			}
		}
	}
	return v, c, d
}

func runVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: ondx version\n\nPrint the ondx version, commit, and build date.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return fmt.Errorf("version: unexpected arguments: %v", fs.Args())
	}

	v, c, d := buildVersion()
	fmt.Printf("ondx %s\ncommit: %s\nbuilt:  %s\ngo:     %s %s/%s\n", v, c, d, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return nil
}
