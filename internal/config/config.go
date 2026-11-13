// Package config manages filesystem paths and runtime settings for agys.
package config

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nox1KCL/Agys/internal/utils"
)

const (
	defaultConfigRootDir = ".gemini"
	defaultDataDir       = "antigravity-cli"
	defaultAppConfigDir  = "agys"
	defaultStateFileName = "state.json"
	defaultCacheFileName = "index.gob"
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
	CacheFile      string
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

	cacheBase, cacheErr := os.UserCacheDir()
	if cacheErr != nil || cacheBase == "" {
		cacheBase = filepath.Join(home, ".cache")
	}
	cfg.CacheFile = filepath.Join(cacheBase, defaultAppConfigDir, defaultCacheFileName)
	return cfg, nil
}

// DefaultWithOverrides returns the default Config with optional dataDir and stateFile overrides.
// Relative paths and "~/" prefixes are resolved to clean absolute paths.
func DefaultWithOverrides(dataDir, stateFile string) (Config, error) {
	cfg, err := Default()
	if err != nil {
		return Config{}, err
	}

	if strings.TrimSpace(dataDir) != "" {
		resolved, err := utils.ResolveAbsPath(dataDir)
		if err != nil {
			return Config{}, fmt.Errorf("resolve data directory: %w", err)
		}
		if resolved != cfg.AntigravityDir {
			applyDataRoot(&cfg, resolved)
			if cfg.CacheFile != "" {
				h := fnv.New32a()
				_, _ = h.Write([]byte(resolved))
				cfg.CacheFile = filepath.Join(filepath.Dir(cfg.CacheFile), fmt.Sprintf("index-%08x.gob", h.Sum32()))
			}
		}
	}

	if strings.TrimSpace(stateFile) != "" {
		resolved, err := utils.ResolveAbsPath(stateFile)
		if err != nil {
			return Config{}, fmt.Errorf("resolve state file: %w", err)
		}
		cfg.StateFile = resolved
	}

	return cfg, nil
}

// FromRoot creates a Config from an explicit Antigravity data directory path.
func FromRoot(root string) Config {
	cleanRoot := filepath.Clean(root)
	var cfg Config
	applyDataRoot(&cfg, cleanRoot)
	cfg.StateFile = filepath.Join(cleanRoot, "agys_state.json")
	cfg.CacheFile = filepath.Join(cleanRoot, "agys_index.gob")
	return cfg
}

func applyDataRoot(cfg *Config, cleanRoot string) {
	cfg.AntigravityDir = cleanRoot
	cfg.BrainDir = filepath.Join(cleanRoot, brainDirName)
	cfg.KnowledgeDir = filepath.Join(cleanRoot, knowledgeDirName)
	cfg.AnnotationsDir = filepath.Join(cleanRoot, annotationsDirName)
}
