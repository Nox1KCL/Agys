package components

import (
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderRowStrictlyRespectsWidth(t *testing.T) {
	theme := styles.DefaultTheme()
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)

	row := Row{
		Session: session.Session{
			ID:        "083f7f21-26a2-4e44-b6ea-fa7fc1795a2f",
			Title:     "🐛 Very long session title with multi-byte runes about Linux Daemon With Unix Domain Sockets",
			WorkDir:   "/home/nox/IT-projects/goProjects/VeryLongProjectDirectoryName/Synapsed",
			UpdatedAt: now.Add(-4 * time.Hour),
			SubSessions: []session.Session{
				{ID: "child-1"},
			},
		},
		HasSubs: true,
	}

	for _, width := range []int{30, 50, 72, 96, 120} {
		for _, selected := range []bool{false, true} {
			out := RenderRow(row, selected, width, now, theme)
			if got := lipgloss.Width(out); got > width {
				t.Fatalf("width=%d selected=%v overflowed: got %d cells (%q)", width, selected, got, out)
			}
		}
	}

	pinnedRow := row
	pinnedRow.Session.Pinned = true
	pinnedOut := RenderRow(pinnedRow, false, 80, now, theme)
	if lipgloss.Width(pinnedOut) > 80 {
		t.Fatalf("pinned row overflowed: %d", lipgloss.Width(pinnedOut))
	}

	archivedRow := row
	archivedRow.Session.Archived = true
	archivedOut := RenderRow(archivedRow, false, 80, now, theme)
	if lipgloss.Width(archivedOut) > 80 {
		t.Fatalf("archived row overflowed: %d", lipgloss.Width(archivedOut))
	}
}

func TestBuildRowsTreeAndSearchExpansion(t *testing.T) {
	parent := session.Session{
		ID:    "parent",
		Title: "Parent",
		SubSessions: []session.Session{
			{ID: "sub-1", ParentID: "parent", Title: "Sub 1"},
			{ID: "sub-2", ParentID: "parent", Title: "Sub 2"},
		},
	}

	results := []session.SearchResult{
		{
			Session:       parent,
			MatchedSubIDs: map[string]bool{"sub-2": true},
		},
	}

	// Matched child automatically surfaces during search even when not manually expanded.
	rows := BuildRows(results, map[string]bool{})
	if len(rows) != 2 || rows[1].Session.ID != "sub-2" || !rows[1].IsSub || !rows[1].IsLastSub {
		t.Fatalf("unexpected auto-expanded search rows: %+v", rows)
	}

	// Manual expansion shows all subsessions with ├─ and └─.
	allRows := BuildRows([]session.SearchResult{{Session: parent}}, map[string]bool{"parent": true})
	if len(allRows) != 3 || allRows[1].IsLastSub || !allRows[2].IsLastSub {
		t.Fatalf("unexpected manually expanded rows: %+v", allRows)
	}
}
