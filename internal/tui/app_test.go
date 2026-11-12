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
