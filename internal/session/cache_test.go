package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestIncrementalCacheHitsAndInvalidation(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	id1 := "11111111-1111-4111-8111-111111111111"
	id2 := "22222222-2222-4222-8222-222222222222"

	dir1 := filepath.Join(cfg.BrainDir, id1)
	logs1 := filepath.Join(dir1, ".system_generated", "logs")
	dir2 := filepath.Join(cfg.BrainDir, id2)
	logs2 := filepath.Join(dir2, ".system_generated", "logs")
	kiDir := filepath.Join(cfg.KnowledgeDir, "ki_cache")

	for _, d := range []string{logs1, logs2, kiDir, cfg.AnnotationsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", d, err)
		}
	}

	t1Path := filepath.Join(logs1, "transcript.jsonl")
	t2Path := filepath.Join(logs2, "transcript.jsonl")
	writeFile := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}

	writeFile(t1Path, `{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-12T10:00:00Z","content":"<USER_REQUEST>\nSession one initial prompt\n</USER_REQUEST>"}`+"\n")
	writeFile(t2Path, `{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-12T10:00:00Z","content":"<USER_REQUEST>\nSession two initial prompt\n</USER_REQUEST>"}`+"\n")
	writeFile(filepath.Join(kiDir, "metadata.json"), `{"title":"Initial KI","summary":"KI summary v1","references":[{"id":"`+id1+`"}]}`)

	ann1 := filepath.Join(cfg.AnnotationsDir, id1+".pbtxt")
	writeFile(ann1, `title:"Session One" pinned:false`)

	// First Collect builds the gob index cache.
	s1, err := Collect(cfg)
	if err != nil {
		t.Fatalf("initial Collect failed: %v", err)
	}
	if len(s1) != 2 || s1[id1].Title != "Session One" || len(s1[id1].KnowledgeItems) != 1 {
		t.Fatalf("unexpected initial sessions: %+v", s1)
	}

	cacheInfo, err := os.Stat(cfg.CacheFile)
	if err != nil {
		t.Fatalf("expected cache file %q to exist: %v", cfg.CacheFile, err)
	}

	// Second Collect with zero changes should hit 100% cache and not rewrite cache file.
	time.Sleep(10 * time.Millisecond)
	s2, err := Collect(cfg)
	if err != nil {
		t.Fatalf("cached Collect failed: %v", err)
	}
	cacheInfoAfter, err := os.Stat(cfg.CacheFile)
	if err != nil {
		t.Fatalf("stat cache after hit: %v", err)
	}
	if !cacheInfoAfter.ModTime().Equal(cacheInfo.ModTime()) {
		t.Fatalf("expected cache hit to skip rewriting cache file (before=%v after=%v)", cacheInfo.ModTime(), cacheInfoAfter.ModTime())
	}
	if s2[id2].LastPrompt != "Session two initial prompt" {
		t.Fatalf("unexpected cached prompt: %q", s2[id2].LastPrompt)
	}

	// Appending to session 2 transcript invalidates only session 2 cleanly.
	writeFile(t2Path, `{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-12T10:00:00Z","content":"<USER_REQUEST>\nSession two initial prompt\n</USER_REQUEST>"}`+"\n"+
		`{"step_index":1,"type":"USER_INPUT","created_at":"2026-11-12T10:05:00Z","content":"<USER_REQUEST>\nSession two updated prompt\n</USER_REQUEST>"}`+"\n")
	writeFile(ann1, `title:"Session One Renamed Ann" pinned:true`)
	writeFile(filepath.Join(kiDir, "metadata.json"), `{"title":"Updated KI","summary":"KI summary v2","references":[{"id":"`+id1+`"}]}`)

	s3, err := Collect(cfg)
	if err != nil {
		t.Fatalf("invalidated Collect failed: %v", err)
	}
	if s3[id2].LastPrompt != "Session two updated prompt" {
		t.Fatalf("expected updated transcript prompt, got %q", s3[id2].LastPrompt)
	}
	if s3[id1].Title != "Session One Renamed Ann" || !s3[id1].Pinned {
		t.Fatalf("expected updated annotation for id1, got %+v", s3[id1])
	}
	if len(s3[id1].KnowledgeItems) != 1 || s3[id1].KnowledgeItems[0].Title != "Updated KI" {
		t.Fatalf("expected updated KnowledgeItem, got %+v", s3[id1].KnowledgeItems)
	}

	// Removing a session removes it cleanly and triggers cache refresh.
	if err := os.RemoveAll(dir2); err != nil {
		t.Fatalf("remove dir2: %v", err)
	}
	s4, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect after session removal failed: %v", err)
	}
	if len(s4) != 1 {
		t.Fatalf("got %d sessions after deletion, want 1", len(s4))
	}

	// Corrupt cache must gracefully recover without failing Collect.
	writeFile(cfg.CacheFile, "corrupted-binary-payload")
	s5, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect with corrupt cache failed: %v", err)
	}
	if len(s5) != 1 || s5[id1].Title != "Session One Renamed Ann" {
		t.Fatalf("expected clean rebuild after corrupt cache, got %+v", s5)
	}
}
