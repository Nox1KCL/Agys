package session

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestDeleteSessionMoveToTrashAndCollisionHandling(t *testing.T) {
	trashHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", trashHome)

	root := t.TempDir()
	cfg := config.FromRoot(root)

	idKeep := "11111111-1111-4111-8111-111111111111"
	idTrash := "22222222-2222-4222-8222-222222222222"

	for _, id := range []string{idKeep, idTrash} {
		logDir := filepath.Join(cfg.BrainDir, id, ".system_generated", "logs")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", logDir, err)
		}
		if err := os.WriteFile(filepath.Join(cfg.BrainDir, id, "task.md"), []byte("# Session "+id+"\n"), 0o600); err != nil {
			t.Fatalf("write task.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(logDir, "transcript.jsonl"), []byte(`{"step_index":0,"type":"USER_INPUT","content":"hello"}`+"\n"), 0o600); err != nil {
			t.Fatalf("write transcript: %v", err)
		}
	}
	if err := os.MkdirAll(cfg.AnnotationsDir, 0o755); err != nil {
		t.Fatalf("mkdir annotations: %v", err)
	}
	annPath := filepath.Join(cfg.AnnotationsDir, idTrash+".pbtxt")
	if err := os.WriteFile(annPath, []byte(`title:"Session To Trash"`+"\n"), 0o600); err != nil {
		t.Fatalf("write annotation: %v", err)
	}

	// Build initial gob index cache.
	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 initial sessions, got %d", len(sessions))
	}

	// Pre-create a conflicting entry in Trash to verify collision suffix allocation (.2).
	trashFiles := filepath.Join(trashHome, "Trash", "files")
	trashInfo := filepath.Join(trashHome, "Trash", "info")
	if err := os.MkdirAll(trashFiles, 0o700); err != nil {
		t.Fatalf("mkdir trashFiles: %v", err)
	}
	if err := os.MkdirAll(trashInfo, 0o700); err != nil {
		t.Fatalf("mkdir trashInfo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(trashFiles, idTrash), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write pre-existing trash file: %v", err)
	}

	res, err := DeleteSession(cfg, sessions, idTrash, DeleteModeTrash)
	if err != nil {
		t.Fatalf("DeleteSession(DeleteModeTrash) failed: %v", err)
	}
	if len(res.DeletedIDs) != 1 || res.DeletedIDs[0] != idTrash {
		t.Fatalf("unexpected DeletedIDs: %+v", res.DeletedIDs)
	}
	if res.TrashDir != filepath.Join(trashHome, "Trash") {
		t.Fatalf("unexpected TrashDir: %q", res.TrashDir)
	}

	if _, err := os.Stat(filepath.Join(cfg.BrainDir, idTrash)); !os.IsNotExist(err) {
		t.Fatalf("expected trashed session dir removed from brain, stat err=%v", err)
	}
	if _, err := os.Stat(annPath); !os.IsNotExist(err) {
		t.Fatalf("expected trashed annotation removed from annotations, stat err=%v", err)
	}

	trashedDir := filepath.Join(trashFiles, idTrash+".2")
	if _, err := os.Stat(filepath.Join(trashedDir, "task.md")); err != nil {
		t.Fatalf("expected trashed session files at %q: %v", trashedDir, err)
	}
	infoBytes, err := os.ReadFile(filepath.Join(trashInfo, idTrash+".2.trashinfo"))
	if err != nil {
		t.Fatalf("expected .trashinfo for trashed dir: %v", err)
	}
	infoText := string(infoBytes)
	wantPathLine := "Path=" + encodeTrashPath(filepath.Join(cfg.BrainDir, idTrash))
	if !strings.Contains(infoText, "[Trash Info]") || !strings.Contains(infoText, wantPathLine) || !strings.Contains(infoText, "DeletionDate=") {
		t.Fatalf("unexpected .trashinfo payload:\n%s", infoText)
	}

	if _, err := os.Stat(filepath.Join(trashFiles, idTrash+".pbtxt")); err != nil {
		t.Fatalf("expected trashed annotation in Trash/files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(trashInfo, idTrash+".pbtxt.trashinfo")); err != nil {
		t.Fatalf("expected .trashinfo for annotation: %v", err)
	}

	// Cache and Collect() must reflect removal while keeping untouched sessions.
	if cached, ok := loadIndexCache(cfg.CacheFile, cfg.AntigravityDir); !ok {
		t.Fatal("expected cache to exist")
	} else if _, stillCached := cached.Sessions[idTrash]; stillCached {
		t.Fatalf("expected %s pruned from cache", idTrash)
	}

	after, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect after trash failed: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected 1 session remaining after trash, got %d", len(after))
	}
	if _, ok := after[idKeep]; !ok {
		t.Fatalf("expected %s preserved", idKeep)
	}
}

func TestDeleteSessionPermanentWithNestedSubsessions(t *testing.T) {
	trashHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", trashHome)

	root := t.TempDir()
	cfg := config.FromRoot(root)

	rootID := "11111111-1111-4111-8111-111111111111"
	childID := "22222222-2222-4222-8222-222222222222"
	grandID := "33333333-3333-4333-8333-333333333333"
	otherID := "44444444-4444-4444-8444-444444444444"

	for _, id := range []string{rootID, childID, grandID, otherID} {
		dir := filepath.Join(cfg.BrainDir, id, ".system_generated", "subagents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(cfg.BrainDir, id, "task.md"), []byte("# "+id+"\n"), 0o600); err != nil {
			t.Fatalf("write task.md: %v", err)
		}
	}

	// Link root -> child -> grandchild via subagent descriptors.
	if err := os.WriteFile(
		filepath.Join(cfg.BrainDir, rootID, ".system_generated", "subagents", childID+".json"),
		[]byte(`{"subagent_id":"`+childID+`","role":"Child","type":"research"}`),
		0o600,
	); err != nil {
		t.Fatalf("write child descriptor: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfg.BrainDir, childID, ".system_generated", "subagents", grandID+".json"),
		[]byte(`{"subagent_id":"`+grandID+`","role":"Grandchild","type":"research"}`),
		0o600,
	); err != nil {
		t.Fatalf("write grandchild descriptor: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	res, err := DeleteSession(cfg, sessions, rootID, DeleteModePermanent)
	if err != nil {
		t.Fatalf("DeleteSession(DeleteModePermanent) failed: %v", err)
	}
	if len(res.DeletedIDs) != 3 {
		t.Fatalf("expected 3 sessions deleted (root + child + grandchild), got %+v", res.DeletedIDs)
	}

	for _, deleted := range []string{rootID, childID, grandID} {
		if _, err := os.Stat(filepath.Join(cfg.BrainDir, deleted)); !os.IsNotExist(err) {
			t.Fatalf("expected %s permanently removed, err=%v", deleted, err)
		}
	}

	// Ensure nothing was sent to Trash on permanent delete.
	if _, err := os.Stat(filepath.Join(trashHome, "Trash")); !os.IsNotExist(err) {
		t.Fatalf("did not expect Trash dir created for permanent delete, err=%v", err)
	}

	after, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect after permanent delete failed: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected only unrelated session remaining (zero orphan subsessions), got %+v", after)
	}
	if _, ok := after[otherID]; !ok {
		t.Fatalf("expected %s preserved", otherID)
	}
}

func TestDeleteChildSubsessionPreservesParent(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	parentID := "11111111-1111-4111-8111-111111111111"
	childID := "22222222-2222-4222-8222-222222222222"

	for _, id := range []string{parentID, childID} {
		dir := filepath.Join(cfg.BrainDir, id, ".system_generated", "subagents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(cfg.BrainDir, id, "task.md"), []byte("# "+id+"\n"), 0o600); err != nil {
			t.Fatalf("write task.md: %v", err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(cfg.BrainDir, parentID, ".system_generated", "subagents", childID+".json"),
		[]byte(`{"subagent_id":"`+childID+`","role":"Helper","type":"research"}`),
		0o600,
	); err != nil {
		t.Fatalf("write child descriptor: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	res, err := DeleteSession(cfg, sessions, childID, DeleteModePermanent)
	if err != nil {
		t.Fatalf("DeleteSession(child) failed: %v", err)
	}
	if len(res.DeletedIDs) != 1 || res.DeletedIDs[0] != childID {
		t.Fatalf("expected only childID deleted, got %+v", res.DeletedIDs)
	}

	after, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect after child delete failed: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected parent session preserved, got %+v", after)
	}
	if len(after[parentID].SubSessions) != 0 {
		t.Fatalf("expected deleted child absent from parent SubSessions, got %+v", after[parentID].SubSessions)
	}
}

func TestDeleteSafetyGuardsAndCrossDeviceTrashFallback(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)
	if err := os.MkdirAll(cfg.BrainDir, 0o755); err != nil {
		t.Fatalf("mkdir brain: %v", err)
	}

	outsideDir := filepath.Join(root, "outside-secret")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outsideDir: %v", err)
	}

	validID := "55555555-5555-4555-8555-555555555555"
	badSessions := map[string]Session{
		validID: {
			ID:   validID,
			Path: outsideDir,
		},
	}
	if _, err := DeleteSession(cfg, badSessions, validID, DeleteModePermanent); err == nil {
		t.Fatal("expected error when candidate Path escapes BrainDir")
	}
	if _, err := os.Stat(outsideDir); err != nil {
		t.Fatalf("outsideDir must remain untouched after safety rejection: %v", err)
	}

	// Verify EXDEV copy-and-remove fallback preserves files, nested dirs, and symlinks.
	srcDir := filepath.Join(root, "dir with space")
	nested := filepath.Join(srcDir, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "data.txt"), []byte("cross-device payload"), 0o640); err != nil {
		t.Fatalf("write data.txt: %v", err)
	}
	if err := os.Symlink("data.txt", filepath.Join(nested, "link.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	trashRoot := filepath.Join(t.TempDir(), "Trash")
	filesDir := filepath.Join(trashRoot, "files")
	infoDir := filepath.Join(trashRoot, "info")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		t.Fatalf("mkdir filesDir: %v", err)
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		t.Fatalf("mkdir infoDir: %v", err)
	}

	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	exdevRename := func(_, _ string) error {
		return syscall.EXDEV
	}
	if err := movePathToTrash(srcDir, filesDir, infoDir, now, exdevRename); err != nil {
		t.Fatalf("movePathToTrash with EXDEV failed: %v", err)
	}
	if _, err := os.Stat(srcDir); !os.IsNotExist(err) {
		t.Fatalf("expected original directory removed after EXDEV copy, stat err=%v", err)
	}
	copied, err := os.ReadFile(filepath.Join(filesDir, "dir with space", "nested", "data.txt"))
	if err != nil || string(copied) != "cross-device payload" {
		t.Fatalf("unexpected copied content=%q err=%v", string(copied), err)
	}
	linkTarget, err := os.Readlink(filepath.Join(filesDir, "dir with space", "nested", "link.txt"))
	if err != nil || linkTarget != "data.txt" {
		t.Fatalf("expected preserved relative symlink target, got %q err=%v", linkTarget, err)
	}
	infoRaw, err := os.ReadFile(filepath.Join(infoDir, "dir with space.trashinfo"))
	if err != nil {
		t.Fatalf("read trashinfo: %v", err)
	}
	if !strings.Contains(string(infoRaw), "dir%20with%20space") {
		t.Fatalf("expected percent-escaped Path in trashinfo, got:\n%s", string(infoRaw))
	}
}
