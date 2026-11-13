package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nox1KCL/Agys/internal/session"
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

	if _, _, err := parseCLI([]string{"--plugin-state", "--action", "pin", "--id", "abc"}, &out, &errBuf); err == nil {
		t.Fatal("expected error when combining --plugin-state and --action")
	}
	if _, _, err := parseCLI([]string{"--action", "pin"}, &out, &errBuf); err == nil {
		t.Fatal("expected error when --action lacks --id")
	}
	if _, _, err := parseCLI([]string{"--id", "abc"}, &out, &errBuf); err == nil {
		t.Fatal("expected error when --id lacks --action")
	}
}

func TestRunPluginStateAndActions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))

	dataDir := filepath.Join(home, "antigravity-cli")
	stateFile := filepath.Join(home, "agys-state.json")
	sessionID := "33333333-3333-4333-8333-333333333333"
	logsDir := filepath.Join(dataDir, "brain", sessionID, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(`{"type":"USER_INPUT","content":"CLI headless plugin state test"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	var out, errBuf bytes.Buffer
	if err := run([]string{"--dir", dataDir, "--state", stateFile, "--plugin-state"}, &out, &errBuf); err != nil {
		t.Fatalf("--plugin-state failed: %v (stderr=%s)", err, errBuf.String())
	}

	var snap session.PluginSnapshot
	if err := json.Unmarshal(out.Bytes(), &snap); err != nil {
		t.Fatalf("decode PluginSnapshot failed: %v\nraw:\n%s", err, out.String())
	}
	if !snap.Ready || len(snap.Recent) != 1 || snap.Recent[0].ID != sessionID {
		t.Fatalf("unexpected initial snapshot: %+v", snap)
	}

	out.Reset()
	if err := run([]string{"--dir", dataDir, "--state", stateFile, "--action", "pin", "--id", sessionID}, &out, &errBuf); err != nil {
		t.Fatalf("--action pin failed: %v", err)
	}
	var actRes session.PluginActionResult
	if err := json.Unmarshal(out.Bytes(), &actRes); err != nil {
		t.Fatalf("decode pin result: %v", err)
	}
	if !actRes.OK || actRes.Pinned == nil || !*actRes.Pinned {
		t.Fatalf("unexpected pin result: %+v", actRes)
	}

	out.Reset()
	if err := run([]string{"--dir", dataDir, "--state", stateFile, "--action", "trash", "--id", sessionID}, &out, &errBuf); err != nil {
		t.Fatalf("--action trash failed: %v", err)
	}

	out.Reset()
	if err := run([]string{"--dir", dataDir, "--state", stateFile, "--plugin-state"}, &out, &errBuf); err != nil {
		t.Fatalf("--plugin-state after trash failed: %v", err)
	}
	if err := json.Unmarshal(out.Bytes(), &snap); err != nil {
		t.Fatalf("decode PluginSnapshot after trash failed: %v", err)
	}
	if len(snap.Recent) != 0 || len(snap.Pinned) != 0 || len(snap.Trash) != 1 {
		t.Fatalf("expected 1 trashed session after trash, got recent=%d pinned=%d trash=%d", len(snap.Recent), len(snap.Pinned), len(snap.Trash))
	}

	out.Reset()
	if err := run([]string{"--dir", dataDir, "--state", stateFile, "--action", "restore", "--id", snap.Trash[0].TrashName}, &out, &errBuf); err != nil {
		t.Fatalf("--action restore failed: %v", err)
	}
}
