package components

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

// FormatAccountUsage renders Antigravity account quota limits for the side pane viewport.
func FormatAccountUsage(usage session.AccountUsage, loading, refreshing bool, errMsg string, width int, now time.Time, theme styles.Theme) string {
	if width <= 0 {
		width = 36
	}
	if now.IsZero() {
		now = time.Now()
	}

	var lines []string

	// Top section heading ("Quota Limits" + optional tier badge like "Pro").
	headingText := "Quota Limits"
	headingStyled := theme.PreviewHeading.Render(TruncateWidth(headingText, width))
	if tier := strings.TrimSpace(usage.PlanTier); tier != "" {
		tierRendered := theme.RowBadge.Render(tier)
		tierW := lipgloss.Width(tierRendered)
		if width >= lipgloss.Width(headingText)+tierW+2 {
			leftMax := width - tierW - 1
			headingLeft := theme.PreviewHeading.Render(TruncateWidth(headingText, leftMax))
			gap := max(1, width-lipgloss.Width(headingLeft)-tierW)
			headingStyled = headingLeft + strings.Repeat(" ", gap) + tierRendered
		}
	}
	lines = append(lines, headingStyled)

	if status := accountStatusLine(usage, loading, refreshing, errMsg, now); status != "" {
		style := theme.PreviewMuted
		if errMsg != "" && len(usage.Groups) == 0 {
			style = theme.ErrorText
		}
		for _, wrapped := range WrapPlain(status, width) {
			lines = append(lines, style.Render(TruncateWidth(wrapped, width)))
		}
	}

	if len(usage.Groups) == 0 {
		lines = append(lines, "")
		if loading || refreshing {
			lines = append(lines, theme.PreviewMuted.Render(TruncateWidth("Loading quota limits...", width)))
		} else {
			for _, wrapped := range WrapPlain("No quota data cached yet. Press U to query live limits from agy.", width) {
				lines = append(lines, theme.PreviewMuted.Render(TruncateWidth(wrapped, width)))
			}
		}
		return strings.Join(lines, "\n")
	}

	for _, grp := range usage.Groups {
		lines = append(lines, "")
		dotStyle := theme.QuotaCyan
		if grp.WarmColor {
			dotStyle = theme.QuotaWarm
		}
		dot := dotStyle.Render("●")
		nameMax := max(1, width-lipgloss.Width(dot)-1)
		groupHeader := dot + " " + theme.PreviewBody.Render(TruncateWidth(grp.Name, nameMax))
		lines = append(lines, groupHeader)

		for _, bucket := range grp.Buckets {
			lines = append(lines, renderQuotaBucket(bucket, grp.WarmColor, width, now, theme)...)
		}
	}

	return strings.Join(lines, "\n")
}

func accountStatusLine(usage session.AccountUsage, loading, refreshing bool, errMsg string, now time.Time) string {
	var parts []string
	switch {
	case refreshing || loading:
		parts = append(parts, "Refreshing quotas...")
	case !usage.FetchedAt.IsZero():
		parts = append(parts, "Updated "+formatUsageAge(usage.FetchedAt, now))
	}

	if usage.Source == "omarchy-cache" && len(usage.Groups) > 0 {
		parts = append(parts, "fallback summary")
	} else if usage.Stale && !refreshing && !loading && len(usage.Groups) > 0 {
		parts = append(parts, "cached")
	}

	if cleanErr := strings.TrimSpace(errMsg); cleanErr != "" && !refreshing {
		parts = append(parts, "refresh error: "+cleanErr)
	}
	return strings.Join(parts, " · ")
}

func formatUsageAge(fetchedAt, now time.Time) string {
	diff := now.Sub(fetchedAt)
	if diff < 0 || diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
}

func renderQuotaBucket(bucket session.QuotaBucket, warmGroup bool, width int, now time.Time, theme styles.Theme) []string {
	label := strings.TrimSpace(bucket.Label)
	if label == "" {
		label = "Limit"
	}

	pct := bucket.RemainingPercent
	pctText := fmt.Sprintf("%d%% remaining", pct)
	resetText := session.FormatQuotaReset(bucket.ResetTime, now)

	barStyle := theme.QuotaCyan
	if warmGroup {
		barStyle = theme.QuotaWarm
	}
	pctStyle := theme.PreviewBody
	switch {
	case pct <= 15:
		barStyle = theme.QuotaDanger
		pctStyle = theme.QuotaDanger
	case pct <= 30:
		barStyle = theme.QuotaWarn
		pctStyle = theme.QuotaWarn
	}

	labelStyled := theme.PreviewLabel.Render(label)
	pctStyled := pctStyle.Render(pctText)
	resetStyled := theme.PreviewMuted.Render(resetText)

	var out []string

	labelW := lipgloss.Width(label)
	pctW := lipgloss.Width(pctText)
	resetW := lipgloss.Width(resetText)

	// Match the horizontal "Weekly Limit   Resets in 1d 3h · 4:24PM  61% remaining"
	// row when width allows; stack reset time cleanly beneath on narrow panes.
	switch {
	case resetW > 0 && labelW+2+resetW+1+pctW <= width:
		right := resetStyled + " " + pctStyled
		gap := width - labelW - lipgloss.Width(right)
		out = append(out, labelStyled+strings.Repeat(" ", max(1, gap))+right)
	case labelW+1+pctW <= width:
		gap := width - labelW - pctW
		out = append(out, labelStyled+strings.Repeat(" ", max(1, gap))+pctStyled)
		if resetText != "" {
			out = append(out, theme.PreviewMuted.Render(TruncateWidth(resetText, width)))
		}
	default:
		out = append(out, theme.PreviewLabel.Render(TruncateWidth(label, width)))
		meta := pctText
		if resetText != "" {
			meta = pctText + " · " + resetText
		}
		out = append(out, pctStyle.Render(TruncateWidth(meta, width)))
	}

	barWidth := max(4, width)
	filled := int(math.Round(bucket.RemainingFraction * float64(barWidth)))
	if bucket.RemainingFraction > 0 && filled == 0 {
		filled = 1
	}
	if filled > barWidth {
		filled = barWidth
	}
	if bucket.RemainingFraction < 1 && filled == barWidth && barWidth > 1 {
		filled = barWidth - 1
	}
	empty := barWidth - filled

	bar := barStyle.Render(strings.Repeat("━", filled)) + theme.QuotaTrack.Render(strings.Repeat("━", empty))
	out = append(out, bar)

	return out
}
