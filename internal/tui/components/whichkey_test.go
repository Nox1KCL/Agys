package components

import (
	"strings"
	"testing"

	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderWhichKeyAlignmentAndBounds(t *testing.T) {
	theme := styles.DefaultTheme()
	entries := []WhichKeyEntry{
		{Desc: "Move selection", Key: "↑/k · ↓/j"},
		{Desc: "Search sessions", Key: "/"},
		{Desc: "Pin / unpin session", Key: "p"},
		{Desc: "Archive / unarchive session", Key: "a"},
		{Desc: "Resume selected session", Key: "Enter"},
		{Desc: "Toggle keybindings menu", Key: "?"},
		{Desc: "Quit", Key: "q"},
	}

	for _, size := range []struct{ w, h int }{
		{55, 18},
		{80, 24},
		{120, 36},
	} {
		box, offset := RenderWhichKey(entries, 0, size.w, size.h, theme)
		if offset != 0 {
			t.Fatalf("expected offset=0 for %dx%d, got %d", size.w, size.h, offset)
		}
		lines := strings.Split(box, "\n")
		if len(lines) > size.h {
			t.Fatalf("%dx%d which-key height %d exceeded terminal height", size.w, size.h, len(lines))
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw > size.w {
				t.Fatalf("%dx%d which-key line %d width %d exceeded terminal width (%q)", size.w, size.h, i, lw, line)
			}
		}
	}

	// Verify description precedes key (left description, right key).
	box, _ := RenderWhichKey(entries, 0, 100, 30, theme)
	for _, line := range strings.Split(box, "\n") {
		if strings.Contains(line, "Search sessions") {
			descIdx := strings.Index(line, "Search sessions")
			keyIdx := strings.LastIndex(line, "/")
			if descIdx < 0 || keyIdx <= descIdx {
				t.Fatalf("expected description on left and key on right in %q", line)
			}
		}
	}
}

func TestOverlayBottomCenterPreservesCanvasSize(t *testing.T) {
	theme := styles.DefaultTheme()
	bg := lipgloss.Place(100, 28, lipgloss.Center, lipgloss.Center, "╭────╮\n│ bg │\n╰────╯")
	fg, _ := RenderWhichKey([]WhichKeyEntry{
		{Desc: "Search sessions", Key: "/"},
		{Desc: "Resume session", Key: "Enter"},
	}, 0, 100, 28, theme)

	out := OverlayBottomCenter(bg, fg, 100, 28)
	lines := strings.Split(out, "\n")
	if len(lines) != 28 {
		t.Fatalf("expected 28 canvas lines, got %d", len(lines))
	}
	for i, line := range lines {
		if lw := lipgloss.Width(line); lw > 100 {
			t.Fatalf("line %d exceeded canvas width 100: got %d (%q)", i, lw, line)
		}
	}
	if !strings.Contains(out, "Keybindings") || !strings.Contains(out, "Search sessions") {
		t.Fatalf("expected overlaid which-key text in output")
	}
}
