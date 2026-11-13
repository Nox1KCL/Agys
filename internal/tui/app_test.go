package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func sampleSessions(now time.Time) map[string]session.Session {
	parentID := "11111111-1111-4111-8111-111111111111"
	subID := "22222222-2222-4222-8222-222222222222"
	otherID := "33333333-3333-4333-8333-333333333333"

	sub := session.Session{
		ID:           subID,
		ParentID:     parentID,
		Title:        "Extension code reader",
		SubagentRole: "Extension code reader",
		SubagentType: "research",
		WorkDir:      "/home/nox/IT-projects/Delos",
		FirstPrompt:  "Inspect extension manifest",
		UpdatedAt:    now.Add(-2 * time.Hour),
	}

	return map[string]session.Session{
		parentID: {
			ID:          parentID,
			Title:       "Delos Chrome Extension Setup",
			WorkDir:     "/home/nox/IT-projects/Delos",
			FirstPrompt: "Setup chrome extension build",
			UpdatedAt:   now.Add(-1 * time.Hour),
			SubSessions: []session.Session{sub},
		},
		subID: sub,
		otherID: {
			ID:          otherID,
			Title:       "Linux Daemon With Unix Domain Sockets",
			WorkDir:     "/home/nox/IT-projects/Synapsed",
			FirstPrompt: "Explain unix socket framing",
			UpdatedAt:   now.Add(-25 * time.Hour),
		},
	}
}

func TestModelSearchAndTreeNavigation(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)
	m := New(config.FromRoot(t.TempDir()))
	m.now = func() time.Time { return now }

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = next.(Model)

	next, _ = m.Update(sessionsLoadedMsg{sessions: sampleSessions(now)})
	m = next.(Model)

	if len(m.rows) != 2 {
		t.Fatalf("expected 2 root rows initially, got %d", len(m.rows))
	}

	// Press 'l' to expand subsessions on the first parent.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	if len(m.rows) != 3 || !m.rows[1].IsSub {
		t.Fatalf("expected subsession expanded with 'l', got %+v", m.rows)
	}

	// Press 'l' again to collapse.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	if len(m.rows) != 2 {
		t.Fatalf("expected subsession collapsed, got %d rows", len(m.rows))
	}

	// Press '/' to enter search mode and type "synapsed".
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(Model)
	if !m.searching {
		t.Fatal("expected searching=true after pressing '/'")
	}

	for _, r := range "synapsed" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}

	if len(m.rows) != 1 || m.rows[0].Session.ID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("expected Synapsed session matched by directory, got %+v", m.rows)
	}

	// Press Esc to cancel search and restore full list.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.searching || len(m.rows) != 2 {
		t.Fatalf("expected search cleared on Esc, searching=%v rows=%d", m.searching, len(m.rows))
	}
}

func TestViewFitsWithinTerminalBounds(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)

	assertBounds := func(label string, m Model, w, h int) {
		t.Helper()
		lay := m.layoutDimensions()
		header := m.headerView(lay.listInnerW)
		if strings.Contains(header, "\n") {
			t.Fatalf("%s (%dx%d) header wrapped: %q", label, w, h, header)
		}
		if hw := lipgloss.Width(header); hw > lay.listInnerW {
			t.Fatalf("%s (%dx%d) header width %d exceeded listInnerW %d (%q)", label, w, h, hw, lay.listInnerW, header)
		}

		pHeader := m.previewHeaderView(lay.previewInnerW)
		if strings.Contains(pHeader, "\n") {
			t.Fatalf("%s (%dx%d) preview header wrapped: %q", label, w, h, pHeader)
		}
		if phw := lipgloss.Width(pHeader); phw > lay.previewInnerW {
			t.Fatalf("%s (%dx%d) preview header width %d exceeded previewInnerW %d (%q)", label, w, h, phw, lay.previewInnerW, pHeader)
		}

		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > h {
			t.Fatalf("%s (%dx%d) view exceeded height: got %d lines", label, w, h, len(lines))
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("%s (%dx%d) line %d exceeded width: got %d (%q)", label, w, h, i, lw, line)
			}
		}
	}

	for _, size := range []struct{ w, h int }{
		{55, 18},
		{80, 24},
		{120, 36},
		{190, 50},
	} {
		m := New(config.FromRoot(t.TempDir()))
		m.now = func() time.Time { return now }

		next, _ := m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
		m = next.(Model)
		next, _ = m.Update(sessionsLoadedMsg{sessions: sampleSessions(now)})
		m = next.(Model)

		assertBounds("browse", m, size.w, size.h)

		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		assertBounds("preview-focus", m, size.w, size.h)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)

		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		m = next.(Model)
		assertBounds("account-focus", m, size.w, size.h)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		m = next.(Model)

		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		m = next.(Model)
		assertBounds("which-key", m, size.w, size.h)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		m = next.(Model)

		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m = next.(Model)
		assertBounds("search-placeholder", m, size.w, size.h)

		for _, r := range strings.Repeat("test ", 20) {
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = next.(Model)
		}
		assertBounds("search-long-query", m, size.w, size.h)

		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		m = next.(Model)
		assertBounds("confirm-trash", m, size.w, size.h)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)

		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
		m = next.(Model)
		assertBounds("confirm-permanent", m, size.w, size.h)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)

		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m = next.(Model)
		assertBounds("rename", m, size.w, size.h)
	}
}

func TestPinAndArchivePersistenceAndSorting(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)
	cfg := config.FromRoot(t.TempDir())
	m := New(cfg)
	m.now = func() time.Time { return now }

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = next.(Model)
	next, _ = m.Update(sessionsLoadedMsg{sessions: sampleSessions(now)})
	m = next.(Model)

	// Move to older session (Synapsed) and pin it with 'p'.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.rows[m.cursor].Session.ID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("expected second row selected, got %s", m.rows[m.cursor].Session.ID)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(Model)

	if m.rows[0].Session.ID != "33333333-3333-4333-8333-333333333333" || !m.rows[0].Session.Pinned {
		t.Fatalf("expected pinned session sorted first, got rows=%+v", m.rows)
	}

	st, err := session.LoadUserState(cfg.StateFile)
	if err != nil {
		t.Fatalf("LoadUserState failed: %v", err)
	}
	ov := st.Sessions["33333333-3333-4333-8333-333333333333"]
	if ov.Pinned == nil || !*ov.Pinned {
		t.Fatalf("expected pinned=true persisted in state.json, got %+v", ov)
	}

	// Move to Delos session and archive it with 'a'.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(Model)

	if len(m.rows) != 1 || m.rows[0].Session.ID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("expected archived session hidden by default, got %+v", m.rows)
	}

	st, err = session.LoadUserState(cfg.StateFile)
	if err != nil {
		t.Fatalf("LoadUserState failed: %v", err)
	}
	ovDelos := st.Sessions["11111111-1111-4111-8111-111111111111"]
	if ovDelos.Archived == nil || !*ovDelos.Archived {
		t.Fatalf("expected archived=true persisted in state.json, got %+v", ovDelos)
	}

	// Press 'A' to reveal archived sessions.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = next.(Model)
	if !m.showArchived || len(m.rows) != 2 {
		t.Fatalf("expected archived session shown after pressing A, showArchived=%v rows=%d", m.showArchived, len(m.rows))
	}
	if !strings.Contains(m.View(), "[archived]") {
		t.Fatalf("expected archive badge in view, got:\n%s", m.View())
	}
}

func TestPreviewFocusScrollingAndTwoPaneView(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)
	root := t.TempDir()
	cfg := config.FromRoot(root)

	id := "11111111-1111-4111-8111-111111111111"
	sessDir := filepath.Join(cfg.BrainDir, id)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	var taskLines strings.Builder
	taskLines.WriteString("- [x] First task done\n- [ ] Second task pending\n")
	for i := range 35 {
		taskLines.WriteString("- [ ] Extra line number ")
		taskLines.WriteRune(rune('A' + (i % 26)))
		taskLines.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(sessDir, "task.md"), []byte(taskLines.String()), 0o644); err != nil {
		t.Fatalf("write task.md: %v", err)
	}

	sessions := sampleSessions(now)
	parent := sessions[id]
	parent.Path = sessDir
	sessions[id] = parent

	m := New(cfg)
	m.now = func() time.Time { return now }

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = next.(Model)
	next, _ = m.Update(sessionsLoadedMsg{sessions: sessions})
	m = next.(Model)

	wideView := m.View()
	if !strings.Contains(wideView, "Preview") || !strings.Contains(wideView, "task.md (1/37)") {
		t.Fatalf("expected two-pane view with preview metadata and task summary, got:\n%s", wideView)
	}

	// Focus preview with Tab and scroll down.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if !m.previewFocused {
		t.Fatal("expected previewFocused=true after Tab")
	}

	for range 5 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = next.(Model)
	}
	if m.preview.YOffset == 0 {
		t.Fatalf("expected preview to scroll down, got YOffset=%d", m.preview.YOffset)
	}

	// Narrow mode (< 96 cols) switches full modal to preview when previewFocused.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 72, Height: 22})
	m = next.(Model)
	narrowPreview := m.View()
	if !strings.Contains(narrowPreview, "Preview [scroll]") {
		t.Fatalf("expected narrow preview modal header, got:\n%s", narrowPreview)
	}

	// Esc exits preview focus back to list.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.previewFocused {
		t.Fatal("expected previewFocused=false after Esc")
	}
}

func TestWhichKeyOverlayAndMinimalFooter(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)
	m := New(config.FromRoot(t.TempDir()))
	m.now = func() time.Time { return now }

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = next.(Model)
	next, _ = m.Update(sessionsLoadedMsg{sessions: sampleSessions(now)})
	m = next.(Model)

	browseView := m.View()
	if !strings.Contains(browseView, "? keys") {
		t.Fatalf("expected minimal footer with '? keys', got:\n%s", browseView)
	}
	if strings.Contains(browseView, "Keybindings") {
		t.Fatal("did not expect keybindings popup before pressing '?'")
	}

	// Press '?' to open which-key overlay.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(Model)
	if !m.showHelp {
		t.Fatal("expected showHelp=true after pressing '?'")
	}
	helpView := m.View()
	if !strings.Contains(helpView, "Keybindings") || !strings.Contains(helpView, "Pin / unpin session") {
		t.Fatalf("expected which-key overlay content, got:\n%s", helpView)
	}
	// Background panes remain visible around the floating overlay.
	if !strings.Contains(helpView, "Delos Chrome Extension Setup") && !strings.Contains(helpView, "Preview") {
		t.Fatalf("expected background UI visible around floating overlay, got:\n%s", helpView)
	}

	// Press Esc to close overlay cleanly.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.showHelp {
		t.Fatal("expected showHelp=false after Esc")
	}

	// Typing '?' in search mode must append to query rather than opening help.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(Model)
	if m.showHelp || m.searchInput.Value() != "?" {
		t.Fatalf("expected '?' typed in search input, showHelp=%v val=%q", m.showHelp, m.searchInput.Value())
	}

	// Compact terminal height supports scrolling inside which-key overlay.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	next, _ = m.Update(tea.WindowSizeMsg{Width: 55, Height: 18})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.helpOffset == 0 {
		t.Fatal("expected which-key overlay to scroll on compact terminal height")
	}
}

func TestRenameSessionPersistenceAndReset(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)
	cfg := config.FromRoot(t.TempDir())
	m := New(cfg)
	m.now = func() time.Time { return now }

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = next.(Model)
	next, _ = m.Update(sessionsLoadedMsg{sessions: sampleSessions(now)})
	m = next.(Model)

	// Press 'r' on first session (Delos), replace text with custom name, and press Esc -> no change.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(Model)
	if !m.renaming {
		t.Fatal("expected renaming=true after pressing 'r'")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.renaming || m.rows[0].Session.Title != "Delos Chrome Extension Setup" {
		t.Fatalf("expected Esc to cancel rename, renaming=%v title=%q", m.renaming, m.rows[0].Session.Title)
	}

	// Rename Delos session via 'r' + Enter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(Model)
	for range len("Delos Chrome Extension Setup") {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	for _, ch := range "Custom Delos Title" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	if m.renaming {
		t.Fatal("expected renaming=false after Enter")
	}
	if m.rows[0].Session.Title != "Custom Delos Title" {
		t.Fatalf("expected updated row title, got %q", m.rows[0].Session.Title)
	}
	if !strings.Contains(m.View(), "Custom Delos Title") {
		t.Fatalf("expected updated title in rendered view")
	}

	st, err := session.LoadUserState(cfg.StateFile)
	if err != nil {
		t.Fatalf("LoadUserState failed: %v", err)
	}
	ov := st.Sessions["11111111-1111-4111-8111-111111111111"]
	if ov.Title == nil || *ov.Title != "Custom Delos Title" {
		t.Fatalf("expected persisted custom title, got %+v", ov)
	}

	// Clearing the input on Enter resets to original title.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(Model)
	for range len("Custom Delos Title") {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.rows[0].Session.Title != "Delos Chrome Extension Setup" {
		t.Fatalf("expected restored original title, got %q", m.rows[0].Session.Title)
	}
}

func TestOptionsExitOnResumeAndInitialQuery(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)
	fakeBinDir := t.TempDir()
	fakeAgy := filepath.Join(fakeBinDir, "agy")
	if err := os.WriteFile(fakeAgy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake agy: %v", err)
	}
	t.Setenv("PATH", fakeBinDir)

	m := NewWithOptions(config.FromRoot(t.TempDir()), Options{
		InitialQuery: "synapsed",
		ExitOnResume: true,
		ShowArchived: true,
	})
	m.now = func() time.Time { return now }

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = next.(Model)
	next, _ = m.Update(sessionsLoadedMsg{sessions: sampleSessions(now)})
	m = next.(Model)

	if len(m.rows) != 1 || m.rows[0].Session.ID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("expected initial query filter applied, got %+v", m.rows)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected quit cmd when ExitOnResume=true")
	}
	target, ok := m.PendingResume()
	if !ok || target.ID != "33333333-3333-4333-8333-333333333333" || target.WorkDir != "/home/nox/IT-projects/Synapsed" {
		t.Fatalf("unexpected PendingResume target: ok=%v target=%+v", ok, target)
	}
}

func TestDeleteSessionTrashAndPermanentConfirmationFlow(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	trashHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", trashHome)

	root := t.TempDir()
	cfg := config.FromRoot(root)

	parentID := "11111111-1111-4111-8111-111111111111"
	subID := "22222222-2222-4222-8222-222222222222"
	otherID := "33333333-3333-4333-8333-333333333333"

	for _, id := range []string{parentID, subID, otherID} {
		if err := os.MkdirAll(filepath.Join(cfg.BrainDir, id), 0o755); err != nil {
			t.Fatalf("mkdir session %s: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(cfg.BrainDir, id, "task.md"), []byte("# "+id+"\n"), 0o600); err != nil {
			t.Fatalf("write task.md: %v", err)
		}
	}
	if err := os.MkdirAll(cfg.AnnotationsDir, 0o755); err != nil {
		t.Fatalf("mkdir annotations: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.AnnotationsDir, otherID+".pbtxt"), []byte(`title:"Synapsed"`), 0o600); err != nil {
		t.Fatalf("write annotation: %v", err)
	}

	sessions := sampleSessions(now)
	for id, s := range sessions {
		s.Path = filepath.Join(cfg.BrainDir, id)
		sessions[id] = s
	}
	parent := sessions[parentID]
	parent.SubSessions[0].Path = filepath.Join(cfg.BrainDir, subID)
	sessions[parentID] = parent

	var initialState session.UserState
	initialState.SetPinned(otherID, true)
	if err := session.SaveUserState(cfg.StateFile, initialState); err != nil {
		t.Fatalf("SaveUserState: %v", err)
	}

	m := New(cfg)
	m.now = func() time.Time { return now }
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = next.(Model)
	next, _ = m.Update(sessionsLoadedMsg{sessions: sessions, userState: initialState})
	m = next.(Model)

	// Typing 'd' in search mode must append to query rather than opening confirmation modal.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(Model)
	if m.confirmDelete != nil || m.searchInput.Value() != "d" {
		t.Fatalf("expected 'd' typed in search mode, confirmDelete=%v val=%q", m.confirmDelete, m.searchInput.Value())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)

	// Select otherID (Synapsed) -> press 'd' -> cancel with 'n' -> files stay intact.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.rows[m.cursor].Session.ID != otherID {
		t.Fatalf("expected %s selected, got %s", otherID, m.rows[m.cursor].Session.ID)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(Model)
	if m.confirmDelete == nil || !strings.Contains(m.View(), "Move session to Trash?") {
		t.Fatalf("expected Trash confirmation overlay, got:\n%s", m.View())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(Model)
	if m.confirmDelete != nil {
		t.Fatal("expected confirmation closed on 'n'")
	}
	if _, err := os.Stat(filepath.Join(cfg.BrainDir, otherID)); err != nil {
		t.Fatalf("expected %s intact after cancel: %v", otherID, err)
	}

	// Press 'd' then Enter -> moves otherID to Trash and prunes state.json.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.confirmDelete != nil || m.errMsg != "" {
		t.Fatalf("unexpected state after trash confirm: confirm=%v err=%q", m.confirmDelete, m.errMsg)
	}
	if _, err := os.Stat(filepath.Join(cfg.BrainDir, otherID)); !os.IsNotExist(err) {
		t.Fatalf("expected %s moved out of brain, stat err=%v", otherID, err)
	}
	if _, err := os.Stat(filepath.Join(trashHome, "Trash", "files", otherID)); err != nil {
		t.Fatalf("expected %s in Trash/files: %v", otherID, err)
	}
	if len(m.rows) != 1 || m.rows[0].Session.ID != parentID {
		t.Fatalf("expected only parent row remaining, got %+v", m.rows)
	}
	stAfterTrash, err := session.LoadUserState(cfg.StateFile)
	if err != nil {
		t.Fatalf("LoadUserState after trash: %v", err)
	}
	if _, exists := stAfterTrash.Sessions[otherID]; exists {
		t.Fatalf("expected trashed session removed from state.json")
	}

	// Expand parent -> select child subsession -> press 'd' + 'y' -> only child is trashed.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if !m.rows[m.cursor].IsSub || m.rows[m.cursor].Session.ID != subID {
		t.Fatalf("expected child subsession selected, got %+v", m.rows[m.cursor])
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(Model)
	if !strings.Contains(m.View(), "Subsession of:") {
		t.Fatalf("expected subsession context in confirmation modal, got:\n%s", m.View())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(Model)
	if _, err := os.Stat(filepath.Join(cfg.BrainDir, subID)); !os.IsNotExist(err) {
		t.Fatalf("expected child %s trashed, err=%v", subID, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.BrainDir, parentID)); err != nil {
		t.Fatalf("expected parent %s preserved, err=%v", parentID, err)
	}
	if len(m.sessions[parentID].SubSessions) != 0 {
		t.Fatalf("expected child detached from parent SubSessions, got %+v", m.sessions[parentID].SubSessions)
	}

	// Press 'D' on parentID: Enter defaults safely to Cancel, while Tab + Enter permanently deletes.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = next.(Model)
	if m.confirmDelete == nil || !strings.Contains(m.View(), "Permanently delete session?") {
		t.Fatalf("expected permanent delete modal, got:\n%s", m.View())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.confirmDelete != nil {
		t.Fatal("expected Enter on default Cancel button to cancel permanent delete")
	}
	if _, err := os.Stat(filepath.Join(cfg.BrainDir, parentID)); err != nil {
		t.Fatalf("expected parent %s untouched after default-Cancel Enter: %v", parentID, err)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.confirmDelete != nil || m.errMsg != "" {
		t.Fatalf("unexpected state after permanent delete: confirm=%v err=%q", m.confirmDelete, m.errMsg)
	}
	if _, err := os.Stat(filepath.Join(cfg.BrainDir, parentID)); !os.IsNotExist(err) {
		t.Fatalf("expected parent %s permanently deleted, err=%v", parentID, err)
	}
	if len(m.rows) != 0 {
		t.Fatalf("expected 0 remaining rows, got %d", len(m.rows))
	}
}

func TestAccountPanelAcrossWidthsAndRefresh(t *testing.T) {
	now := time.Date(2026, 11, 13, 13, 3, 0, 0, time.UTC)
	root := t.TempDir()
	cfg := config.FromRoot(root)

	if err := os.MkdirAll(filepath.Dir(cfg.PluginUsageCacheFile), 0o700); err != nil {
		t.Fatalf("mkdir plugin cache: %v", err)
	}
	cachedPayload := `{
	  "groups": [
	    {
	      "name": "Gemini Models",
	      "buckets": [
	        {"id": "gem-w", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.61, "reset_time": "2026-11-14T16:24:00Z"},
	        {"id": "gem-5", "name": "Five Hour", "window": "5h", "remaining_fraction": 0.78, "reset_time": "2026-11-13T15:51:00Z"}
	      ]
	    },
	    {
	      "name": "Claude and GPT models",
	      "buckets": [
	        {"id": "3p-w", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.74, "reset_time": "2026-11-19T14:24:00Z"}
	      ]
	    }
	  ]
	}`
	if err := os.WriteFile(cfg.PluginUsageCacheFile, []byte(cachedPayload), 0o600); err != nil {
		t.Fatalf("write plugin cache: %v", err)
	}
	if err := os.Chtimes(cfg.PluginUsageCacheFile, now, now); err != nil {
		t.Fatalf("chtimes plugin cache: %v", err)
	}

	m := New(cfg)
	m.now = func() time.Time { return now }

	next, _ := m.Update(sessionsLoadedMsg{sessions: sampleSessions(now)})
	m = next.(Model)

	// Wide terminal (190 cols): Preview (left) + Sessions (middle) + Account (right) side-by-side.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 190, Height: 50})
	m = next.(Model)
	wideView := m.View()
	if !strings.Contains(wideView, "Preview") || !strings.Contains(wideView, "Quota Limits") || !strings.Contains(wideView, "61% remaining") {
		t.Fatalf("expected 3-pane wide view with Preview and Account limits, got:\n%s", wideView)
	}
	foundOrder := false
	for _, line := range strings.Split(wideView, "\n") {
		pIdx := strings.Index(line, "Preview")
		sIdx := strings.Index(line, "/ search")
		aIdx := strings.Index(line, "Account")
		if pIdx >= 0 && sIdx >= 0 && aIdx >= 0 {
			if !(pIdx < sIdx && sIdx < aIdx) {
				t.Fatalf("expected Preview < Sessions < Account horizontal order, got line: %q", line)
			}
			foundOrder = true
			break
		}
	}
	if !foundOrder {
		t.Fatalf("expected header row containing Preview, Sessions, and Account in 3-pane view:\n%s", wideView)
	}

	// Medium terminal (120 cols): 2-pane default shows Preview on left & Sessions on right; pressing u switches right pane to Account.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = next.(Model)
	medBrowse := m.View()
	if strings.Contains(medBrowse, "Quota Limits") {
		t.Fatalf("expected Preview in 2-pane browse view before pressing u, got:\n%s", medBrowse)
	}
	foundTwoPaneOrder := false
	for _, line := range strings.Split(medBrowse, "\n") {
		pIdx := strings.Index(line, "Preview")
		sIdx := strings.Index(line, "/ search")
		if pIdx >= 0 && sIdx >= 0 {
			if pIdx >= sIdx {
				t.Fatalf("expected Preview left of Sessions in 2-pane view, got line: %q", line)
			}
			foundTwoPaneOrder = true
			break
		}
	}
	if !foundTwoPaneOrder {
		t.Fatalf("expected header row containing Preview and Sessions in 2-pane view:\n%s", medBrowse)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(Model)
	if !m.accountFocused {
		t.Fatal("expected accountFocused=true after pressing u")
	}
	medAccount := m.View()
	if !strings.Contains(medAccount, "Quota Limits") || !strings.Contains(medAccount, "Claude and GPT models") {
		t.Fatalf("expected Account panel visible in 120-col mode after u, got:\n%s", medAccount)
	}

	// Narrow terminal (80 cols): pressing u keeps Account accessible; Esc returns cleanly.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	m = next.(Model)
	narrowAccount := m.View()
	if !strings.Contains(narrowAccount, "Account [scroll]") || !strings.Contains(narrowAccount, "Quota Limits") {
		t.Fatalf("expected Account panel accessible on narrow 80x18 terminal, got:\n%s", narrowAccount)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.account.YOffset == 0 {
		t.Fatalf("expected downward scroll in short narrow Account viewport, got YOffset=%d", m.account.YOffset)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.accountFocused {
		t.Fatal("expected Esc to exit accountFocused")
	}

	// Live refresh with 'U' using fake agy CLI in PATH.
	binDir := t.TempDir()
	fakeAgy := filepath.Join(binDir, "agy")
	script := `#!/bin/sh
if [ "$1" = "-p" ] && [ "$2" = "/usage" ]; then
  printf '%s\n' '{"command":{"data":{"tier":"Pro","groups":[{"name":"Gemini Models","buckets":[{"id":"w","name":"Weekly","window":"weekly","remaining_fraction":0.92,"reset_time":"2026-11-15T16:24:00Z"}]}]}}}'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(fakeAgy, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agy: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("AGY_BIN", "")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(Model)
	if !m.accountRefreshing || cmd == nil {
		t.Fatalf("expected accountRefreshing=true and non-nil cmd on U")
	}

	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.accountRefreshing || m.accountErr != "" {
		t.Fatalf("expected refresh to complete cleanly, err=%q", m.accountErr)
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 190, Height: 50})
	m = next.(Model)
	refreshedView := m.View()
	if !strings.Contains(refreshedView, "Account · Pro") || !strings.Contains(refreshedView, "92% remaining") {
		t.Fatalf("expected refreshed live quota tier & percentage in view, got:\n%s", refreshedView)
	}
}
