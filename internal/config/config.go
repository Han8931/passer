package config

import (
	"errors"
	"fmt"
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
	SetUsername  string `toml:"set_username"`
	Reveal       string `toml:"reveal"`
	Add          string `toml:"add"`
	Edit         string `toml:"edit"`
	EditFile     string `toml:"edit_file"`
	Delete       string `toml:"delete"`
	Rename       string `toml:"rename"`
	ThemeToggle  string `toml:"theme_toggle"`
}

// Theme mirrors bada's palette keys so the two applications can share a look
// (and a user's hand-tuned colors) without translation.
//
// Every built-in preset holds to a contrast contract, enforced by
// TestPresetContrast: text roles reach 4.5:1 on each surface they are drawn
// on, the border reaches 3:1 against the canvas, and the three surfaces
// (canvas, status, status-alt) stay far enough apart to read as separate
// bands. The pairings are not obvious from the keys alone — `warning` lands on
// `selection_bg` because a revealed secret can sit on the selected row, and
// `muted` lands on all four surfaces — so change a color by running the test,
// not by eye.
type Theme struct {
	// Preset names a built-in palette. When set, it provides the base colors;
	// any other field left non-empty overrides that base.
	Preset      string `toml:"preset"`
	Background  string `toml:"background"`
	Foreground  string `toml:"foreground"`
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

// SaveThemePreset records a theme choice by rewriting only the [theme] table.
//
// Re-marshalling the whole Config would strip every comment from a
// hand-edited file and clobber anything changed on disk since passer started.
// The [theme] table is replaced wholesale, not just its preset key, because
// color overrides left behind would mask the preset the user just picked.
func SaveThemePreset(path, preset string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		cfg.Theme, _ = PresetTheme(preset)
		return write(path, cfg)
	}
	if err != nil {
		return err
	}

	updated := replaceThemeTable(string(data), preset)
	// Never write a file passer itself can't read back.
	var check Config
	if err := toml.Unmarshal([]byte(updated), &check); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(updated))
}

// replaceThemeTable swaps the body of the [theme] table for a single preset
// line, appending the table if the file has none. Comments directly above the
// next table header belong to that table and are kept.
func replaceThemeTable(src, preset string) string {
	body := fmt.Sprintf("preset = '%s'\n", preset)
	lines := strings.SplitAfter(src, "\n")

	start := -1
	for i, line := range lines {
		if isTableHeader(line) && tableName(line) == "theme" {
			start = i
			break
		}
	}
	if start < 0 {
		if src != "" && !strings.HasSuffix(src, "\n") {
			src += "\n"
		}
		return src + "\n[theme]\n" + body
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if isTableHeader(lines[i]) {
			end = i
			break
		}
	}
	keep := end
	if end < len(lines) {
		for keep > start+1 {
			t := strings.TrimSpace(lines[keep-1])
			if t != "" && !strings.HasPrefix(t, "#") {
				break
			}
			keep--
		}
		// One blank line separates the tables; drop the ones already there.
		for keep < end && strings.TrimSpace(lines[keep]) == "" {
			keep++
		}
	}

	var b strings.Builder
	for _, line := range lines[:start+1] {
		b.WriteString(line)
	}
	if !strings.HasSuffix(lines[start], "\n") {
		b.WriteString("\n")
	}
	b.WriteString(body)
	if end < len(lines) {
		b.WriteString("\n")
	}
	for _, line := range lines[keep:] {
		b.WriteString(line)
	}
	return b.String()
}

func isTableHeader(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "[")
}

func tableName(line string) string {
	t := strings.TrimSpace(line)
	if i := strings.Index(t, "#"); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	return strings.TrimSpace(strings.Trim(t, "[]"))
}

// writeFileAtomic replaces path via a temp file and rename, so a crash midway
// leaves the old config rather than half of a new one.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

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
			Background:  "#18211F",
			Foreground:  "#E0E8E1",
			Title:       "#7FC9D5",
			Heading:     "#BED2C7",
			Accent:      "#8CD5BD",
			Muted:       "#A0B1A7",
			Success:     "#97D873",
			Warning:     "#E7BB76",
			Danger:      "#F09898",
			Border:      "#566F63",
			SelectionBg: "#26483C",
			SelectionFg: "#EFF8F1",
			StatusBg:    "#222F29",
			StatusFg:    "#D8E4DB",
			StatusAltBg: "#2B3D34",
			StatusAltFg: "#EFF8F1",
			RowStripeBg: "#1D2923",
		}, true
	case "purple", "violet", "amethyst":
		return Theme{
			Preset:      "purple",
			Background:  "#181525",
			Foreground:  "#EDE9FE",
			Title:       "#C4B5FD",
			Heading:     "#DDD6FE",
			Accent:      "#F0ABFC",
			Muted:       "#A1A1AA",
			Success:     "#34D399",
			Warning:     "#FBBF24",
			Danger:      "#FB7185",
			Border:      "#705895",
			SelectionBg: "#471497",
			SelectionFg: "#FAF5FF",
			StatusBg:    "#1E1B2E",
			StatusFg:    "#E9D5FF",
			StatusAltBg: "#341271",
			StatusAltFg: "#F5F3FF",
			RowStripeBg: "#211A33",
		}, true
	case "ocean", "marine", "aqua":
		return Theme{
			Preset:      "ocean",
			Background:  "#0B202E",
			Foreground:  "#E0F2FE",
			Title:       "#67E8F9",
			Heading:     "#5EEAD4",
			Accent:      "#38BDF8",
			Muted:       "#AEBACB",
			Success:     "#5CD65C",
			Warning:     "#FBBF24",
			Danger:      "#FB7185",
			Border:      "#486E7A",
			SelectionBg: "#073E4D",
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
			Background:  "#142019",
			Foreground:  "#ECFDF5",
			Title:       "#BBF7D0",
			Heading:     "#86EFAC",
			Accent:      "#FACC15",
			Muted:       "#9CA3AF",
			Success:     "#22C55E",
			Warning:     "#F59E0B",
			Danger:      "#F87171",
			Border:      "#5A6F41",
			SelectionBg: "#124224",
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
			Background:  "#23171D",
			Foreground:  "#FFF1F2",
			Title:       "#FDA4AF",
			Heading:     "#F9A8D4",
			Accent:      "#FBBF24",
			Muted:       "#A8A29E",
			Success:     "#4ADE80",
			Warning:     "#F59E0B",
			Danger:      "#F43F5E",
			Border:      "#905467",
			SelectionBg: "#780032",
			SelectionFg: "#FFF1F2",
			StatusBg:    "#2A171C",
			StatusFg:    "#FFE4E6",
			StatusAltBg: "#56122B",
			StatusAltFg: "#FFF1F2",
			RowStripeBg: "#25161B",
		}, true
	case "graphite", "mono", "neutral":
		return Theme{
			Preset:      "graphite",
			Background:  "#18181B",
			Foreground:  "#F4F4F5",
			Title:       "#E5E7EB",
			Heading:     "#94A3B8",
			Accent:      "#A3E635",
			Muted:       "#9CA3AF",
			Success:     "#4ADE80",
			Warning:     "#EAB308",
			Danger:      "#EF4444",
			Border:      "#64646F",
			SelectionBg: "#383840",
			SelectionFg: "#FAFAFA",
			StatusBg:    "#18181B",
			StatusFg:    "#D4D4D8",
			StatusAltBg: "#2B2B2E",
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
	set(&dst.Background, src.Background)
	set(&dst.Foreground, src.Foreground)
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
		{&k.SetUsername, def.SetUsername},
		{&k.Reveal, def.Reveal},
		{&k.Add, def.Add},
		{&k.Edit, def.Edit},
		{&k.EditFile, def.EditFile},
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
			SetUsername:  "U",
			Reveal:       " ",
			Add:          "a",
			Edit:         "e",
			EditFile:     "E",
			Delete:       "D",
			Rename:       "m",
			ThemeToggle:  "t",
		},
		Theme: Theme{
			Preset:      "light",
			Background:  "#F5F2EB",
			Foreground:  "#25332F",
			Title:       "#1C5560",
			Heading:     "#3C5C53",
			Accent:      "#23685D",
			Muted:       "#525B54",
			Success:     "#2F6B2A",
			Warning:     "#7A4A12",
			Danger:      "#B43E43",
			Border:      "#819081",
			SelectionBg: "#C6DCD0",
			SelectionFg: "#25332F",
			StatusBg:    "#E6E8DF",
			StatusFg:    "#25332F",
			StatusAltBg: "#D8DBCD",
			StatusAltFg: "#42574E",
			RowStripeBg: "#ECEFE6",
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
