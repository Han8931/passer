package config

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadOrCreateWritesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.ClipClearSeconds != 45 {
		t.Errorf("ClipClearSeconds = %d, want pass's default of 45", cfg.Security.ClipClearSeconds)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config was not created: %v", err)
	}
}

func TestLoadOrCreateWritesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if _, err := LoadOrCreate(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config mode = %o, want 0600", perm)
	}
}

func TestExplicitZeroDisablesAutoClear(t *testing.T) {
	// The zero value and "unset" must not be confused: a user who writes 0 is
	// deliberately turning auto-clear off.
	path := writeConfig(t, "[security]\nclip_clear_seconds = 0\n")

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.ClipClearSeconds != 0 {
		t.Errorf("ClipClearSeconds = %d, want the explicit 0 to survive", cfg.Security.ClipClearSeconds)
	}
	// Untouched keys in the same section keep their defaults.
	if cfg.Security.RevealSeconds != 30 {
		t.Errorf("RevealSeconds = %d, want the default 30", cfg.Security.RevealSeconds)
	}
}

func TestNegativeTimeoutsClampToZero(t *testing.T) {
	path := writeConfig(t, "[security]\nclip_clear_seconds = -5\nreveal_seconds = -1\nlock_seconds = -9\n")
	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.ClipClearSeconds != 0 || cfg.Security.RevealSeconds != 0 || cfg.Security.LockSeconds != 0 {
		t.Errorf("negative timeouts survived: %+v", cfg.Security)
	}
}

func TestGenerateLengthIsClamped(t *testing.T) {
	tests := []struct {
		body string
		want int
	}{
		{"[generate]\nlength = 0\n", 24},
		{"[generate]\nlength = -3\n", 24},
		{"[generate]\nlength = 9999\n", 256},
		{"[generate]\nlength = 32\n", 32},
	}
	for _, tc := range tests {
		cfg, err := LoadOrCreate(writeConfig(t, tc.body))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Generate.Length != tc.want {
			t.Errorf("%q gave length %d, want %d", tc.body, cfg.Generate.Length, tc.want)
		}
	}
}

func TestExplicitSymbolsFalseSurvives(t *testing.T) {
	cfg, err := LoadOrCreate(writeConfig(t, "[generate]\nsymbols = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Generate.Symbols {
		t.Error("symbols = false was overridden by the default")
	}
}

func TestThemePresetAppliesAsBase(t *testing.T) {
	cfg, err := LoadOrCreate(writeConfig(t, "[theme]\npreset = \"ocean\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := PresetTheme("ocean")
	if cfg.Theme.Accent != want.Accent {
		t.Errorf("Accent = %q, want the ocean palette's %q", cfg.Theme.Accent, want.Accent)
	}
}

func TestExplicitColorOverridesPreset(t *testing.T) {
	cfg, err := LoadOrCreate(writeConfig(t, "[theme]\npreset = \"ocean\"\naccent = \"#ABCDEF\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme.Accent != "#ABCDEF" {
		t.Errorf("Accent = %q, want the hand-tuned override to win", cfg.Theme.Accent)
	}
	want, _ := PresetTheme("ocean")
	if cfg.Theme.Border != want.Border {
		t.Errorf("Border = %q, want the preset value %q to remain", cfg.Theme.Border, want.Border)
	}
}

func TestUnknownPresetLeavesThemeAlone(t *testing.T) {
	cfg, err := LoadOrCreate(writeConfig(t, "[theme]\npreset = \"chartreuse\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme.Accent == "" {
		t.Error("an unknown preset should fall back to the default palette, not an empty one")
	}
}

func TestMissingKeysFallBackToDefaults(t *testing.T) {
	cfg, err := LoadOrCreate(writeConfig(t, "[keys]\nquit = \"Q\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Keys.Quit != "Q" {
		t.Errorf("Quit = %q, want the override Q", cfg.Keys.Quit)
	}
	if cfg.Keys.CopyPassword != "y" {
		t.Errorf("CopyPassword = %q, want the default y", cfg.Keys.CopyPassword)
	}
}

func TestAllPresetsAreResolvable(t *testing.T) {
	for _, name := range ThemePresetNames() {
		theme, ok := PresetTheme(name)
		if !ok {
			t.Errorf("PresetTheme(%q) reported unknown, but it is listed", name)
			continue
		}
		if theme.Preset != name {
			t.Errorf("PresetTheme(%q).Preset = %q; the name must round-trip so :theme can save it", name, theme.Preset)
		}
		if theme.Accent == "" || theme.Border == "" || theme.StatusBg == "" {
			t.Errorf("palette %q has empty colors: %+v", name, theme)
		}
	}
}

// relativeLuminance implements the WCAG 2.1 definition for an "#RRGGBB"
// string. Anything malformed returns false so the caller reports it as a bad
// color rather than silently scoring it.
func relativeLuminance(color string) (float64, bool) {
	hex := strings.TrimPrefix(strings.TrimSpace(color), "#")
	if len(hex) != 6 {
		return 0, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, false
	}
	channel := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r := channel(float64(v >> 16 & 0xFF))
	g := channel(float64(v >> 8 & 0xFF))
	b := channel(float64(v & 0xFF))
	return 0.2126*r + 0.7152*g + 0.0722*b, true
}

func contrastRatio(t *testing.T, fg, bg string) float64 {
	t.Helper()
	lf, ok := relativeLuminance(fg)
	if !ok {
		t.Fatalf("not a six-digit hex color: %q", fg)
	}
	lb, ok := relativeLuminance(bg)
	if !ok {
		t.Fatalf("not a six-digit hex color: %q", bg)
	}
	hi, lo := math.Max(lf, lb), math.Min(lf, lb)
	return (hi + 0.05) / (lo + 0.05)
}

// TestPresetContrast pins the contract documented on Theme. The pairings are
// the ones the renderer can actually produce, which is wider than it looks:
// the detail view stripes a selected row's background underneath cells that
// keep their own foreground, so muted, heading and warning all land on
// selection_bg — warning by way of the revealed-secret style.
func TestPresetContrast(t *testing.T) {
	// Text needs 4.5:1 (WCAG AA); accent-on-selection is a single highlighted
	// rune rather than a run of prose, so it is held to a looser 4:1.
	text := []struct {
		fg, bg string
		min    float64
	}{
		{"Foreground", "Background", 4.5},
		{"Muted", "Background", 4.5},
		{"Title", "Background", 4.5},
		{"Heading", "Background", 4.5},
		{"Accent", "Background", 4.5},
		{"Success", "Background", 4.5},
		{"Warning", "Background", 4.5},
		{"Danger", "Background", 4.5},

		{"SelectionFg", "SelectionBg", 4.5},
		{"Heading", "SelectionBg", 4.5},
		{"Muted", "SelectionBg", 4.5},
		{"Warning", "SelectionBg", 4.5},
		{"Accent", "SelectionBg", 4.0},

		{"StatusFg", "StatusBg", 4.5},
		{"Muted", "StatusBg", 4.5},
		{"Heading", "StatusBg", 4.5},
		{"Success", "StatusBg", 4.5},
		{"Warning", "StatusBg", 4.5},
		{"Danger", "StatusBg", 4.5},

		{"StatusAltFg", "StatusAltBg", 4.5},
		{"Muted", "StatusAltBg", 4.5},

		// The rounded frame is a graphical object, not text: WCAG asks 3:1.
		{"Border", "Background", 3.0},
	}
	// Surfaces only have to be told apart from each other, so the thresholds
	// are far below the text ones — but not 1.0, which is what a table header
	// sharing the status bar's background would score.
	surfaces := []struct {
		a, b string
		min  float64
	}{
		{"SelectionBg", "Background", 1.18},
		{"StatusAltBg", "Background", 1.25},
		{"StatusAltBg", "StatusBg", 1.12},
	}

	for _, name := range ThemePresetNames() {
		t.Run(name, func(t *testing.T) {
			theme, ok := PresetTheme(name)
			if !ok {
				t.Fatalf("PresetTheme(%q) reported unknown", name)
			}
			color := themeColors(t, theme)
			for _, c := range text {
				if got := contrastRatio(t, color[c.fg], color[c.bg]); got < c.min {
					t.Errorf("%s on %s = %.2f:1 (%s on %s); want >= %.1f:1",
						c.fg, c.bg, got, color[c.fg], color[c.bg], c.min)
				}
			}
			for _, s := range surfaces {
				if got := contrastRatio(t, color[s.a], color[s.b]); got < s.min {
					t.Errorf("surfaces %s and %s = %.2f:1 (%s and %s); they will not read as separate bands, want >= %.2f:1",
						s.a, s.b, got, color[s.a], color[s.b], s.min)
				}
			}
		})
	}
}

// themeColors indexes a Theme by field name so the contrast table can name
// pairings in the same vocabulary as the TOML keys.
func themeColors(t *testing.T, theme Theme) map[string]string {
	t.Helper()
	return map[string]string{
		"Background":  theme.Background,
		"Foreground":  theme.Foreground,
		"Title":       theme.Title,
		"Heading":     theme.Heading,
		"Accent":      theme.Accent,
		"Muted":       theme.Muted,
		"Success":     theme.Success,
		"Warning":     theme.Warning,
		"Danger":      theme.Danger,
		"Border":      theme.Border,
		"SelectionBg": theme.SelectionBg,
		"SelectionFg": theme.SelectionFg,
		"StatusBg":    theme.StatusBg,
		"StatusFg":    theme.StatusFg,
		"StatusAltBg": theme.StatusAltBg,
		"StatusAltFg": theme.StatusAltFg,
	}
}

// TestSemanticRolesAreDistinguishable guards the roles that share a line. The
// status bar puts the entry name (heading) beside the match counter (success),
// and a palette tuned only for contrast against the canvas can leave them the
// same color — which is how the earlier single-hue light palette lost the
// signal in `success`.
func TestSemanticRolesAreDistinguishable(t *testing.T) {
	pairs := []struct{ a, b string }{
		{"Accent", "Success"},
		{"Heading", "Success"},
	}
	for _, name := range ThemePresetNames() {
		t.Run(name, func(t *testing.T) {
			theme, _ := PresetTheme(name)
			color := themeColors(t, theme)
			for _, p := range pairs {
				dh := hueDistance(t, color[p.a], color[p.b])
				ratio := contrastRatio(t, color[p.a], color[p.b])
				// Either lever alone is enough to tell two roles apart.
				if dh < 40 && ratio < 1.5 {
					t.Errorf("%s (%s) and %s (%s) differ by only %.0f° of hue and %.2f:1; they read as one color side by side",
						p.a, color[p.a], p.b, color[p.b], dh, ratio)
				}
			}
		})
	}
}

// hueDistance returns the shortest angle between two colors' hues, in degrees.
func hueDistance(t *testing.T, a, b string) float64 {
	t.Helper()
	ha, hb := hueOf(t, a), hueOf(t, b)
	d := math.Abs(ha - hb)
	if d > 180 {
		d = 360 - d
	}
	return d
}

func hueOf(t *testing.T, color string) float64 {
	t.Helper()
	hex := strings.TrimPrefix(strings.TrimSpace(color), "#")
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		t.Fatalf("not a six-digit hex color: %q", color)
	}
	r := float64(v>>16&0xFF) / 255
	g := float64(v>>8&0xFF) / 255
	b := float64(v&0xFF) / 255
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := max - min
	if d == 0 {
		return 0 // grey has no hue; callers fall back to the contrast test
	}
	var h float64
	switch max {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h
}

func TestSaveRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := defaultConfig()
	cfg.Theme, _ = PresetTheme("forest")
	cfg.Generate.Length = 40

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme.Preset != "forest" {
		t.Errorf("Preset = %q after a round trip, want forest", got.Theme.Preset)
	}
	if got.Generate.Length != 40 {
		t.Errorf("Length = %d after a round trip, want 40", got.Generate.Length)
	}
}

func TestSaveThemePresetKeepsTheRestOfTheFile(t *testing.T) {
	example, err := os.ReadFile(filepath.Join("..", "..", "config.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(example), "clip_clear_seconds = 45", "clip_clear_seconds = 10", 1)
	path := writeConfig(t, body)

	if err := SaveThemePreset(path, "ocean"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	themeAt := strings.Index(body, "[theme]")
	if !strings.HasPrefix(out, body[:themeAt]) {
		t.Error("everything before [theme], comments included, must be kept byte for byte")
	}
	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := PresetTheme("ocean")
	if cfg.Theme != want {
		t.Errorf("theme = %+v, want the pure ocean preset with no stale light overrides", cfg.Theme)
	}
	if cfg.Security.ClipClearSeconds != 10 {
		t.Errorf("ClipClearSeconds = %d, want the value edited on disk kept", cfg.Security.ClipClearSeconds)
	}
}

func TestSaveThemePresetKeepsLaterTables(t *testing.T) {
	path := writeConfig(t, "[theme]\npreset = 'light'\naccent = '#000000'\n\n# keys follow\n[keys]\nquit = 'x'\n")
	if err := SaveThemePreset(path, "dark"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := "[theme]\npreset = 'dark'\n\n# keys follow\n[keys]\nquit = 'x'\n"
	if string(data) != want {
		t.Errorf("file =\n%s\nwant\n%s", data, want)
	}
}

func TestSaveThemePresetAppendsAMissingTable(t *testing.T) {
	path := writeConfig(t, "store_dir = ''")
	if err := SaveThemePreset(path, "rose"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme.Preset != "rose" {
		t.Errorf("preset = %q, want rose", cfg.Theme.Preset)
	}
}
