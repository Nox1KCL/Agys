package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultAndFromRoot(t *testing.T) {
	home := t.TempDir()
	xdgConfig := filepath.Join(home, "custom-config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

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
	wantState := filepath.Join(xdgConfig, "agys", "state.json")
	if cfg.StateFile != wantState {
		t.Fatalf("StateFile = %q, want %q", cfg.StateFile, wantState)
	}

	isolated := FromRoot(filepath.Join(home, "sandbox"))
	if isolated.StateFile != filepath.Join(home, "sandbox", "agys_state.json") {
		t.Fatalf("FromRoot StateFile = %q", isolated.StateFile)
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
}
