// Package utils provides shared filesystem and path helpers for agys.
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirExists reports whether path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ExpandHome resolves a leading "~/" prefix against the current user's home directory.
func ExpandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return filepath.Clean(path), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

// ResolveAbsPath trims whitespace, expands a leading "~/", and converts relative paths to clean absolute paths.
func ResolveAbsPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", nil
	}

	expanded, err := ExpandHome(trimmed)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", fmt.Errorf("resolve absolute path %q: %w", expanded, err)
		}
		expanded = abs
	}

	return filepath.Clean(expanded), nil
}
