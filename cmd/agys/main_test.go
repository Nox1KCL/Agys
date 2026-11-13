package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseCLIHelpAndVersion(t *testing.T) {
	for _, flagArg := range []string{"-h", "--help"} {
		var out, errBuf bytes.Buffer
		_, done, err := parseCLI([]string{flagArg}, &out, &errBuf)
		if err != nil || !done {
			t.Fatalf("parseCLI(%q) got done=%v err=%v", flagArg, done, err)
		}
		if !strings.Contains(out.String(), "--exit-on-resume") || !strings.Contains(out.String(), "--dir") {
			t.Fatalf("expected usage flags in help output for %q, got:\n%s", flagArg, out.String())
		}
	}

	for _, flagArg := range []string{"-v", "--version"} {
		var out, errBuf bytes.Buffer
		_, done, err := parseCLI([]string{flagArg}, &out, &errBuf)
		if err != nil || !done {
			t.Fatalf("parseCLI(%q) got done=%v err=%v", flagArg, done, err)
		}
		if !strings.HasPrefix(out.String(), "agys ") {
			t.Fatalf("expected version prefix for %q, got %q", flagArg, out.String())
		}
	}
}

func TestParseCLIOptionsAndErrors(t *testing.T) {
	var out, errBuf bytes.Buffer
	opts, done, err := parseCLI([]string{
		"-d", "~/custom-ag",
		"--state", "./state.json",
		"-s", "synapsed",
		"-a",
		"-e",
	}, &out, &errBuf)
	if err != nil || done {
		t.Fatalf("unexpected parseCLI result: done=%v err=%v", done, err)
	}
	if opts.dataDir != "~/custom-ag" || opts.stateFile != "./state.json" {
		t.Fatalf("unexpected paths: %+v", opts)
	}
	if opts.searchQuery != "synapsed" || !opts.showArchived || !opts.exitOnResume {
		t.Fatalf("unexpected options: %+v", opts)
	}

	if _, _, err := parseCLI([]string{"extra-arg"}, &out, &errBuf); err == nil {
		t.Fatal("expected error for unexpected positional argument")
	}
}
