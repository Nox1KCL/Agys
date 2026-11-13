package components

import (
	"strings"
	"testing"

	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderConfirmDialogAndOverlayCenterBounds(t *testing.T) {
	theme := styles.DefaultTheme()

	for _, permanent := range []bool{false, true} {
		dlg := ConfirmDialog{
			Permanent:      permanent,
			Title:          "Refactor authentication middleware across API routes with very long title wrapping cleanly",
			SessionID:      "11111111-1111-4111-8111-111111111111",
			SubCount:       2,
			TrashPath:      "~/.local/share/Trash",
			ConfirmFocused: !permanent,
		}

		for _, size := range []struct{ w, h int }{
			{55, 18},
			{80, 24},
			{120, 36},
		} {
			box := RenderConfirmDialog(dlg, size.w, size.h, theme)
			lines := strings.Split(box, "\n")
			if len(lines) > size.h {
				t.Fatalf("permanent=%v %dx%d confirm height %d exceeded terminal height", permanent, size.w, size.h, len(lines))
			}
			for i, line := range lines {
				if lw := lipgloss.Width(line); lw > size.w {
					t.Fatalf("permanent=%v %dx%d line %d exceeded width %d (%q)", permanent, size.w, size.h, i, lw, line)
				}
			}

			bg := lipgloss.Place(size.w, size.h, lipgloss.Center, lipgloss.Center, "╭────╮\n│ bg │\n╰────╯")
			out := OverlayCenter(bg, box, size.w, size.h)
			outLines := strings.Split(out, "\n")
			if len(outLines) != size.h {
				t.Fatalf("expected %d overlaid lines, got %d", size.h, len(outLines))
			}
			for i, line := range outLines {
				if lw := lipgloss.Width(line); lw > size.w {
					t.Fatalf("overlaid line %d exceeded %d: got %d (%q)", i, size.w, lw, line)
				}
			}
			if !strings.Contains(out, "Cancel") {
				t.Fatalf("expected Cancel button visible in confirm overlay at %dx%d:\n%s", size.w, size.h, out)
			}
		}
	}
}
