package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RelativeTime formats t relative to now using compact units (e.g. "now", "5m ago", "1d ago", "1w ago").
func RelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}

	diff := now.Sub(t)
	if diff < 0 {
		diff = 0
	}

	switch {
	case diff < time.Minute:
		return "now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff/time.Minute))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff/time.Hour))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff/(24*time.Hour)))
	case diff < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(diff/(7*24*time.Hour)))
	case diff < 365*24*time.Hour:
		months := int(diff / (30 * 24 * time.Hour))
		if months < 1 {
			months = 1
		}
		return fmt.Sprintf("%dmo ago", months)
	default:
		years := int(diff / (365 * 24 * time.Hour))
		if years < 1 {
			years = 1
		}
		return fmt.Sprintf("%dy ago", years)
	}
}

// ShortenHome replaces the current user's home directory prefix with "~".
func ShortenHome(path string) string {
	return ShortenHomeWith(path, userHomeDir())
}

// ShortenHomeWith replaces home prefix with "~" when path equals or resides inside home.
func ShortenHomeWith(path, home string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if home == "" {
		return clean
	}

	cleanHome := filepath.Clean(home)
	if clean == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if strings.HasPrefix(clean, prefix) {
		return "~" + string(filepath.Separator) + strings.TrimPrefix(clean, prefix)
	}
	return clean
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
