package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
)

// UsageCacheTTL controls how long cached quota limits are considered fresh
// before Agys triggers a background refresh from the agy CLI.
const UsageCacheTTL = 120 * time.Second

// QuotaBucket represents a single rolling usage window (for example Weekly or 5-Hour).
type QuotaBucket struct {
	ID                string  `json:"id,omitempty"`
	Name              string  `json:"name,omitempty"`
	Label             string  `json:"label"`
	Window            string  `json:"window,omitempty"`
	RemainingFraction float64 `json:"remaining_fraction"`
	RemainingPercent  int     `json:"remaining_percent"`
	ResetTime         string  `json:"reset_time,omitempty"`
	Description       string  `json:"description,omitempty"`
}

// QuotaGroup represents a family of models sharing quota limits.
type QuotaGroup struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	WarmColor   bool          `json:"warm_color,omitempty"`
	Buckets     []QuotaBucket `json:"buckets"`
}

// AccountUsage holds parsed Antigravity account quota limits and metadata.
type AccountUsage struct {
	PlanTier  string       `json:"plan_tier,omitempty"`
	Groups    []QuotaGroup `json:"groups"`
	FetchedAt time.Time    `json:"fetched_at,omitempty"`
	Source    string       `json:"source,omitempty"`
	Stale     bool         `json:"stale,omitempty"`
}

type usageEnvelope struct {
	Command *struct {
		Data *rawUsageData `json:"data"`
	} `json:"command"`
	Data *rawUsageData `json:"data"`
}

type rawUsageData struct {
	Plan     string          `json:"plan,omitempty"`
	Tier     string          `json:"tier,omitempty"`
	PlanTier string          `json:"plan_tier,omitempty"`
	Groups   []rawQuotaGroup `json:"groups"`
}

type rawQuotaGroup struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Buckets     []rawQuotaBucket `json:"buckets"`
}

type rawQuotaBucket struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Window            string   `json:"window"`
	RemainingFraction *float64 `json:"remaining_fraction"`
	RemainingPercent  *float64 `json:"remaining_percent"`
	ResetTime         string   `json:"reset_time"`
	Description       string   `json:"description"`
}

type omarchyAgentUsage struct {
	TierLabel       string `json:"tierLabel"`
	UpdatedAt       string `json:"updatedAt"`
	LimitsStale     bool   `json:"limitsStale"`
	LimitsFetchedAt int64  `json:"limitsFetchedAt"`
	Limits          []struct {
		Label    string  `json:"label"`
		Title    string  `json:"title"`
		Percent  float64 `json:"percent"`
		ResetsAt string  `json:"resetsAt"`
	} `json:"limits"`
}

// LoadCachedAccountUsage returns the best locally cached quota snapshot without network calls,
// along with whether a snapshot exists and whether a background refresh should be run.
func LoadCachedAccountUsage(cfg config.Config, now time.Time) (usage AccountUsage, ok bool, needsRefresh bool) {
	if now.IsZero() {
		now = time.Now()
	}

	var best AccountUsage
	hasGrouped := false

	if cached, found := loadNormalizedUsageFile(cfg.UsageCacheFile); found && len(cached.Groups) > 0 {
		best = cached
		hasGrouped = true
	}

	if pluginCached, found := loadPluginUsageCache(cfg.PluginUsageCacheFile); found && len(pluginCached.Groups) > 0 {
		if !hasGrouped || pluginCached.FetchedAt.After(best.FetchedAt) {
			best = pluginCached
			hasGrouped = true
		}
	}

	omarchyFallback, hasOmarchy := loadOmarchyUsageFallback(cfg.OmarchyUsageFile)
	if hasGrouped {
		if best.PlanTier == "" && hasOmarchy && omarchyFallback.PlanTier != "" {
			best.PlanTier = omarchyFallback.PlanTier
		}
		best.Stale = best.FetchedAt.IsZero() || now.Sub(best.FetchedAt) > UsageCacheTTL
		return best, true, best.Stale
	}

	if hasOmarchy {
		omarchyFallback.Stale = true
		return omarchyFallback, true, true
	}

	return AccountUsage{}, false, true
}

// FetchLiveAccountUsage executes "agy -p /usage --output-format json" and stores
// a normalized cache snapshot to cfg.UsageCacheFile when available.
func FetchLiveAccountUsage(ctx context.Context, cfg config.Config, now time.Time) (AccountUsage, error) {
	if now.IsZero() {
		now = time.Now()
	}

	bin, err := ResolveBinary()
	if err != nil {
		return AccountUsage{}, err
	}

	cmd := exec.CommandContext(ctx, bin, "-p", "/usage", "--output-format", "json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = runErr.Error()
		}
		return AccountUsage{}, fmt.Errorf("agy /usage failed: %s", detail)
	}

	parsed, err := ParseUsageCLIOutput(stdout.Bytes(), now)
	if err != nil {
		return AccountUsage{}, err
	}

	if parsed.PlanTier == "" {
		if omarchyFallback, ok := loadOmarchyUsageFallback(cfg.OmarchyUsageFile); ok && omarchyFallback.PlanTier != "" {
			parsed.PlanTier = omarchyFallback.PlanTier
		}
	}

	if cfg.UsageCacheFile != "" {
		_ = saveNormalizedUsageFile(cfg.UsageCacheFile, parsed)
	}
	return parsed, nil
}

// ParseUsageCLIOutput decodes JSON produced by "agy -p /usage --output-format json".
func ParseUsageCLIOutput(data []byte, fetchedAt time.Time) (AccountUsage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return AccountUsage{}, fmt.Errorf("agy /usage returned empty output")
	}

	var env usageEnvelope
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return AccountUsage{}, fmt.Errorf("parse agy /usage JSON: %w", err)
	}

	var raw *rawUsageData
	switch {
	case env.Command != nil && env.Command.Data != nil:
		raw = env.Command.Data
	case env.Data != nil:
		raw = env.Data
	default:
		var direct rawUsageData
		if err := json.Unmarshal(trimmed, &direct); err == nil && len(direct.Groups) > 0 {
			raw = &direct
		}
	}

	if raw == nil || len(raw.Groups) == 0 {
		return AccountUsage{}, fmt.Errorf("agy /usage output did not contain quota groups")
	}

	return normalizeRawUsageData(*raw, fetchedAt, "live"), nil
}

// FormatQuotaReset formats an ISO reset timestamp matching Antigravity's
// "Resets in 1d 3h · 4:24PM" display.
func FormatQuotaReset(resetTimeRaw string, now time.Time) string {
	trimmed := strings.TrimSpace(resetTimeRaw)
	if trimmed == "" {
		return ""
	}

	resetAt, ok := parseTimestampString(trimmed)
	if !ok {
		return trimmed
	}

	if now.IsZero() {
		now = time.Now()
	}

	clockStr := resetAt.Local().Format("3:04PM")
	diff := resetAt.Sub(now)
	if diff <= 0 {
		return "Resetting · " + clockStr
	}

	totalMins := int(diff.Minutes())
	days := totalMins / 1440
	hours := (totalMins % 1440) / 60
	mins := totalMins % 60

	var dur string
	switch {
	case days > 0:
		dur = fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		dur = fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		dur = fmt.Sprintf("%dm", mins)
	default:
		dur = "<1m"
	}

	return fmt.Sprintf("Resets in %s · %s", dur, clockStr)
}

func loadNormalizedUsageFile(path string) (AccountUsage, bool) {
	if strings.TrimSpace(path) == "" {
		return AccountUsage{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AccountUsage{}, false
	}
	var usage AccountUsage
	if err := json.Unmarshal(data, &usage); err != nil || len(usage.Groups) == 0 {
		return AccountUsage{}, false
	}
	for i := range usage.Groups {
		normalizeGroupDefaults(&usage.Groups[i])
	}
	if usage.Source == "" {
		usage.Source = "cache"
	}
	return usage, true
}

func saveNormalizedUsageFile(path string, usage AccountUsage) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create usage cache dir: %w", err)
	}
	payload, err := json.Marshal(usage)
	if err != nil {
		return fmt.Errorf("encode usage cache: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "usage-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp usage cache: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp usage cache: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp usage cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp usage cache: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace usage cache: %w", err)
	}
	cleanup = false
	return nil
}

func loadPluginUsageCache(path string) (AccountUsage, bool) {
	if strings.TrimSpace(path) == "" {
		return AccountUsage{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return AccountUsage{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AccountUsage{}, false
	}
	var raw rawUsageData
	if err := json.Unmarshal(data, &raw); err != nil || len(raw.Groups) == 0 {
		return AccountUsage{}, false
	}
	return normalizeRawUsageData(raw, info.ModTime(), "plugin-cache"), true
}

func loadOmarchyUsageFallback(path string) (AccountUsage, bool) {
	if strings.TrimSpace(path) == "" {
		return AccountUsage{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AccountUsage{}, false
	}
	var src omarchyAgentUsage
	if err := json.Unmarshal(data, &src); err != nil {
		return AccountUsage{}, false
	}
	tier := strings.TrimSpace(src.TierLabel)
	if len(src.Limits) == 0 && tier == "" {
		return AccountUsage{}, false
	}

	var fetchedAt time.Time
	if src.LimitsFetchedAt > 0 {
		fetchedAt = time.UnixMilli(src.LimitsFetchedAt)
	} else if t, ok := parseTimestampString(src.UpdatedAt); ok {
		fetchedAt = t
	}

	buckets := make([]QuotaBucket, 0, len(src.Limits))
	for _, lim := range src.Limits {
		usedFrac := clampFraction(lim.Percent)
		remFrac := clampFraction(1.0 - usedFrac)
		name := firstNonEmptyTrimmed(lim.Label, lim.Title, "Limit")
		buckets = append(buckets, QuotaBucket{
			Name:              name,
			Label:             normalizeBucketLabel(name, lim.Title),
			RemainingFraction: remFrac,
			RemainingPercent:  fractionToPercent(remFrac),
			ResetTime:         strings.TrimSpace(lim.ResetsAt),
		})
	}

	var groups []QuotaGroup
	if len(buckets) > 0 {
		groups = []QuotaGroup{
			{
				Name:    "Antigravity Quotas",
				Buckets: buckets,
			},
		}
	}

	return AccountUsage{
		PlanTier:  tier,
		Groups:    groups,
		FetchedAt: fetchedAt,
		Source:    "omarchy-cache",
		Stale:     true,
	}, len(groups) > 0 || tier != ""
}

func normalizeRawUsageData(raw rawUsageData, fetchedAt time.Time, source string) AccountUsage {
	groups := make([]QuotaGroup, 0, len(raw.Groups))
	for _, g := range raw.Groups {
		gName := strings.TrimSpace(g.Name)
		if gName == "" {
			gName = "Model Group"
		}
		buckets := make([]QuotaBucket, 0, len(g.Buckets))
		for _, b := range g.Buckets {
			var remFrac float64
			switch {
			case b.RemainingFraction != nil:
				remFrac = clampFraction(*b.RemainingFraction)
			case b.RemainingPercent != nil:
				remFrac = clampFraction(*b.RemainingPercent / 100.0)
			default:
				remFrac = 1.0
			}

			buckets = append(buckets, QuotaBucket{
				ID:                strings.TrimSpace(b.ID),
				Name:              strings.TrimSpace(b.Name),
				Label:             normalizeBucketLabel(b.Name, b.Window),
				Window:            strings.TrimSpace(b.Window),
				RemainingFraction: remFrac,
				RemainingPercent:  fractionToPercent(remFrac),
				ResetTime:         strings.TrimSpace(b.ResetTime),
				Description:       strings.TrimSpace(b.Description),
			})
		}
		grp := QuotaGroup{
			Name:        gName,
			Description: strings.TrimSpace(g.Description),
			WarmColor:   isWarmModelGroup(gName),
			Buckets:     buckets,
		}
		groups = append(groups, grp)
	}

	return AccountUsage{
		PlanTier:  firstNonEmptyTrimmed(raw.PlanTier, raw.Tier, raw.Plan),
		Groups:    groups,
		FetchedAt: fetchedAt,
		Source:    source,
	}
}

func normalizeGroupDefaults(g *QuotaGroup) {
	if g.Name == "" {
		g.Name = "Model Group"
	}
	g.WarmColor = g.WarmColor || isWarmModelGroup(g.Name)
	for i := range g.Buckets {
		b := &g.Buckets[i]
		b.RemainingFraction = clampFraction(b.RemainingFraction)
		if b.RemainingPercent == 0 && b.RemainingFraction > 0 {
			b.RemainingPercent = fractionToPercent(b.RemainingFraction)
		}
		if b.Label == "" {
			b.Label = normalizeBucketLabel(b.Name, b.Window)
		}
	}
}

func isWarmModelGroup(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "claude") ||
		strings.Contains(lower, "gpt") ||
		strings.Contains(lower, "anthropic") ||
		strings.Contains(lower, "openai")
}

func normalizeBucketLabel(name, window string) string {
	combined := strings.ToLower(name + " " + window)
	switch {
	case strings.Contains(combined, "weekly"), strings.Contains(combined, "7-day"), strings.Contains(combined, "7d"):
		return "Weekly Limit"
	case strings.Contains(combined, "5h"), strings.Contains(combined, "5-hour"), strings.Contains(combined, "five hour"), strings.Contains(combined, "session"):
		return "5-Hour Limit"
	}
	if clean := strings.TrimSpace(name); clean != "" {
		return clean
	}
	if clean := strings.TrimSpace(window); clean != "" {
		return clean
	}
	return "Limit"
}

func clampFraction(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func fractionToPercent(f float64) int {
	pct := int(math.Round(clampFraction(f) * 100))
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func parseTimestampString(s string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02 15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func firstNonEmptyTrimmed(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
