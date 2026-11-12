package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/Nox1KCL/Agys/internal/utils"
	"github.com/charmbracelet/lipgloss"
)

// FormatPreview formats session metadata, user prompts, Knowledge Items, and brain artifacts
// wrapped strictly within width terminal columns.
func FormatPreview(s session.Session, art session.SessionArtifacts, width int, now time.Time, theme styles.Theme) string {
	if width <= 0 {
		return ""
	}
	if width < 12 {
		width = 12
	}

	var out []string

	// Title and status flags.
	var statusTags []string
	if s.Pinned {
		statusTags = append(statusTags, "★ Pinned")
	}
	if s.Archived {
		statusTags = append(statusTags, "Archived")
	}
	if s.ParentID != "" {
		statusTags = append(statusTags, "Subsession")
	}
	if len(statusTags) > 0 {
		out = appendWrappedStyled(&out, strings.Join(statusTags, " · "), width, theme.PinnedBadge)
	}

	title := strings.TrimSpace(s.Title)
	if title == "" {
		title = "(untitled)"
	}
	out = appendWrappedStyled(&out, title, width, theme.PreviewHeading)
	out = append(out, "")

	// Metadata section.
	if !s.UpdatedAt.IsZero() {
		dateLine := fmt.Sprintf("%s (%s)", utils.RelativeTime(s.UpdatedAt, now), s.UpdatedAt.Local().Format("2006-01-02 15:04"))
		out = appendLabeledField(&out, "Updated: ", dateLine, width, theme)
	}
	dir := "—"
	if s.WorkDir != "" {
		dir = utils.ShortenHome(s.WorkDir)
	}
	out = appendLabeledField(&out, "Directory: ", dir, width, theme)
	out = appendLabeledField(&out, "UUID: ", s.ID, width, theme)

	if len(s.SubSessions) > 0 {
		out = appendLabeledField(&out, "Subsessions: ", fmt.Sprintf("%d", len(s.SubSessions)), width, theme)
		for _, sub := range s.SubSessions {
			label := sub.Title
			if sub.SubagentType != "" {
				label = fmt.Sprintf("[%s] %s", sub.SubagentType, label)
			}
			out = appendWrappedStyled(&out, "  • "+label, width, theme.PreviewMuted)
		}
	}

	hasRichContent := false

	// User prompts.
	if fp := strings.TrimSpace(s.FirstPrompt); fp != "" {
		hasRichContent = true
		if lp := strings.TrimSpace(s.LastPrompt); lp != "" && lp != fp {
			out = appendSection(&out, "First Prompt", fp, width, theme)
			out = appendSection(&out, "Last Prompt", lp, width, theme)
		} else {
			out = appendSection(&out, "User Prompt", fp, width, theme)
		}
	}

	// Knowledge Items.
	if len(s.KnowledgeItems) > 0 {
		hasRichContent = true
		var kiLines []string
		for i, ki := range s.KnowledgeItems {
			if i > 0 {
				kiLines = append(kiLines, "")
			}
			kiLines = append(kiLines, "• "+ki.Title)
			if ki.Summary != "" {
				kiLines = append(kiLines, ki.Summary)
			}
		}
		out = appendSection(&out, "Knowledge Items", strings.Join(kiLines, "\n"), width, theme)
	}

	// Brain markdown artifacts.
	if art.TaskMarkdown != "" {
		hasRichContent = true
		heading := "task.md"
		if art.TaskTotal > 0 {
			heading = fmt.Sprintf("task.md (%d/%d)", art.TaskDone, art.TaskTotal)
		}
		out = appendSection(&out, heading, art.TaskMarkdown, width, theme)
	}
	if art.PlanMarkdown != "" {
		hasRichContent = true
		out = appendSection(&out, "implementation_plan.md", art.PlanMarkdown, width, theme)
	}
	if art.Walkthrough != "" {
		hasRichContent = true
		out = appendSection(&out, "walkthrough.md", art.Walkthrough, width, theme)
	}

	if !hasRichContent {
		out = append(out, "")
		out = appendWrappedStyled(&out, "No saved prompts, Knowledge Items, or artifacts for this session.", width, theme.PreviewMuted)
	}

	return strings.Join(out, "\n")
}

func appendSection(out *[]string, heading, body string, width int, theme styles.Theme) []string {
	*out = append(*out, "")
	*out = append(*out, theme.PreviewHeading.Render(TruncateWidth("─ "+heading, width)))
	*out = appendWrappedStyled(out, body, width, theme.PreviewBody)
	return *out
}

func appendLabeledField(out *[]string, label, value string, width int, theme styles.Theme) []string {
	labelW := lipgloss.Width(label)
	valBudget := width - labelW
	if valBudget < 8 {
		*out = appendWrappedStyled(out, label+value, width, theme.PreviewBody)
		return *out
	}

	wrapped := WrapPlain(value, valBudget)
	if len(wrapped) == 0 {
		*out = append(*out, theme.PreviewLabel.Render(label))
		return *out
	}
	*out = append(*out, theme.PreviewLabel.Render(label)+theme.PreviewBody.Render(wrapped[0]))
	indent := strings.Repeat(" ", labelW)
	for _, cont := range wrapped[1:] {
		*out = append(*out, indent+theme.PreviewBody.Render(cont))
	}
	return *out
}

func appendWrappedStyled(out *[]string, text string, width int, st lipgloss.Style) []string {
	for _, line := range WrapPlain(text, width) {
		*out = append(*out, st.Render(line))
	}
	return *out
}

// WrapPlain wraps plain multi-line text so every returned line fits within width terminal cells.
func WrapPlain(text string, width int) []string {
	if width <= 0 {
		return nil
	}

	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))

	for _, raw := range rawLines {
		trimmedRight := strings.TrimRight(raw, " \t")
		if trimmedRight == "" {
			lines = append(lines, "")
			continue
		}

		// Preserve leading indentation (for markdown lists/code lines).
		indentLen := 0
		for indentLen < len(trimmedRight) && trimmedRight[indentLen] == ' ' {
			indentLen++
		}
		if indentLen >= width-4 {
			indentLen = 0
		}
		indent := trimmedRight[:indentLen]
		body := strings.TrimSpace(trimmedRight[indentLen:])
		if body == "" {
			lines = append(lines, "")
			continue
		}

		words := strings.Fields(body)
		cur := indent
		curW := len(indent)

		for _, word := range words {
			wordW := lipgloss.Width(word)
			if curW == len(indent) {
				cur, curW = appendWordFit(&lines, indent, word, wordW, width)
				continue
			}

			if curW+1+wordW <= width {
				cur += " " + word
				curW += 1 + wordW
				continue
			}

			lines = append(lines, cur)
			cur, curW = appendWordFit(&lines, indent, word, wordW, width)
		}

		if cur != "" {
			lines = append(lines, cur)
		}
	}

	return lines
}

func appendWordFit(lines *[]string, indent, word string, wordW, width int) (string, int) {
	if len(indent)+wordW <= width {
		cur := indent + word
		return cur, len(indent) + wordW
	}
	chunks := breakLongToken(word, max(1, width-len(indent)))
	for _, part := range chunks[:len(chunks)-1] {
		*lines = append(*lines, indent+part)
	}
	last := chunks[len(chunks)-1]
	cur := indent + last
	return cur, lipgloss.Width(cur)
}

func breakLongToken(token string, maxCells int) []string {
	if maxCells <= 0 || lipgloss.Width(token) <= maxCells {
		return []string{token}
	}
	var chunks []string
	var b strings.Builder
	used := 0
	for _, r := range token {
		rw := lipgloss.Width(string(r))
		if used+rw > maxCells && used > 0 {
			chunks = append(chunks, b.String())
			b.Reset()
			used = 0
		}
		b.WriteRune(r)
		used += rw
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
}
