package components

import (
	"strings"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func TestFormatAccountUsageRespectsWidthAcrossLayouts(t *testing.T) {
	now := time.Date(2026, 11, 13, 13, 3, 0, 0, time.UTC)
	usage := session.AccountUsage{
		PlanTier:  "Pro",
		FetchedAt: now.Add(-2 * time.Minute),
		Source:    "live",
		Groups: []session.QuotaGroup{
			{
				Name:      "Gemini Models",
				WarmColor: false,
				Buckets: []session.QuotaBucket{
					{
						Label:             "Weekly Limit",
						RemainingFraction: 0.61,
						RemainingPercent:  61,
						ResetTime:         "2026-11-14T16:24:00Z",
					},
					{
						Label:             "5-Hour Limit",
						RemainingFraction: 0.78,
						RemainingPercent:  78,
						ResetTime:         "2026-11-13T15:51:00Z",
					},
				},
			},
			{
				Name:      "Claude and GPT models",
				WarmColor: true,
				Buckets: []session.QuotaBucket{
					{
						Label:             "Weekly Limit",
						RemainingFraction: 0.12,
						RemainingPercent:  12,
						ResetTime:         "2026-11-19T14:24:00Z",
					},
				},
			},
		},
	}

	theme := styles.DefaultANSIFallbackTheme()

	for _, width := range []int{24, 34, 48, 56, 72} {
		out := FormatAccountUsage(usage, false, false, "", width, now, theme)
		if !strings.Contains(out, "Quota Limits") || !strings.Contains(out, "Pro") {
			t.Fatalf("width %d missing heading/tier:\n%s", width, out)
		}
		if !strings.Contains(out, "Gemini Models") || !strings.Contains(out, "Claude and GPT models") {
			t.Fatalf("width %d missing model groups:\n%s", width, out)
		}
		if !strings.Contains(out, "61%") || !strings.Contains(out, "12%") {
			t.Fatalf("width %d missing percentages:\n%s", width, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Fatalf("width %d exceeded: got %d in line %q", width, w, line)
			}
		}
	}
}

func TestFormatAccountUsageEmptyAndErrorStates(t *testing.T) {
	now := time.Date(2026, 11, 13, 13, 3, 0, 0, time.UTC)
	theme := styles.DefaultANSIFallbackTheme()

	loadingView := FormatAccountUsage(session.AccountUsage{}, true, true, "", 40, now, theme)
	if !strings.Contains(loadingView, "Loading quota limits...") {
		t.Fatalf("expected loading message, got:\n%s", loadingView)
	}

	errView := FormatAccountUsage(session.AccountUsage{}, false, false, "agy not in PATH", 40, now, theme)
	if !strings.Contains(errView, "Press U") || !strings.Contains(errView, "agy not in PATH") {
		t.Fatalf("expected helpful retry hint + error, got:\n%s", errView)
	}
}
