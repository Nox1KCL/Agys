package styles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLoadOmarchyPaletteVariants(t *testing.T) {
	dir := t.TempDir()
	colorsPath := filepath.Join(dir, "colors.toml")

	content := `
# Omarchy theme sample
mode = "dark"
accent = "#f38d70" # trailing comment
selection = '#403e41'
muted = #72696a # bare hex + comment
foreground = "#e6d9db"
bright_foreground = "#e6d9db"
light_foreground = "#c3b7b8"
dark_foreground = "#72696a"
cyan = "#78dce8"
yellow = "#ffd866"
orange = "#fc9867"
red = "#fd6883"
`
	if err := os.WriteFile(colorsPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write colors.toml: %v", err)
	}

	p, ok := LoadOmarchyPalette(colorsPath)
	if !ok {
		t.Fatal("expected palette to load cleanly")
	}
	if p.Accent != "#f38d70" || p.Selection != "#403e41" || p.Muted != "#72696a" || p.Red != "#fd6883" || p.Cyan != "#78dce8" || p.Orange != "#fc9867" {
		t.Fatalf("unexpected parsed palette: %+v", p)
	}

	theme := ThemeFromPalette(p)
	if got := theme.ModalBorderActive.GetBorderTopForeground(); got != lipgloss.Color("#f38d70") {
		t.Fatalf("expected active border accent #f38d70, got %#v", got)
	}
	if got := theme.QuotaCyan.GetForeground(); got != lipgloss.Color("#78dce8") {
		t.Fatalf("expected QuotaCyan #78dce8, got %#v", got)
	}
	if got := theme.QuotaWarm.GetForeground(); got != lipgloss.Color("#fc9867") {
		t.Fatalf("expected QuotaWarm #fc9867, got %#v", got)
	}
	if got := theme.ModalBorderDanger.GetBorderTopForeground(); got != lipgloss.Color("#fd6883") {
		t.Fatalf("expected danger border #fd6883, got %#v", got)
	}
	if got := theme.SelectedRow.GetBackground(); got != lipgloss.Color("#403e41") {
		t.Fatalf("expected SelectedRow background #403e41, got %#v", got)
	}
	if got := theme.ModalBorder.GetBackground(); got != (lipgloss.NoColor{}) {
		t.Fatalf("expected transparent container background, got %#v", got)
	}
}

func TestDefaultANSIFallbackWhenMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist", "colors.toml")
	if _, ok := LoadOmarchyPalette(missing); ok {
		t.Fatal("expected LoadOmarchyPalette to return ok=false for missing file")
	}

	fallback := DefaultANSIFallbackTheme()
	if got := fallback.ModalBorderActive.GetBorderTopForeground(); got != AccentColor {
		t.Fatalf("expected fallback accent %v, got %#v", AccentColor, got)
	}
	if got := fallback.ModalBorderDanger.GetBorderTopForeground(); got != ErrorColor {
		t.Fatalf("expected fallback danger color %v, got %#v", ErrorColor, got)
	}
	if !fallback.SelectedRow.GetReverse() {
		t.Fatal("expected fallback SelectedRow to use Reverse(true)")
	}
}

func TestCurrentThemeSignatureChangesOnEdit(t *testing.T) {
	colorsPath := filepath.Join(t.TempDir(), "colors.toml")
	if err := os.WriteFile(colorsPath, []byte(`accent = "#7aa2f7"`+"\n"), 0o600); err != nil {
		t.Fatalf("write colors.toml: %v", err)
	}
	sig1 := CurrentThemeSignature(colorsPath)
	if sig1 == "" {
		t.Fatal("expected non-empty signature")
	}

	if err := os.WriteFile(colorsPath, []byte(`accent = "#f38d70" # updated`+"\n"), 0o600); err != nil {
		t.Fatalf("rewrite colors.toml: %v", err)
	}
	sig2 := CurrentThemeSignature(colorsPath)
	if sig2 == "" || sig2 == sig1 {
		t.Fatalf("expected signature change after update, got sig1=%q sig2=%q", sig1, sig2)
	}
}
