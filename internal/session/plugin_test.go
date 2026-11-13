package session

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestBuildPluginSnapshotAndActions(t *testing.T) {
	home := t.TempDir()
	xdgData := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", xdgData)

	cfg := config.FromRoot(filepath.Join(home, "ag-data"))
	now := time.Date(2026, 11, 13, 23, 0, 0, 0, time.UTC)

	ids := []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	}
	for i, id := range ids {
		logDir := filepath.Join(cfg.BrainDir, id, ".system_generated", "logs")
		if err := os.MkdirAll(logDir, 0o700); err != nil {
			t.Fatalf("mkdir session: %v", err)
		}
		payload := `{"type":"USER_INPUT","content":"Prompt for session ` + id[:4] + `"}` + "\n"
		if err := os.WriteFile(filepath.Join(logDir, "transcript.jsonl"), []byte(payload), 0o600); err != nil {
			t.Fatalf("write transcript: %v", err)
		}
		mod := now.Add(-time.Duration(i) * time.Hour)
		_ = os.Chtimes(filepath.Join(logDir, "transcript.jsonl"), mod, mod)
	}

	// Lock ids[0] as actively working.
	if err := os.MkdirAll(cfg.PresenceDir, 0o700); err != nil {
		t.Fatalf("mkdir presence: %v", err)
	}
	lockHolder, err := os.OpenFile(filepath.Join(cfg.PresenceDir, ids[0]+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer lockHolder.Close()
	if err := syscall.Flock(int(lockHolder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock: %v", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockHolder.Fd()), syscall.LOCK_UN)
	}()

	// Cached plugin quota usage.
	if err := os.MkdirAll(filepath.Dir(cfg.PluginUsageCacheFile), 0o700); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	quotaJSON := `{"groups":[{"name":"Gemini Models","buckets":[{"name":"Weekly","window":"weekly","remaining_fraction":0.14,"reset_time":"2026-11-14T16:24:00Z"}]}]}`
	if err := os.WriteFile(cfg.PluginUsageCacheFile, []byte(quotaJSON), 0o600); err != nil {
		t.Fatalf("write plugin cache: %v", err)
	}

	// Pin ids[0], archive ids[1], leave ids[2] in recent.
	if res, err := RunPluginAction(cfg, "pin", ids[0]); err != nil || res.Pinned == nil || !*res.Pinned {
		t.Fatalf("pin failed: res=%+v err=%v", res, err)
	}
	if res, err := RunPluginAction(cfg, "archive", ids[1]); err != nil || res.Archived == nil || !*res.Archived {
		t.Fatalf("archive failed: res=%+v err=%v", res, err)
	}

	snap, err := BuildPluginSnapshot(context.Background(), cfg, false, now)
	if err != nil {
		t.Fatalf("BuildPluginSnapshot() failed: %v", err)
	}
	if !snap.Ready || snap.ActiveCount != 1 || snap.ActiveStatus != "Working" {
		t.Fatalf("unexpected active snapshot metadata: %+v", snap)
	}
	if snap.Usage.MinRemainingPercent != 14 {
		t.Fatalf("MinRemainingPercent = %d, want 14", snap.Usage.MinRemainingPercent)
	}
	if len(snap.Pinned) != 1 || snap.Pinned[0].ID != ids[0] || !snap.Pinned[0].Working {
		t.Fatalf("unexpected pinned list: %+v", snap.Pinned)
	}
	if len(snap.Recent) != 1 || snap.Recent[0].ID != ids[2] {
		t.Fatalf("unexpected recent list: %+v", snap.Recent)
	}
	if len(snap.Archived) != 1 || snap.Archived[0].ID != ids[1] {
		t.Fatalf("unexpected archived list: %+v", snap.Archived)
	}

	// Move ids[2] to Trash, verify Trash list, then Restore and Purge.
	if _, err := RunPluginAction(cfg, "trash", ids[2]); err != nil {
		t.Fatalf("trash action failed: %v", err)
	}
	snap, err = BuildPluginSnapshot(context.Background(), cfg, false, now)
	if err != nil {
		t.Fatalf("BuildPluginSnapshot() after trash failed: %v", err)
	}
	if len(snap.Recent) != 0 || len(snap.Trash) != 1 || snap.Trash[0].ID != ids[2] {
		t.Fatalf("unexpected trash state: recent=%+v trash=%+v", snap.Recent, snap.Trash)
	}

	if _, err := RunPluginAction(cfg, "restore", ids[2]); err != nil {
		t.Fatalf("restore action failed: %v", err)
	}
	if _, err := RunPluginAction(cfg, "trash", ids[2]); err != nil {
		t.Fatalf("re-trash failed: %v", err)
	}
	if _, err := RunPluginAction(cfg, "purge", ids[2]); err != nil {
		t.Fatalf("purge action failed: %v", err)
	}
	snap, err = BuildPluginSnapshot(context.Background(), cfg, false, now)
	if err != nil {
		t.Fatalf("BuildPluginSnapshot() after purge failed: %v", err)
	}
	if len(snap.Trash) != 0 || len(snap.Recent) != 0 {
		t.Fatalf("expected empty trash & recent after purge, got trash=%+v recent=%+v", snap.Trash, snap.Recent)
	}
}
