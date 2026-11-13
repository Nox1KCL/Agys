package components

import (
	"fmt"
	"strings"

	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmDialog configures the floating confirmation modal for moving a session
// to Trash or permanently deleting it.
type ConfirmDialog struct {
	Permanent      bool
	Title          string
	SessionID      string
	ParentTitle    string
	SubCount       int
	TrashPath      string
	ConfirmFocused bool
}

// RenderConfirmDialog renders a bounded floating confirmation box sized to fit within termW x termH.
func RenderConfirmDialog(dlg ConfirmDialog, termW, termH int, theme styles.Theme) string {
	if termW <= 0 {
		termW = 80
	}
	if termH <= 0 {
		termH = 24
	}

	minW := min(max(20, termW-2), 46)
	maxW := min(max(20, termW-2), 62)
	boxW := termW * 58 / 100
	if boxW < minW {
		boxW = minW
	}
	if boxW > maxW {
		boxW = maxW
	}
	innerW := max(12, boxW-6)

	heading := "Move session to Trash?"
	headingStyle := theme.SearchPrompt
	if dlg.Permanent {
		heading = "Permanently delete session?"
		headingStyle = theme.ErrorText
	}

	headerLine := renderConfirmHeader(heading, headingStyle, innerW, theme)
	dividerLine := theme.Divider.Render(strings.Repeat("─", innerW))

	var body []string
	title := strings.TrimSpace(dlg.Title)
	if title == "" {
		title = "(untitled)"
	}
	for _, line := range WrapPlain(title, innerW) {
		body = append(body, theme.PreviewHeading.Render(line))
	}

	if parent := strings.TrimSpace(dlg.ParentTitle); parent != "" {
		body = append(body, theme.PreviewLabel.Render("Subsession of: ")+theme.PreviewMuted.Render(TruncateWidth(parent, max(1, innerW-15))))
	}
	if dlg.SessionID != "" {
		body = append(body, theme.PreviewLabel.Render("UUID: ")+theme.PreviewMuted.Render(TruncateWidth(dlg.SessionID, max(1, innerW-6))))
	}
	if dlg.SubCount > 0 {
		verb := "trashed"
		if dlg.Permanent {
			verb = "deleted"
		}
		plural := ""
		if dlg.SubCount != 1 {
			plural = "s"
		}
		subNotice := fmt.Sprintf("Includes %d subsession%s (also %s)", dlg.SubCount, plural, verb)
		subStyle := theme.PinnedBadge
		if dlg.Permanent {
			subStyle = theme.ErrorText
		}
		for _, line := range WrapPlain(subNotice, innerW) {
			body = append(body, subStyle.Render(line))
		}
	}

	body = append(body, "")

	if dlg.Permanent {
		for _, line := range WrapPlain("This cannot be undone. Session files will be permanently removed.", innerW) {
			body = append(body, theme.ErrorText.Render(line))
		}
	} else {
		dest := strings.TrimSpace(dlg.TrashPath)
		if dest == "" {
			dest = "~/.local/share/Trash"
		}
		for _, line := range WrapPlain("Destination: "+dest, innerW) {
			body = append(body, theme.PreviewMuted.Render(line))
		}
	}

	buttonsLine := renderConfirmButtons(dlg.Permanent, dlg.ConfirmFocused, innerW, theme)
	hintLine := theme.Footer.Render(TruncateWidth("←/→ switch · y confirm · n/Esc cancel", innerW))

	// Assemble lines while preserving header, buttons, and footer hint on compact heights.
	maxInnerH := max(5, termH-4)
	lines := make([]string, 0, maxInnerH)
	lines = append(lines, headerLine, dividerLine)

	fixedTail := []string{"", buttonsLine, hintLine}
	bodyBudget := maxInnerH - len(lines) - len(fixedTail)
	if bodyBudget < 1 {
		fixedTail = []string{buttonsLine}
		bodyBudget = max(0, maxInnerH-len(lines)-len(fixedTail))
	}

	if len(body) > bodyBudget {
		trimmed := make([]string, 0, len(body))
		for _, b := range body {
			if b != "" {
				trimmed = append(trimmed, b)
			}
		}
		body = trimmed
	}
	if len(body) > bodyBudget {
		body = body[:bodyBudget]
	}

	lines = append(lines, body...)
	lines = append(lines, fixedTail...)

	innerH := len(lines)
	boxH := min(max(7, termH-2), innerH+4)
	if innerH+4 > boxH {
		innerH = max(3, boxH-4)
		if len(lines) > innerH {
			lines = lines[:innerH]
		}
	}

	borderStyle := theme.ModalBorderActive
	if dlg.Permanent {
		borderStyle = theme.ModalBorderDanger
	}

	return borderStyle.
		Width(innerW + 4).
		Height(innerH + 2).
		MaxWidth(boxW).
		MaxHeight(boxH).
		Render(strings.Join(lines, "\n"))
}

func renderConfirmHeader(title string, titleStyle lipgloss.Style, innerW int, theme styles.Theme) string {
	hint := "Esc"
	hintW := lipgloss.Width(hint)
	if innerW < hintW+8 {
		return titleStyle.Render(TruncateWidth(title, innerW))
	}
	left := titleStyle.Render(TruncateWidth(title, innerW-hintW-1))
	gap := max(1, innerW-lipgloss.Width(left)-hintW)
	return left + strings.Repeat(" ", gap) + theme.PreviewMuted.Render(hint)
}

func renderConfirmButtons(permanent, confirmFocused bool, innerW int, theme styles.Theme) string {
	confirmFull := "[ Move to Trash (y) ]"
	confirmShort := "[ Trash (y) ]"
	if permanent {
		confirmFull = "[ Delete permanently (y) ]"
		confirmShort = "[ Delete (y) ]"
	}
	cancelFull := "[ Cancel (n) ]"
	cancelShort := "[ Cancel ]"

	confirmLabel := confirmFull
	cancelLabel := cancelFull
	sep := "   "

	if lipgloss.Width(confirmLabel)+len(sep)+lipgloss.Width(cancelLabel) > innerW {
		confirmLabel = confirmShort
		cancelLabel = cancelFull
		sep = "  "
	}
	if lipgloss.Width(confirmLabel)+len(sep)+lipgloss.Width(cancelLabel) > innerW {
		cancelLabel = cancelShort
		sep = " "
	}

	confirmStyle := theme.PreviewMuted
	if permanent {
		confirmStyle = theme.ErrorText
	}
	cancelStyle := theme.PreviewMuted

	if confirmFocused {
		confirmStyle = theme.SelectedRow
	} else {
		cancelStyle = theme.SelectedRow
	}

	row := confirmStyle.Render(confirmLabel) + sep + cancelStyle.Render(cancelLabel)
	if lipgloss.Width(row) > innerW {
		return TruncateWidth(row, innerW)
	}
	return row
}
