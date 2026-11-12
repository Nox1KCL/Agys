package components

import (
	"strings"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func TestFormatPreviewRespectsWidthAndSections(t *testing.T) {
	theme := styles.DefaultTheme()
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)

	s := session.Session{
		ID:          "7772dffa-a2df-45d4-862e-ae9b8c25d36e",
		Title:       "Long session title with Unicode — and right-hand preview pane",
		WorkDir:     "/home/nox/IT-projects/VeryLongProjectName/Subdirectory/Component",
		FirstPrompt: "How should bufio be structured in Go for network stream protocols?",
		LastPrompt:  "What happens if each request handler creates fresh readers and writers?",
		UpdatedAt:   now.Add(-3 * time.Hour),
		Pinned:      true,
		KnowledgeItems: []session.KnowledgeItem{
			{Title: "Go Bufio Patterns", Summary: "Always call Flush() on bufio.Writer before returning."},
		},
		SubSessions: []session.Session{
			{ID: "sub-1", Title: "Codebase Reader", SubagentType: "research"},
		},
	}
	art := session.SessionArtifacts{
		TaskMarkdown: "- [x] Core logic\n- [ ] Preview UI",
		TaskDone:     1,
		TaskTotal:    2,
		PlanMarkdown: "## Implementation\nSplit left list and right preview panes cleanly.",
		Walkthrough:  "Verified with go test ./...",
	}

	for _, width := range []int{28, 46, 64} {
		out := FormatPreview(s, art, width, now, theme)
		for i, line := range strings.Split(out, "\n") {
			if lw := lipgloss.Width(line); lw > width {
				t.Fatalf("width=%d line %d exceeded limit: got %d (%q)", width, i, lw, line)
			}
		}
	}

	out := FormatPreview(s, art, 60, now, theme)
	for _, want := range []string{
		"★ Pinned",
		"First Prompt",
		"Last Prompt",
		"Knowledge Items",
		"task.md (1/2)",
		"implementation_plan.md",
		"walkthrough.md",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected preview to contain %q, got:\n%s", want, out)
		}
	}
}
