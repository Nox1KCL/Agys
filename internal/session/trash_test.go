package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestListRestoreAndPurgeTrashedSessions(t *testing.T) {
	home := t.TempDir()
	xdgData := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", xdgData)

	root := filepath.Join(home, "ag-data")
	cfg := config.FromRoot(root)

	id1 := "11111111-1111-4111-8111-111111111111"
	id2 := "22222222-2222-4222-8222-222222222222"

	for _, id := range []string{id1, id2} {
		dir := filepath.Join(cfg.BrainDir, id, ".system_generated", "logs")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir session: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "transcript.jsonl"), []byte(`{"type":"USER_INPUT","content":"Restore test session `+"\n"+`ignored"}`+"\n"), 0o600); err != nil {
			t.Fatalf("write transcript: %v", err)
		}
	}
	if err := os.MkdirAll(cfg.AnnotationsDir, 0o700); err != nil {
		t.Fatalf("mkdir annotations: %v", err)
	}
	ann1 := filepath.Join(cfg.AnnotationsDir, id1+".pbtxt")
	if err := os.WriteFile(ann1, []byte("last_user_view_time: 100\n"), 0o600); err != nil {
		t.Fatalf("write annotation: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect() failed: %v", err)
	}
	if _, err := DeleteSession(cfg, sessions, id1, DeleteModeTrash); err != nil {
		t.Fatalf("DeleteSession(id1) failed: %v", err)
	}
	if _, err := DeleteSession(cfg, sessions, id2, DeleteModeTrash); err != nil {
		t.Fatalf("DeleteSession(id2) failed: %v", err)
	}

	trashed, err := ListTrashedSessions(cfg)
	if err != nil {
		t.Fatalf("ListTrashedSessions() failed: %v", err)
	}
	if len(trashed) != 2 {
		t.Fatalf("expected 2 trashed sessions, got %d (%+v)", len(trashed), trashed)
	}

	restored, err := RestoreTrashedSession(cfg, id1)
	if err != nil {
		t.Fatalf("RestoreTrashedSession(id1) failed: %v", err)
	}
	if restored.ID != id1 {
		t.Fatalf("restored.ID = %q, want %q", restored.ID, id1)
	}
	if _, err := os.Stat(filepath.Join(cfg.BrainDir, id1)); err != nil {
		t.Fatalf("expected restored session in brain dir: %v", err)
	}
	if _, err := os.Stat(ann1); err != nil {
		t.Fatalf("expected restored companion annotation at %s: %v", ann1, err)
	}

	if _, err := PurgeTrashedSession(cfg, id2); err != nil {
		t.Fatalf("PurgeTrashedSession(id2) failed: %v", err)
	}
	remaining, err := ListTrashedSessions(cfg)
	if err != nil {
		t.Fatalf("ListTrashedSessions() after purge failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining trashed sessions, got %d (%+v)", len(remaining), remaining)
	}
}
