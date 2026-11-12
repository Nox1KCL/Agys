package components

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/Nox1KCL/Agys/internal/utils"
	"github.com/charmbracelet/lipgloss"
)

const dateColWidth = 7

// Row represents a single visible entry in the flattened session tree list.
type Row struct {
	Session   session.Session
	ParentID  string
	IsSub     bool
	IsLastSub bool
	HasSubs   bool
	Expanded  bool
}

// BuildRows flattens search results into visible root and expanded subsession rows.
func BuildRows(results []session.SearchResult, expanded map[string]bool) []Row {
	rows := make([]Row, 0, len(results))

	for _, res := range results {
		root := res.Session
		userExpanded := expanded[root.ID]
		visibleSubs := visibleSubsessions(root, userExpanded, res.MatchedSubIDs)

		rows = append(rows, Row{
			Session:  root,
			HasSubs:  len(root.SubSessions) > 0,
			Expanded: len(visibleSubs) > 0,
		})

		for i, sub := range visibleSubs {
			rows = append(rows, Row{
				Session:   sub,
				ParentID:  root.ID,
				IsSub:     true,
				IsLastSub: i == len(visibleSubs)-1,
			})
		}
	}

	return rows
}

func visibleSubsessions(root session.Session, userExpanded bool, matched map[string]bool) []session.Session {
	if len(root.SubSessions) == 0 {
		return nil
	}
	if userExpanded {
		return root.SubSessions
	}
	if len(matched) == 0 {
		return nil
	}

	out := make([]session.Session, 0, len(matched))
	for _, sub := range root.SubSessions {
		if matched[sub.ID] {
			out = append(out, sub)
		}
	}
	return out
}

// RenderRow renders a single row strictly within width terminal columns.
// Layout: [cursor/tree] [date]  [title...]  [dir]
func RenderRow(row Row, selected bool, width int, now time.Time, theme styles.Theme) string {
	if width <= 0 {
		return ""
	}

	prefix := rowPrefix(row, selected)
	prefixW := lipgloss.Width(prefix)

	dateStr := padRight(utils.RelativeTime(row.Session.UpdatedAt, now), dateColWidth)
	if width < prefixW+dateColWidth+6 {
		plain := TruncateWidth(prefix+row.Session.Title, width)
		return applyRowStyle(plain, row, selected, width, theme)
	}

	dirText := ""
	dirGap := 0
	if row.Session.WorkDir != "" && width >= 54 {
		maxDir := width * 28 / 100
		if maxDir < 14 {
			maxDir = 14
		}
		if maxDir > 28 {
			maxDir = 28
		}
		dirText = CompressDir(row.Session.WorkDir, maxDir)
		if dirText != "" {
			dirGap = 2
		}
	}

	dirW := lipgloss.Width(dirText)
	titleBudget := width - prefixW - dateColWidth - 2 - dirW - dirGap
	if titleBudget < 10 && dirW > 0 {
		dirText = ""
		dirW = 0
		dirGap = 0
		titleBudget = width - prefixW - dateColWidth - 2
	}
	if titleBudget < 1 {
		titleBudget = 1
	}

	var badgeBuilder strings.Builder
	if row.Session.Archived {
		badgeBuilder.WriteString(" [archived]")
	}
	if row.HasSubs && !row.Expanded {
		badgeBuilder.WriteString(fmt.Sprintf(" (+%d)", len(row.Session.SubSessions)))
	}
	badge := badgeBuilder.String()
	badgeW := lipgloss.Width(badge)

	rawTitle := row.Session.Title
	if row.IsSub && row.Session.SubagentType != "" {
		rawTitle = fmt.Sprintf("[%s] %s", row.Session.SubagentType, rawTitle)
	}
	if row.Session.Pinned {
		rawTitle = "★ " + rawTitle
	}

	var titlePart string
	if badgeW > 0 && titleBudget > badgeW+4 {
		titlePart = TruncateWidth(rawTitle, titleBudget-badgeW) + badge
	} else {
		badge = ""
		titlePart = TruncateWidth(rawTitle, titleBudget)
	}

	titleW := lipgloss.Width(titlePart)
	spacerW := width - prefixW - dateColWidth - 2 - titleW - dirW
	if spacerW < 0 {
		spacerW = 0
	}
	spacer := strings.Repeat(" ", spacerW)

	if selected || row.IsSub || row.Session.Archived {
		plain := prefix + dateStr + "  " + titlePart + spacer + dirText
		plain = TruncateWidth(plain, width)
		return applyRowStyle(plain, row, selected, width, theme)
	}

	baseTitle := titlePart
	if badge != "" && strings.HasSuffix(titlePart, badge) {
		baseTitle = strings.TrimSuffix(titlePart, badge)
	} else {
		badge = ""
	}

	var titleStyled string
	if row.Session.Pinned && strings.HasPrefix(baseTitle, "★ ") {
		rest := strings.TrimPrefix(baseTitle, "★ ")
		titleStyled = theme.PinnedBadge.Render("★ ") + theme.RowTitle.Render(rest)
	} else {
		titleStyled = theme.RowTitle.Render(baseTitle)
	}
	if badge != "" {
		titleStyled += theme.RowBadge.Render(badge)
	}

	return prefix +
		theme.RowDate.Render(dateStr) +
		"  " +
		titleStyled +
		spacer +
		theme.RowDir.Render(dirText)
}

func rowPrefix(row Row, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}
	if !row.IsSub {
		return cursor
	}
	if row.IsLastSub {
		return cursor + "└─ "
	}
	return cursor + "├─ "
}

func applyRowStyle(plain string, row Row, selected bool, width int, theme styles.Theme) string {
	padded := padRight(plain, width)
	if selected {
		if row.IsSub {
			return theme.SubRowSelected.Render(padded)
		}
		return theme.SelectedRow.Render(padded)
	}
	if row.IsSub {
		return theme.SubRow.Render(padded)
	}
	if row.Session.Archived {
		return theme.ArchivedRow.Render(padded)
	}
	return padded
}

// CompressDir shortens a directory path using "~" and middle elision to fit maxWidth cells.
func CompressDir(dir string, maxWidth int) string {
	if maxWidth <= 0 || dir == "" {
		return ""
	}
	short := utils.ShortenHome(dir)
	if lipgloss.Width(short) <= maxWidth {
		return short
	}

	base := filepath.Base(short)
	prefix := ".../"
	if strings.HasPrefix(short, "~/") {
		prefix = "~/.../"
	}
	compact := prefix + base
	if lipgloss.Width(compact) <= maxWidth {
		return compact
	}
	if lipgloss.Width(".../"+base) <= maxWidth {
		return ".../" + base
	}
	return TruncateWidth(base, maxWidth)
}

// TruncateWidth sharply truncates s to at most maxWidth terminal cells, appending "...".
func TruncateWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	const ellipsis = "..."
	if maxWidth <= len(ellipsis) {
		return cutToCells(s, maxWidth)
	}

	return cutToCells(s, maxWidth-len(ellipsis)) + ellipsis
}

func cutToCells(s string, maxCells int) string {
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > maxCells {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
