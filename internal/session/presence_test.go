package session

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestDetectActiveSessions(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)
	if err := os.MkdirAll(cfg.PresenceDir, 0o700); err != nil {
		t.Fatalf("mkdir presence: %v", err)
	}

	activeID := "11111111-1111-4111-8111-111111111111"
	staleID := "22222222-2222-4222-8222-222222222222"

	activeLock := filepath.Join(cfg.PresenceDir, activeID+".lock")
	staleLock := filepath.Join(cfg.PresenceDir, staleID+".lock")

	holder, err := os.OpenFile(activeLock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create active lock: %v", err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("acquire exclusive flock: %v", err)
	}
	defer func() {
		_ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)
	}()

	if err := os.WriteFile(staleLock, nil, 0o600); err != nil {
		t.Fatalf("create stale lock: %v", err)
	}

	logDir := filepath.Join(cfg.BrainDir, activeID, ".system_generated", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "transcript.jsonl"), []byte(`{"type":"USER_INPUT","content":"hi"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	got := DetectActiveSessions(cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 active session, got %d (%+v)", len(got), got)
	}
	state, ok := got[activeID]
	if !ok || !state.Active || !state.Working {
		t.Fatalf("unexpected active state for %s: %+v (ok=%v)", activeID, state, ok)
	}
	if _, exists := got[staleID]; exists {
		t.Fatalf("stale unlocked file %s should not be marked active", staleID)
	}
}
