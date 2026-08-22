package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	DefaultConfigFileName = "config.toml"
	DefaultConfigPathFile = "config.path"
)

// Keymap holds the rebindable keys. Every action the UI dispatches on goes
// through here, so a user can move the whole app onto their own muscle memory.
type Keymap struct {
	Quit         string `toml:"quit"`
	Up           string `toml:"up"`
	Down         string `toml:"down"`
	Open         string `toml:"open"`
	Cancel       string `toml:"cancel"`
	Search       string `toml:"search"`
	Command      string `toml:"command"`
	Help         string `toml:"help"`
	Reload       string `toml:"reload"`
	CopyPassword string `toml:"copy_password"`
	CopyUsername string `toml:"copy_username"`
	CopyURL      string `toml:"copy_url"`
	Reveal       string `toml:"reveal"`
	Add          string `toml:"add"`
	Edit         string `toml:"edit"`
	Delete       string `toml:"delete"`
	Rename       string `toml:"rename"`
	ThemeToggle  string `toml:"theme_toggle"`
}

// Theme mirrors bada's palette keys so the two applications can share a look
// (and a user's hand-tuned colors) without translation.
type Theme struct {
	// Preset names a built-in palette. When set, it provides the base colors;
	// any other field left non-empty overrides that base.
	Preset      string `toml:"preset"`
	Title       string `toml:"title"`
	Heading     string `toml:"heading"`
	Accent      string `toml:"accent"`
	Muted       string `toml:"muted"`
	Success     string `toml:"success"`
	Warning     string `toml:"warning"`
	Danger      string `toml:"danger"`
	Border      string `toml:"border"`
	SelectionBg string `toml:"selection_bg"`
	SelectionFg string `toml:"selection_fg"`
	StatusBg    string `toml:"status_bg"`
	StatusFg    string `toml:"status_fg"`
	StatusAltBg string `toml:"status_alt_bg"`
	StatusAltFg string `toml:"status_alt_fg"`
	RowStripeBg string `toml:"row_stripe_bg"`
}

// Security groups the settings that decide how long a secret is exposed.
type Security struct {
	// ClipClearSeconds is how long a copied secret stays on the clipboard.
	// 0 disables auto-clear, which we allow but do not recommend.
	ClipClearSeconds int `toml:"clip_clear_seconds"`
	// RevealSeconds auto-hides an on-screen password again. A revealed
	// password is readable by anyone near the terminal and, worse, survives in
	// scrollback; re-masking on a timer bounds that exposure.
	RevealSeconds int `toml:"reveal_seconds"`
	// LockSeconds drops the decrypted entry from memory after a period with no
	// keypress. 0 keeps it until the detail view closes.
	LockSeconds int `toml:"lock_seconds"`
}

// Generate holds the defaults for new passwords, matching `pass generate`.
type Generate struct {
	Length  int  `toml:"length"`
	Symbols bool `toml:"symbols"`
}

type Config struct {
	StoreDir string `toml:"store_dir"`
	// UsernameFields are extra field names to treat as the account identifier,
	// tried before the built-in conventions (login, username, user, email).
	UsernameFields []string `toml:"username_fields"`
	Security       Security `toml:"security"`
	Generate       Generate `toml:"generate"`
	Keys           Keymap   `toml:"keys"`
	Theme          Theme    `toml:"theme"`
}

func LoadOrCreate(path string) (Config, error) {
	cfg := defaultConfig()
	if strings.TrimSpace(path) == "" {
		return cfg, errors.New("config path is empty")
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := write(path, cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	// Decode a second time into a zero-valued struct so we can tell which theme
	// fields the user actually set, then fold in the named preset (if any).
	var raw Config
	if err := toml.Unmarshal(data, &raw); err == nil {
		resolveThemePreset(&cfg, raw.Theme)
	}
	applyKeyDefaults(&cfg)
	clampValues(&cfg)
	return cfg, nil
}

func Save(path string, cfg Config) error { return write(path, cfg) }

// ThemePresetNames lists the built-in palettes in display order. It is the
// single source of truth for the names the :theme command offers.
func ThemePresetNames() []string {
	return []string{"light", "dark", "purple", "ocean", "forest", "rose", "graphite"}
}

// PresetTheme returns a built-in palette by name. The second result is false
// for an unknown name.
func PresetTheme(name string) (Theme, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "light", "default", "":
		t := defaultConfig().Theme
		t.Preset = "light"
		return t, true
	case "dark", "slate", "dark-slate":
		return Theme{
			Preset:      "dark",
			Title:       "#8AB4F8",
			Heading:     "#7DD3FC",
			Accent:      "#F2CC60",
			Muted:       "#8B949E",
			Success:     "#3FB950",
			Warning:     "#D29922",
			Danger:      "#F85149",
			Border:      "#30363D",
			SelectionBg: "#264F78",
			SelectionFg: "#F0F6FC",
			StatusBg:    "#161B22",
			StatusFg:    "#C9D1D9",
			StatusAltBg: "#21262D",
			StatusAltFg: "#F0F6FC",
			RowStripeBg: "#11161D",
		}, true
	case "purple", "violet", "amethyst":
		return Theme{
			Preset:      "purple",
			Title:       "#C4B5FD",
			Heading:     "#DDD6FE",
			Accent:      "#F0ABFC",
			Muted:       "#A1A1AA",
			Success:     "#34D399",
			Warning:     "#FBBF24",
			Danger:      "#FB7185",
			Border:      "#4C1D95",
			SelectionBg: "#6D28D9",
			SelectionFg: "#FAF5FF",
			StatusBg:    "#1E1B2E",
			StatusFg:    "#E9D5FF",
			StatusAltBg: "#2E1065",
			StatusAltFg: "#F5F3FF",
			RowStripeBg: "#211A33",
		}, true
	case "ocean", "marine", "aqua":
		return Theme{
			Preset:      "ocean",
			Title:       "#67E8F9",
			Heading:     "#5EEAD4",
			Accent:      "#38BDF8",
			Muted:       "#94A3B8",
			Success:     "#2DD4BF",
			Warning:     "#FBBF24",
			Danger:      "#FB7185",
			Border:      "#155E75",
			SelectionBg: "#0E7490",
			SelectionFg: "#ECFEFF",
			StatusBg:    "#082F49",
			StatusFg:    "#BAE6FD",
			StatusAltBg: "#164E63",
			StatusAltFg: "#ECFEFF",
			RowStripeBg: "#0B2538",
		}, true
	case "forest", "evergreen", "green":
		return Theme{
			Preset:      "forest",
			Title:       "#BBF7D0",
			Heading:     "#86EFAC",
			Accent:      "#FACC15",
			Muted:       "#9CA3AF",
			Success:     "#22C55E",
			Warning:     "#F59E0B",
			Danger:      "#F87171",
			Border:      "#365314",
			SelectionBg: "#166534",
			SelectionFg: "#F0FDF4",
			StatusBg:    "#172016",
			StatusFg:    "#DCFCE7",
			StatusAltBg: "#1F3A1D",
			StatusAltFg: "#F0FDF4",
			RowStripeBg: "#141C13",
		}, true
	case "rose", "blush", "magenta":
		return Theme{
			Preset:      "rose",
			Title:       "#FDA4AF",
			Heading:     "#F9A8D4",
			Accent:      "#FBBF24",
			Muted:       "#A8A29E",
			Success:     "#4ADE80",
			Warning:     "#F59E0B",
			Danger:      "#F43F5E",
			Border:      "#881337",
			SelectionBg: "#BE185D",
			SelectionFg: "#FFF1F2",
			StatusBg:    "#2A171C",
			StatusFg:    "#FFE4E6",
			StatusAltBg: "#4C1026",
			StatusAltFg: "#FFF1F2",
			RowStripeBg: "#25161B",
		}, true
	case "graphite", "mono", "neutral":
		return Theme{
			Preset:      "graphite",
			Title:       "#E5E7EB",
			Heading:     "#CBD5E1",
			Accent:      "#A3E635",
			Muted:       "#9CA3AF",
			Success:     "#84CC16",
			Warning:     "#EAB308",
			Danger:      "#EF4444",
			Border:      "#3F3F46",
			SelectionBg: "#52525B",
			SelectionFg: "#FAFAFA",
			StatusBg:    "#18181B",
			StatusFg:    "#D4D4D8",
			StatusAltBg: "#27272A",
			StatusAltFg: "#FAFAFA",
			RowStripeBg: "#202024",
		}, true
	}
	return Theme{}, false
}

// resolveThemePreset applies a named preset as the base palette when the config
// sets [theme].preset, keeping any explicitly-provided color as an override.
// raw holds only the fields the user actually wrote (zero elsewhere).
func resolveThemePreset(cfg *Config, raw Theme) {
	if strings.TrimSpace(raw.Preset) == "" {
		return
	}
	base, ok := PresetTheme(raw.Preset)
	if !ok {
		return
	}
	overlayTheme(&base, raw)
	cfg.Theme = base
}

// overlayTheme copies every non-empty color from src onto dst. Preset is not
// overlaid.
func overlayTheme(dst *Theme, src Theme) {
	set := func(d *string, s string) {
		if strings.TrimSpace(s) != "" {
			*d = s
		}
	}
	set(&dst.Title, src.Title)
	set(&dst.Heading, src.Heading)
	set(&dst.Accent, src.Accent)
	set(&dst.Muted, src.Muted)
	set(&dst.Success, src.Success)
	set(&dst.Warning, src.Warning)
	set(&dst.Danger, src.Danger)
	set(&dst.Border, src.Border)
	set(&dst.SelectionBg, src.SelectionBg)
	set(&dst.SelectionFg, src.SelectionFg)
	set(&dst.StatusBg, src.StatusBg)
	set(&dst.StatusFg, src.StatusFg)
	set(&dst.StatusAltBg, src.StatusAltBg)
	set(&dst.StatusAltFg, src.StatusAltFg)
	set(&dst.RowStripeBg, src.RowStripeBg)
}

func applyKeyDefaults(cfg *Config) {
	def := defaultConfig().Keys
	k := &cfg.Keys
	for _, f := range []struct {
		dst *string
		def string
	}{
		{&k.Quit, def.Quit},
		{&k.Up, def.Up},
		{&k.Down, def.Down},
		{&k.Open, def.Open},
		{&k.Cancel, def.Cancel},
		{&k.Search, def.Search},
		{&k.Command, def.Command},
		{&k.Help, def.Help},
		{&k.Reload, def.Reload},
		{&k.CopyPassword, def.CopyPassword},
		{&k.CopyUsername, def.CopyUsername},
		{&k.CopyURL, def.CopyURL},
		{&k.Reveal, def.Reveal},
		{&k.Add, def.Add},
		{&k.Edit, def.Edit},
		{&k.Delete, def.Delete},
		{&k.Rename, def.Rename},
		{&k.ThemeToggle, def.ThemeToggle},
	} {
		if *f.dst == "" {
			*f.dst = f.def
		}
	}
}

// clampValues brings numeric settings into range.
//
// Defaults need no work here: decoding happens over an already-populated
// Config, so a key the file omits keeps its default while an explicitly written
// `clip_clear_seconds = 0` survives as the user's deliberate "never clear".
// Only genuinely out-of-range values are corrected.
func clampValues(cfg *Config) {
	def := defaultConfig()
	if cfg.Security.ClipClearSeconds < 0 {
		cfg.Security.ClipClearSeconds = 0
	}
	if cfg.Security.RevealSeconds < 0 {
		cfg.Security.RevealSeconds = 0
	}
	if cfg.Security.LockSeconds < 0 {
		cfg.Security.LockSeconds = 0
	}
	if cfg.Generate.Length <= 0 {
		cfg.Generate.Length = def.Generate.Length
	}
	if cfg.Generate.Length > 256 {
		cfg.Generate.Length = 256
	}
}

func write(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	// 0600: the config names the store location and the user's key preferences.
	// It holds no secrets, but a password manager should not be the loosest
	// file in ~/.config.
	return os.WriteFile(path, data, 0o600)
}

func defaultConfig() Config {
	return Config{
		StoreDir:       "",
		UsernameFields: []string{"login", "username", "user", "email"},
		Security: Security{
			ClipClearSeconds: 45, // pass's own default
			RevealSeconds:    30,
			LockSeconds:      120,
		},
		Generate: Generate{
			Length:  24,
			Symbols: true,
		},
		Keys: Keymap{
			Quit:         "q",
			Up:           "k",
			Down:         "j",
			Open:         "enter",
			Cancel:       "esc",
			Search:       "/",
			Command:      ":",
			Help:         "?",
			Reload:       "R",
			CopyPassword: "y",
			CopyUsername: "u",
			CopyURL:      "b",
			Reveal:       " ",
			Add:          "a",
			Edit:         "e",
			Delete:       "D",
			Rename:       "m",
			ThemeToggle:  "t",
		},
		Theme: Theme{
			Preset:      "light",
			Title:       "#2563EB",
			Heading:     "#0F766E",
			Accent:      "#D97706",
			Muted:       "#64748B",
			Success:     "#16A34A",
			Warning:     "#B45309",
			Danger:      "#DC2626",
			Border:      "#CBD5E1",
			SelectionBg: "#DBEAFE",
			SelectionFg: "#0F172A",
			StatusBg:    "#E2E8F0",
			StatusFg:    "#0F172A",
			StatusAltBg: "#0F766E",
			StatusAltFg: "#F8FAFC",
			RowStripeBg: "#F1F5F9",
		},
	}
}

func DefaultConfigDir() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "passer")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "passer")
	}
	return "passer"
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), DefaultConfigFileName)
}

func ConfigPathFile() string {
	return filepath.Join(DefaultConfigDir(), DefaultConfigPathFile)
}

func ResolveConfigPath() string {
	if env := strings.TrimSpace(os.Getenv("PASSER_CONFIG")); env != "" {
		return env
	}
	if data, err := os.ReadFile(ConfigPathFile()); err == nil {
		if path := strings.TrimSpace(string(data)); path != "" {
			return path
		}
	}
	return DefaultConfigPath()
}

func SetConfigPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("config path is empty")
	}
	if err := os.MkdirAll(DefaultConfigDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(ConfigPathFile(), []byte(path+"\n"), 0o600)
}
