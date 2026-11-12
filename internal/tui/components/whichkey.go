package components

import (
	"fmt"
	"strings"

	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// WhichKeyEntry represents a single row inside the floating keybindings overlay.
type WhichKeyEntry struct {
	Desc string
	Key  string
}

// WhichKeyLayout holds calculated outer and inner dimensions for the which-key popup.
type WhichKeyLayout struct {
	BoxW   int
	BoxH   int
	InnerW int
	InnerH int
	BodyH  int
}

// CalcWhichKeyLayout computes clamped dimensions for the which-key floating window.
func CalcWhichKeyLayout(termW, termH, entryCount int) WhichKeyLayout {
	if termW <= 0 {
		termW = 80
	}
	if termH <= 0 {
		termH = 24
	}

	minW := min(max(12, termW-2), 44)
	maxW := min(max(12, termW-2), 62)
	boxW := termW * 60 / 100
	if boxW < minW {
		boxW = minW
	}
	if boxW > maxW {
		boxW = maxW
	}
	innerW := max(10, boxW-6)

	// Border (2) + vertical padding (2) + header (1) + divider (1) = 6 lines around body.
	desiredH := max(1, entryCount) + 6
	maxH := max(7, termH-2)
	boxH := min(desiredH, maxH)
	innerH := max(3, boxH-4)
	bodyH := max(1, innerH-2)

	return WhichKeyLayout{
		BoxW:   boxW,
		BoxH:   boxH,
		InnerW: innerW,
		InnerH: innerH,
		BodyH:  bodyH,
	}
}

// RenderWhichKey renders a LazyVim/Yazi-style floating keybindings box with descriptions on the left
// and hotkeys aligned on the right.
func RenderWhichKey(entries []WhichKeyEntry, offset, termW, termH int, theme styles.Theme) (rendered string, clampedOffset int) {
	lay := CalcWhichKeyLayout(termW, termH, len(entries))

	maxOffset := max(0, len(entries)-lay.BodyH)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	headerRight := "Esc / ? close"
	if len(entries) > lay.BodyH {
		headerRight = fmt.Sprintf("↑↓ scroll · Esc/? close (%d-%d/%d)", offset+1, min(len(entries), offset+lay.BodyH), len(entries))
	}

	title := "Keybindings"
	hintW := lipgloss.Width(headerRight)
	if lay.InnerW < hintW+lipgloss.Width(title)+2 {
		headerRight = "? / Esc"
		hintW = lipgloss.Width(headerRight)
		if lay.InnerW < hintW+lipgloss.Width(title)+2 {
			headerRight = ""
			hintW = 0
		}
	}

	var headerLine string
	if hintW == 0 {
		headerLine = theme.SearchPrompt.Render(TruncateWidth(title, lay.InnerW))
	} else {
		leftTitle := theme.SearchPrompt.Render(TruncateWidth(title, lay.InnerW-hintW-1))
		gap := max(1, lay.InnerW-lipgloss.Width(leftTitle)-hintW)
		headerLine = leftTitle + strings.Repeat(" ", gap) + theme.PreviewMuted.Render(headerRight)
	}

	lines := make([]string, 0, lay.InnerH)
	lines = append(lines, headerLine)
	lines = append(lines, theme.Divider.Render(strings.Repeat("─", lay.InnerW)))

	end := min(len(entries), offset+lay.BodyH)
	for i := offset; i < end; i++ {
		lines = append(lines, renderWhichKeyRow(entries[i], lay.InnerW, theme))
	}
	for i := end - offset; i < lay.BodyH; i++ {
		lines = append(lines, "")
	}

	box := theme.ModalBorderActive.
		Width(lay.InnerW + 4).
		Height(lay.InnerH + 2).
		MaxWidth(lay.BoxW).
		MaxHeight(lay.BoxH).
		Render(strings.Join(lines, "\n"))

	return box, offset
}

func renderWhichKeyRow(entry WhichKeyEntry, width int, theme styles.Theme) string {
	keyText := strings.TrimSpace(entry.Key)
	keyW := lipgloss.Width(keyText)
	if keyW >= width-3 {
		return theme.PreviewBody.Render(TruncateWidth(entry.Desc, width))
	}

	descBudget := max(1, width-keyW-2)
	descText := TruncateWidth(entry.Desc, descBudget)
	gap := max(2, width-lipgloss.Width(descText)-keyW)

	return theme.PreviewBody.Render(descText) + strings.Repeat(" ", gap) + theme.SearchPrompt.Render(keyText)
}

// OverlayBottomCenter composites a floating popup window on top of a terminal-sized background string,
// anchored bottom-center like LazyVim/Yazi overlays while keeping every line strictly ANSI-safe.
func OverlayBottomCenter(bg, fg string, termW, termH int) string {
	if termW <= 0 || termH <= 0 {
		return fg
	}

	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	if len(bgLines) == 0 || len(fgLines) == 0 {
		return fg
	}

	popH := len(fgLines)
	popW := 0
	for _, l := range fgLines {
		if w := lipgloss.Width(l); w > popW {
			popW = w
		}
	}

	startX := max(0, (termW-popW)/2)
	startY := termH - popH - 2
	if startY < 1 || popH >= termH-4 {
		startY = max(0, (len(bgLines)-popH)/2)
	}

	for i, fgLine := range fgLines {
		y := startY + i
		if y < 0 || y >= len(bgLines) {
			continue
		}

		bgLine := bgLines[y]
		bgW := lipgloss.Width(bgLine)
		fgW := lipgloss.Width(fgLine)
		if fgW < popW {
			fgLine += strings.Repeat(" ", popW-fgW)
			fgW = popW
		}

		left := ansi.Cut(bgLine, 0, startX)
		if lw := lipgloss.Width(left); lw < startX {
			left += strings.Repeat(" ", startX-lw)
		}

		rightStart := startX + fgW
		right := ""
		if rightStart < bgW {
			right = ansi.TruncateLeft(bgLine, rightStart, "")
			maxRightW := max(0, bgW-rightStart)
			if rw := lipgloss.Width(right); rw > maxRightW {
				right = " " + ansi.TruncateLeft(bgLine, rightStart+1, "")
				if lipgloss.Width(right) > maxRightW {
					right = ansi.Cut(right, 0, maxRightW)
				}
			}
		}

		bgLines[y] = left + "\x1b[0m" + fgLine + "\x1b[0m" + right
	}

	return strings.Join(bgLines, "\n")
}
