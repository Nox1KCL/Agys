package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultAndFromRoot(t *testing.T) {
	home := t.TempDir()
	xdgConfig := filepath.Join(home, "custom-config")
	xdgCache := filepath.Join(home, "custom-cache")
	xdgState := filepath.Join(home, "custom-state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_CACHE_HOME", xdgCache)
	t.Setenv("XDG_STATE_HOME", xdgState)

	cfg, err := Default()
	if err != nil {
		t.Fatalf("Default() failed: %v", err)
	}

	wantRoot := filepath.Join(home, ".gemini", "antigravity-cli")
	if cfg.AntigravityDir != wantRoot {
		t.Fatalf("AntigravityDir = %q, want %q", cfg.AntigravityDir, wantRoot)
	}
	if cfg.BrainDir != filepath.Join(wantRoot, "brain") {
		t.Fatalf("BrainDir = %q", cfg.BrainDir)
	}
	if cfg.KnowledgeDir != filepath.Join(wantRoot, "knowledge") {
		t.Fatalf("KnowledgeDir = %q", cfg.KnowledgeDir)
	}
	if cfg.AnnotationsDir != filepath.Join(wantRoot, "annotations") {
		t.Fatalf("AnnotationsDir = %q", cfg.AnnotationsDir)
	}
	if cfg.PresenceDir != filepath.Join(wantRoot, "presence") {
		t.Fatalf("PresenceDir = %q", cfg.PresenceDir)
	}
	wantState := filepath.Join(xdgConfig, "agys", "state.json")
	if cfg.StateFile != wantState {
		t.Fatalf("StateFile = %q, want %q", cfg.StateFile, wantState)
	}
	wantCache := filepath.Join(xdgCache, "agys", "index.gob")
	if cfg.CacheFile != wantCache {
		t.Fatalf("CacheFile = %q, want %q", cfg.CacheFile, wantCache)
	}
	wantUsageCache := filepath.Join(xdgCache, "agys", "usage.json")
	if cfg.UsageCacheFile != wantUsageCache {
		t.Fatalf("UsageCacheFile = %q, want %q", cfg.UsageCacheFile, wantUsageCache)
	}
	if cfg.PluginUsageCacheFile != filepath.Join(wantRoot, "cache", "quota_usage_cache.json") {
		t.Fatalf("PluginUsageCacheFile = %q", cfg.PluginUsageCacheFile)
	}
	wantOmarchyUsage := filepath.Join(xdgState, "omarchy", "agents", "usage", "agy.json")
	if cfg.OmarchyUsageFile != wantOmarchyUsage {
		t.Fatalf("OmarchyUsageFile = %q, want %q", cfg.OmarchyUsageFile, wantOmarchyUsage)
	}

	isolated := FromRoot(filepath.Join(home, "sandbox"))
	if isolated.StateFile != filepath.Join(home, "sandbox", "agys_state.json") {
		t.Fatalf("FromRoot StateFile = %q", isolated.StateFile)
	}
	if isolated.CacheFile != filepath.Join(home, "sandbox", "agys_index.gob") {
		t.Fatalf("FromRoot CacheFile = %q", isolated.CacheFile)
	}
	if isolated.UsageCacheFile != filepath.Join(home, "sandbox", "agys_usage.json") {
		t.Fatalf("FromRoot UsageCacheFile = %q", isolated.UsageCacheFile)
	}
	if isolated.PluginUsageCacheFile != filepath.Join(home, "sandbox", "cache", "quota_usage_cache.json") {
		t.Fatalf("FromRoot PluginUsageCacheFile = %q", isolated.PluginUsageCacheFile)
	}
	if isolated.OmarchyUsageFile != "" {
		t.Fatalf("expected isolated FromRoot OmarchyUsageFile empty, got %q", isolated.OmarchyUsageFile)
	}

	overridden, err := DefaultWithOverrides("~/custom-antigravity", "~/custom-state.json")
	if err != nil {
		t.Fatalf("DefaultWithOverrides() failed: %v", err)
	}
	wantCustomRoot := filepath.Join(home, "custom-antigravity")
	if overridden.AntigravityDir != wantCustomRoot || overridden.BrainDir != filepath.Join(wantCustomRoot, "brain") {
		t.Fatalf("unexpected overridden data dirs: %+v", overridden)
	}
	if overridden.StateFile != filepath.Join(home, "custom-state.json") {
		t.Fatalf("overridden StateFile = %q", overridden.StateFile)
	}
	if overridden.CacheFile == cfg.CacheFile || filepath.Dir(overridden.CacheFile) != filepath.Dir(cfg.CacheFile) {
		t.Fatalf("expected isolated CacheFile for custom data dir, got %q (default %q)", overridden.CacheFile, cfg.CacheFile)
	}
	if overridden.UsageCacheFile == cfg.UsageCacheFile || filepath.Dir(overridden.UsageCacheFile) != filepath.Dir(cfg.UsageCacheFile) {
		t.Fatalf("expected isolated UsageCacheFile for custom data dir, got %q (default %q)", overridden.UsageCacheFile, cfg.UsageCacheFile)
	}
}
