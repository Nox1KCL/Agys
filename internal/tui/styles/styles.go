// Package styles defines Lip Gloss styles synchronized with the active Omarchy
// theme (when present) and the terminal's ANSI palette, preserving transparent backgrounds.
package styles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Default ANSI palette indices used as fallback when no Omarchy palette is found.
var (
	AccentColor = lipgloss.Color("4") // Terminal Blue (matches Omarchy ANSI accent slot)
	MutedColor  = lipgloss.Color("8") // Terminal BrightBlack
	ErrorColor  = lipgloss.Color("1") // Terminal Red
)

// OmarchyPalette captures color tokens parsed from Omarchy's current colors.toml.
type OmarchyPalette struct {
	Mode                string
	Accent              string
	Selection           string
	SelectionForeground string
	Muted               string
	Foreground          string
	BrightForeground    string
	LightForeground     string
	DarkForeground      string
	Blue                string
	Red                 string
	SourcePath          string
}

// Theme holds reusable Lip Gloss styles with transparent outer backgrounds.
type Theme struct {
	ModalBorder       lipgloss.Style
	ModalBorderActive lipgloss.Style
	HeaderHint        lipgloss.Style
	SearchPrompt      lipgloss.Style
	Divider           lipgloss.Style
	RowDate           lipgloss.Style
	RowTitle          lipgloss.Style
	RowDir            lipgloss.Style
	RowBadge          lipgloss.Style
	PinnedBadge       lipgloss.Style
	ArchivedRow       lipgloss.Style
	SelectedRow       lipgloss.Style
	SubRow            lipgloss.Style
	SubRowSelected    lipgloss.Style
	PreviewHeading    lipgloss.Style
	PreviewLabel      lipgloss.Style
	PreviewBody       lipgloss.Style
	PreviewMuted      lipgloss.Style
	Footer            lipgloss.Style
	ErrorText         lipgloss.Style
}

// DefaultTheme loads the active Omarchy theme when available, falling back to terminal ANSI 16 colors.
func DefaultTheme() Theme {
	theme, _ := LoadTheme()
	return theme
}

// LoadTheme resolves the active theme along with a filesystem signature for live reload checks.
func LoadTheme() (Theme, string) {
	p, ok := LoadOmarchyPalette()
	if !ok {
		return DefaultANSIFallbackTheme(), ""
	}
	return ThemeFromPalette(p), paletteSignature(p.SourcePath)
}

// DefaultANSIFallbackTheme returns the pure ANSI 16-color theme without hardcoded hex values.
func DefaultANSIFallbackTheme() Theme {
	return Theme{
		ModalBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(MutedColor).
			Padding(1, 2),
		ModalBorderActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(AccentColor).
			Padding(1, 2),
		HeaderHint: lipgloss.NewStyle().
			Foreground(MutedColor),
		SearchPrompt: lipgloss.NewStyle().
			Foreground(AccentColor).
			Bold(true),
		Divider: lipgloss.NewStyle().
			Foreground(MutedColor),
		RowDate: lipgloss.NewStyle().
			Foreground(MutedColor),
		RowTitle: lipgloss.NewStyle(),
		RowDir: lipgloss.NewStyle().
			Foreground(MutedColor),
		RowBadge: lipgloss.NewStyle().
			Foreground(AccentColor),
		PinnedBadge: lipgloss.NewStyle().
			Foreground(AccentColor).
			Bold(true),
		ArchivedRow: lipgloss.NewStyle().
			Foreground(MutedColor),
		SelectedRow: lipgloss.NewStyle().
			Bold(true).
			Reverse(true),
		SubRow: lipgloss.NewStyle().
			Foreground(MutedColor),
		SubRowSelected: lipgloss.NewStyle().
			Bold(true).
			Reverse(true),
		PreviewHeading: lipgloss.NewStyle().
			Foreground(AccentColor).
			Bold(true),
		PreviewLabel: lipgloss.NewStyle().
			Foreground(MutedColor),
		PreviewBody: lipgloss.NewStyle(),
		PreviewMuted: lipgloss.NewStyle().
			Foreground(MutedColor),
		Footer: lipgloss.NewStyle().
			Foreground(MutedColor),
		ErrorText: lipgloss.NewStyle().
			Foreground(ErrorColor).
			Bold(true),
	}
}

// ThemeFromPalette constructs a Theme from an OmarchyPalette while leaving base backgrounds transparent.
func ThemeFromPalette(p OmarchyPalette) Theme {
	accent := lipgloss.Color(firstNonEmpty(p.Accent, p.Blue, string(AccentColor)))
	borderMuted := lipgloss.Color(firstNonEmpty(p.Muted, p.DarkForeground, string(MutedColor)))
	secondaryText := lipgloss.Color(firstNonEmpty(p.LightForeground, p.DarkForeground, p.Muted, string(MutedColor)))
	dimText := lipgloss.Color(firstNonEmpty(p.DarkForeground, p.Muted, string(MutedColor)))
	errColor := lipgloss.Color(firstNonEmpty(p.Red, string(ErrorColor)))

	baseFg := lipgloss.NewStyle()
	if p.Foreground != "" {
		baseFg = baseFg.Foreground(lipgloss.Color(p.Foreground))
	}

	selStyle := lipgloss.NewStyle().Bold(true)
	if p.Selection != "" {
		selStyle = selStyle.Background(lipgloss.Color(p.Selection))
		if selFg := firstNonEmpty(p.SelectionForeground, p.BrightForeground, p.Foreground); selFg != "" {
			selStyle = selStyle.Foreground(lipgloss.Color(selFg))
		}
	} else {
		selStyle = selStyle.Reverse(true)
	}

	return Theme{
		ModalBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderMuted).
			Padding(1, 2),
		ModalBorderActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2),
		HeaderHint: lipgloss.NewStyle().
			Foreground(secondaryText),
		SearchPrompt: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),
		Divider: lipgloss.NewStyle().
			Foreground(borderMuted),
		RowDate: lipgloss.NewStyle().
			Foreground(secondaryText),
		RowTitle: baseFg,
		RowDir: lipgloss.NewStyle().
			Foreground(dimText),
		RowBadge: lipgloss.NewStyle().
			Foreground(accent),
		PinnedBadge: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),
		ArchivedRow: lipgloss.NewStyle().
			Foreground(dimText),
		SelectedRow: selStyle,
		SubRow: lipgloss.NewStyle().
			Foreground(dimText),
		SubRowSelected: selStyle,
		PreviewHeading: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),
		PreviewLabel: lipgloss.NewStyle().
			Foreground(secondaryText),
		PreviewBody: baseFg,
		PreviewMuted: lipgloss.NewStyle().
			Foreground(dimText),
		Footer: lipgloss.NewStyle().
			Foreground(secondaryText),
		ErrorText: lipgloss.NewStyle().
			Foreground(errColor).
			Bold(true),
	}
}

// DefaultOmarchyThemePaths returns the candidate paths for Omarchy's active colors.toml.
func DefaultOmarchyThemePaths() []string {
	var paths []string

	if stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); stateHome != "" {
		paths = append(paths, filepath.Join(stateHome, "omarchy", "current", "theme", "colors.toml"))
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".local", "state", "omarchy", "current", "theme", "colors.toml"))
	}

	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		paths = append(paths, filepath.Join(configHome, "omarchy", "current", "theme", "colors.toml"))
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "omarchy", "current", "theme", "colors.toml"))
	}

	return paths
}

// LoadOmarchyPalette searches paths (or DefaultOmarchyThemePaths when empty) and parses colors.toml.
func LoadOmarchyPalette(paths ...string) (OmarchyPalette, bool) {
	if len(paths) == 0 {
		paths = DefaultOmarchyThemePaths()
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		pal := parseColorsTOML(string(raw))
		pal.SourcePath = p
		if pal.Accent != "" || pal.Foreground != "" || pal.Selection != "" {
			return pal, true
		}
	}
	return OmarchyPalette{}, false
}

// CurrentThemeSignature returns a lightweight cache signature of the active theme file on disk.
func CurrentThemeSignature(paths ...string) string {
	if len(paths) == 0 {
		paths = DefaultOmarchyThemePaths()
	}
	for _, p := range paths {
		if sig := paletteSignature(p); sig != "" {
			return sig
		}
	}
	return ""
}

func paletteSignature(path string) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	return fmt.Sprintf("%s|%s|%d|%d", path, resolved, info.ModTime().UnixNano(), info.Size())
}

func parseColorsTOML(content string) OmarchyPalette {
	var p OmarchyPalette
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := extractTOMLValue(line[eq+1:])
		if val == "" {
			continue
		}

		switch key {
		case "mode":
			p.Mode = strings.ToLower(val)
		case "accent":
			p.Accent = normalizeHexColor(val)
		case "selection", "selection_background":
			p.Selection = normalizeHexColor(val)
		case "selection_foreground":
			p.SelectionForeground = normalizeHexColor(val)
		case "muted":
			p.Muted = normalizeHexColor(val)
		case "foreground":
			p.Foreground = normalizeHexColor(val)
		case "bright_foreground":
			p.BrightForeground = normalizeHexColor(val)
		case "light_foreground":
			p.LightForeground = normalizeHexColor(val)
		case "dark_foreground":
			p.DarkForeground = normalizeHexColor(val)
		case "blue":
			p.Blue = normalizeHexColor(val)
		case "red":
			p.Red = normalizeHexColor(val)
		}
	}
	return p
}

// extractTOMLValue parses quoted or bare TOML string tokens and ignores trailing comments.
func extractTOMLValue(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	if q := s[0]; q == '"' || q == '\'' {
		end := strings.IndexByte(s[1:], q)
		if end < 0 {
			return ""
		}
		return strings.TrimSpace(s[1 : 1+end])
	}

	// Bare value: trim inline "#" comment only if preceded by whitespace (preserving #RRGGBB).
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			s = s[:i]
			break
		}
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func normalizeHexColor(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "#") {
		if len(s) == 6 && isHexDigits(s) {
			return "#" + s
		}
		return ""
	}
	hex := s[1:]
	if (len(hex) == 6 || len(hex) == 3) && isHexDigits(hex) {
		return "#" + hex
	}
	return ""
}

func isHexDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !isDigit && !isLower && !isUpper {
			return false
		}
	}
	return len(s) > 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
