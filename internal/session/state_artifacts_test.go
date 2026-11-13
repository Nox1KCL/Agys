package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestUserStateOverridesAndOrdering(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	idPinnedByState := "11111111-1111-4111-8111-111111111111"
	idUnpinnedFromAnn := "22222222-2222-4222-8222-222222222222"
	idArchived := "33333333-3333-4333-8333-333333333333"

	for _, dir := range []string{
		filepath.Join(cfg.BrainDir, idPinnedByState),
		filepath.Join(cfg.BrainDir, idUnpinnedFromAnn),
		filepath.Join(cfg.BrainDir, idArchived),
		cfg.AnnotationsDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
	}

	now := time.Now()
	writeTimedMD := func(id, body string, ts time.Time) {
		p := filepath.Join(cfg.BrainDir, id, "task.md")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %q: %v", p, err)
		}
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatalf("chtimes %q: %v", p, err)
		}
	}

	// idPinnedByState is oldest, idUnpinnedFromAnn is newest, idArchived is middle.
	writeTimedMD(idPinnedByState, "# Older Pinned\n", now.Add(-48*time.Hour))
	writeTimedMD(idArchived, "# Archived Item\n", now.Add(-24*time.Hour))
	writeTimedMD(idUnpinnedFromAnn, "# Newer Unpinned\n", now.Add(-1*time.Hour))

	// Annotations initially mark idUnpinnedFromAnn as pinned:true.
	if err := os.WriteFile(filepath.Join(cfg.AnnotationsDir, idUnpinnedFromAnn+".pbtxt"), []byte(`title:"Newer Unpinned" pinned:true`), 0o600); err != nil {
		t.Fatalf("write annotation: %v", err)
	}

	var st UserState
	st.SetPinned(idPinnedByState, true)
	st.SetPinned(idUnpinnedFromAnn, false)
	st.SetArchived(idArchived, true)
	if err := SaveUserState(cfg.StateFile, st); err != nil {
		t.Fatalf("SaveUserState failed: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if !sessions[idPinnedByState].Pinned {
		t.Fatalf("expected %s pinned=true", idPinnedByState)
	}
	if sessions[idUnpinnedFromAnn].Pinned {
		t.Fatalf("expected %s unpinned override to take precedence over annotation", idUnpinnedFromAnn)
	}
	if !sessions[idArchived].Archived {
		t.Fatalf("expected %s archived=true", idArchived)
	}

	// Default Search excludes archived and puts pinned first despite older mtime.
	visible := Search(sessions, "")
	if len(visible) != 2 {
		t.Fatalf("got %d visible sessions, want 2", len(visible))
	}
	if visible[0].Session.ID != idPinnedByState || visible[1].Session.ID != idUnpinnedFromAnn {
		t.Fatalf("unexpected order with pinned first: [%s, %s]", visible[0].Session.ID, visible[1].Session.ID)
	}

	// SearchWithOptions(IncludeArchived: true) reveals archived session.
	withArchived := SearchWithOptions(sessions, "", SearchOptions{IncludeArchived: true})
	if len(withArchived) != 3 {
		t.Fatalf("got %d sessions with archived included, want 3", len(withArchived))
	}

	// Custom title overrides annotation title without altering annotations/.
	st.SetTitle(idUnpinnedFromAnn, "  Renamed\nSession\tTitle ")
	if err := SaveUserState(cfg.StateFile, st); err != nil {
		t.Fatalf("SaveUserState with custom title failed: %v", err)
	}

	sessions, err = Collect(cfg)
	if err != nil {
		t.Fatalf("Collect after rename failed: %v", err)
	}
	renamed := sessions[idUnpinnedFromAnn]
	if renamed.Title != "Renamed Session Title" || renamed.TitleSource != TitleSourceCustom {
		t.Fatalf("unexpected renamed session: title=%q source=%q", renamed.Title, renamed.TitleSource)
	}
	if renamed.DefaultTitle != "Newer Unpinned" || renamed.DefaultTitleSource != TitleSourceAnnotation {
		t.Fatalf("unexpected preserved default title: title=%q source=%q", renamed.DefaultTitle, renamed.DefaultTitleSource)
	}

	// Clearing custom title removes override cleanly and restores annotation title.
	st.SetTitle(idUnpinnedFromAnn, "   ")
	if err := SaveUserState(cfg.StateFile, st); err != nil {
		t.Fatalf("SaveUserState clear title failed: %v", err)
	}
	sessions, err = Collect(cfg)
	if err != nil {
		t.Fatalf("Collect after clearing title failed: %v", err)
	}
	if sessions[idUnpinnedFromAnn].Title != "Newer Unpinned" || sessions[idUnpinnedFromAnn].TitleSource != TitleSourceAnnotation {
		t.Fatalf("expected restored annotation title, got %+v", sessions[idUnpinnedFromAnn])
	}
}

func TestLoadArtifactsCheckboxesAndTruncation(t *testing.T) {
	dir := t.TempDir()
	taskBody := "# Sprint\n- [x] Init\n- [X] Tests\n- [/] UI\n- [ ] Release\n"
	planBody := strings.Repeat("Plan line\n", 150)

	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte(taskBody), 0o600); err != nil {
		t.Fatalf("write task.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "implementation_plan.md"), []byte(planBody), 0o600); err != nil {
		t.Fatalf("write implementation_plan.md: %v", err)
	}

	art := LoadArtifacts(dir)
	if art.TaskDone != 2 || art.TaskTotal != 4 {
		t.Fatalf("task progress = %d/%d, want 2/4", art.TaskDone, art.TaskTotal)
	}
	if !strings.Contains(art.TaskMarkdown, "- [x] Init") {
		t.Fatalf("unexpected TaskMarkdown: %q", art.TaskMarkdown)
	}
	if !strings.Contains(art.PlanMarkdown, "... (truncated)") {
		t.Fatalf("expected truncated plan indicator, got len=%d", len(art.PlanMarkdown))
	}
	if art.Walkthrough != "" {
		t.Fatalf("expected empty walkthrough, got %q", art.Walkthrough)
	}
}
