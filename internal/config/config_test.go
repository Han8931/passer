package config

import (
	"os"
	"path/filepath"
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
