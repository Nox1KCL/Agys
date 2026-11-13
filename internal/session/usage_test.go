package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestParseUsageCLIOutput(t *testing.T) {
	now := time.Date(2026, 11, 13, 13, 0, 0, 0, time.UTC)
	payload := `{
	  "command": {
	    "data": {
	      "tier": "Pro",
	      "groups": [
	        {
	          "name": "Gemini Models",
	          "description": "Gemini Flash, Gemini Pro",
	          "buckets": [
	            {"id": "gem-week", "name": "Weekly Limit Remaining", "window": "weekly", "remaining_fraction": 0.61, "reset_time": "2026-11-14T16:24:00Z"},
	            {"id": "gem-5h", "name": "Five Hour Limit Remaining", "window": "5h", "remaining_fraction": 0.78, "reset_time": "2026-11-13T15:51:00Z"}
	          ]
	        },
	        {
	          "name": "Claude and GPT models",
	          "buckets": [
	            {"id": "3p-week", "name": "Weekly Limit Remaining", "window": "weekly", "remaining_fraction": 0.74, "reset_time": "2026-11-19T14:24:00Z"}
	          ]
	        }
	      ]
	    }
	  }
	}`

	usage, err := ParseUsageCLIOutput([]byte(payload), now)
	if err != nil {
		t.Fatalf("ParseUsageCLIOutput() failed: %v", err)
	}
	if usage.PlanTier != "Pro" {
		t.Fatalf("PlanTier = %q, want Pro", usage.PlanTier)
	}
	if len(usage.Groups) != 2 {
		t.Fatalf("len(Groups) = %d, want 2", len(usage.Groups))
	}
	if usage.Groups[0].WarmColor {
		t.Fatal("expected Gemini Models to not use WarmColor")
	}
	if !usage.Groups[1].WarmColor {
		t.Fatal("expected Claude and GPT models to use WarmColor")
	}
	if usage.Groups[0].Buckets[0].Label != "Weekly Limit" || usage.Groups[0].Buckets[0].RemainingPercent != 61 {
		t.Fatalf("unexpected bucket[0]: %+v", usage.Groups[0].Buckets[0])
	}
	if usage.Groups[0].Buckets[1].Label != "5-Hour Limit" || usage.Groups[0].Buckets[1].RemainingPercent != 78 {
		t.Fatalf("unexpected bucket[1]: %+v", usage.Groups[0].Buckets[1])
	}
}

func TestLoadCachedAndFetchLiveAccountUsage(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)
	omarchyPath := filepath.Join(root, "omarchy_agy.json")
	cfg.OmarchyUsageFile = omarchyPath

	omarchyData := `{
	  "tierLabel": "Pro",
	  "limits": [
	    {"label": "Session (5-hour)", "title": "Session", "percent": 0.0, "resetsAt": "2026-11-13T18:00:00Z"},
	    {"label": "Weekly (7-day)", "title": "Weekly", "percent": 0.39, "resetsAt": "2026-11-14T16:24:00Z"}
	  ]
	}`
	if err := os.WriteFile(omarchyPath, []byte(omarchyData), 0o600); err != nil {
		t.Fatalf("write omarchy fallback: %v", err)
	}

	now := time.Date(2026, 11, 13, 13, 0, 0, 0, time.UTC)
	usage, ok, needsRefresh := LoadCachedAccountUsage(cfg, now)
	if !ok || !needsRefresh {
		t.Fatalf("expected fallback snapshot ok=true, needsRefresh=true; got ok=%v refresh=%v", ok, needsRefresh)
	}
	if usage.PlanTier != "Pro" || len(usage.Groups) != 1 || len(usage.Groups[0].Buckets) != 2 {
		t.Fatalf("unexpected fallback usage: %+v", usage)
	}
	if usage.Groups[0].Buckets[1].RemainingPercent != 61 {
		t.Fatalf("expected 1.0-0.39 -> 61%% remaining, got %d%%", usage.Groups[0].Buckets[1].RemainingPercent)
	}

	// Write newer plugin cache and verify LoadCachedAccountUsage picks it up.
	if err := os.MkdirAll(filepath.Dir(cfg.PluginUsageCacheFile), 0o700); err != nil {
		t.Fatalf("mkdir plugin cache: %v", err)
	}
	pluginPayload := `{"groups":[{"name":"Gemini 4 Argon","buckets":[{"name":"Weekly","window":"weekly","remaining_fraction":0.97,"reset_time":"2026-11-14T17:47:00Z"}]}]}`
	if err := os.WriteFile(cfg.PluginUsageCacheFile, []byte(pluginPayload), 0o600); err != nil {
		t.Fatalf("write plugin cache: %v", err)
	}
	if err := os.Chtimes(cfg.PluginUsageCacheFile, now, now); err != nil {
		t.Fatalf("chtimes plugin cache: %v", err)
	}

	usage, ok, needsRefresh = LoadCachedAccountUsage(cfg, now.Add(30*time.Second))
	if !ok || needsRefresh {
		t.Fatalf("expected fresh plugin cache (needsRefresh=false), got ok=%v needsRefresh=%v", ok, needsRefresh)
	}
	if usage.PlanTier != "Pro" {
		t.Fatalf("expected tier enrichment from omarchy file, got %q", usage.PlanTier)
	}
	if usage.Groups[0].Name != "Gemini 4 Argon" || usage.Groups[0].Buckets[0].RemainingPercent != 97 {
		t.Fatalf("unexpected plugin cached groups: %+v", usage.Groups)
	}

	// Fake agy CLI in PATH for FetchLiveAccountUsage.
	binDir := t.TempDir()
	fakeAgy := filepath.Join(binDir, "agy")
	script := `#!/bin/sh
if [ "$1" = "-p" ] && [ "$2" = "/usage" ]; then
  printf '%s\n' '{"command":{"data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"w","name":"Weekly","window":"weekly","remaining_fraction":0.85,"reset_time":"2026-11-14T16:24:00Z"}]}]}}}'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(fakeAgy, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agy: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("AGY_BIN", "")

	live, err := FetchLiveAccountUsage(context.Background(), cfg, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("FetchLiveAccountUsage() failed: %v", err)
	}
	if live.Source != "live" || len(live.Groups) != 1 || live.Groups[0].Buckets[0].RemainingPercent != 85 {
		t.Fatalf("unexpected live usage: %+v", live)
	}

	rawSaved, err := os.ReadFile(cfg.UsageCacheFile)
	if err != nil {
		t.Fatalf("expected persisted UsageCacheFile at %s: %v", cfg.UsageCacheFile, err)
	}
	var roundtrip AccountUsage
	if err := json.Unmarshal(rawSaved, &roundtrip); err != nil || roundtrip.Groups[0].Buckets[0].RemainingPercent != 85 {
		t.Fatalf("unexpected persisted cache: err=%v val=%+v", err, roundtrip)
	}
}

func TestFormatQuotaReset(t *testing.T) {
	loc := time.FixedZone("UTC", 0)
	now := time.Date(2026, 11, 13, 13, 3, 0, 0, loc)

	gotDays := FormatQuotaReset("2026-11-14T16:24:00Z", now)
	wantClock := time.Date(2026, 11, 14, 16, 24, 0, 0, loc).Local().Format("3:04PM")
	if gotDays != "Resets in 1d 3h · "+wantClock {
		t.Fatalf("FormatQuotaReset(days) = %q, want %q", gotDays, "Resets in 1d 3h · "+wantClock)
	}

	gotHours := FormatQuotaReset("2026-11-13T15:51:00Z", now)
	wantHoursClock := time.Date(2026, 11, 13, 15, 51, 0, 0, loc).Local().Format("3:04PM")
	if gotHours != "Resets in 2h 48m · "+wantHoursClock {
		t.Fatalf("FormatQuotaReset(hours) = %q, want %q", gotHours, "Resets in 2h 48m · "+wantHoursClock)
	}

	gotPast := FormatQuotaReset("2026-11-13T12:00:00Z", now)
	wantPastClock := time.Date(2026, 11, 13, 12, 0, 0, 0, loc).Local().Format("3:04PM")
	if gotPast != "Resetting · "+wantPastClock {
		t.Fatalf("FormatQuotaReset(past) = %q", gotPast)
	}
}
