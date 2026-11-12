// Package config manages filesystem paths and runtime settings for agys.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultConfigRootDir = ".gemini"
	defaultDataDir       = "antigravity-cli"
	defaultAppConfigDir  = "agys"
	defaultStateFileName = "state.json"
	brainDirName         = "brain"
	knowledgeDirName     = "knowledge"
	annotationsDirName   = "annotations"
)

// Config holds resolved directory paths for Antigravity CLI data.
type Config struct {
	AntigravityDir string
	BrainDir       string
	KnowledgeDir   string
	AnnotationsDir string
	StateFile      string
}

// Default builds a Config rooted at ~/.gemini/antigravity-cli with user state under ~/.config/agys/state.json.
func Default() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve user home directory: %w", err)
	}

	root := filepath.Join(home, defaultConfigRootDir, defaultDataDir)
	cfg := FromRoot(root)

	configBase, cfgErr := os.UserConfigDir()
	if cfgErr != nil || configBase == "" {
		configBase = filepath.Join(home, ".config")
	}
	cfg.StateFile = filepath.Join(configBase, defaultAppConfigDir, defaultStateFileName)
	return cfg, nil
}

// FromRoot creates a Config from an explicit Antigravity data directory path.
func FromRoot(root string) Config {
	cleanRoot := filepath.Clean(root)
	return Config{
		AntigravityDir: cleanRoot,
		BrainDir:       filepath.Join(cleanRoot, brainDirName),
		KnowledgeDir:   filepath.Join(cleanRoot, knowledgeDirName),
		AnnotationsDir: filepath.Join(cleanRoot, annotationsDirName),
		StateFile:      filepath.Join(cleanRoot, "agys_state.json"),
	}
}
