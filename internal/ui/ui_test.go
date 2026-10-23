package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"passer/internal/config"
	"passer/internal/pass"
)

func testModel(names ...string) Model {
	cfg := config.Config{}
	items := make([]pass.Item, len(names))
	for i, n := range names {
		items[i] = pass.Item{Name: n, Modified: time.Now()}
	}
	ti := textinput.New() // a zero-value textinput panics when focused
	ti.Prompt = ""
	m := Model{
		cfg:    cfg,
		items:  items,
		input:  ti,
		width:  100,
		height: 30,
		styles: buildStyles(cfg.Theme),
	}
	m.applyFilter()
	return m
}

func TestApplyFilterKeepsCursorOnTheSameEntry(t *testing.T) {
	m := testModel("a", "b", "c")
	m.cursor = 2 // on "c"

	m.query = "c"
	m.applyFilter()
	if got := m.currentName(); got != "c" {
		t.Fatalf("after filtering, cursor is on %q, want c", got)
	}

	m.query = ""
	m.applyFilter()
	if got := m.currentName(); got != "c" {
		t.Errorf("after clearing the filter, cursor is on %q, want c", got)
	}
}

func TestApplyFilterHonoursPendingSelect(t *testing.T) {
	m := testModel("a", "b", "c")
	m.pendingSelect = "b"
	m.applyFilter()

	if got := m.currentName(); got != "b" {
		t.Errorf("cursor is on %q, want the newly written entry b", got)
	}
	if m.pendingSelect != "" {
		t.Error("pendingSelect should be consumed once applied")
	}
}

func TestApplyFilterResetsCursorWhenTheEntryIsGone(t *testing.T) {
	m := testModel("a", "b", "c")
	m.cursor = 2

	// "c" was deleted underneath us.
	m.items = m.items[:2]
	m.applyFilter()

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 when the selected entry disappeared", m.cursor)
	}
	if got := m.currentName(); got != "a" {
		t.Errorf("currentName = %q, want a", got)
	}
}

func TestMoveCursorClampsToBounds(t *testing.T) {
	m := testModel("a", "b", "c")

	m.moveCursor(-5)
	if m.cursor != 0 {
		t.Errorf("cursor = %d after moving up past the top, want 0", m.cursor)
	}
	m.moveCursor(99)
	if m.cursor != 2 {
		t.Errorf("cursor = %d after moving down past the end, want 2", m.cursor)
	}
}

func TestMoveCursorOnEmptyList(t *testing.T) {
	m := testModel()
	m.moveCursor(1)
	if m.cursor != 0 {
		t.Errorf("cursor = %d on an empty list, want 0", m.cursor)
	}
	if name := m.currentName(); name != "" {
		t.Errorf("currentName = %q on an empty list, want empty", name)
	}
}

func TestDetailRowsMasksSecretFields(t *testing.T) {
	entry := pass.ParseEntry("x", strings.Join([]string{
		"hunter2",
		"login: han",
		"url: https://example.com",
		"otp: JBSWY3DPEHPK3PXP",
		"recovery codes: 1111 2222",
		"just a note",
	}, "\n"))

	m := testModel()
	m.entry = &entry
	rows := m.detailRows()

	secret := map[string]bool{}
	for _, r := range rows {
		secret[r.label] = r.secret
	}

	for _, label := range []string{"password", "otp", "recovery codes"} {
		if !secret[label] {
			t.Errorf("%q should be masked alongside the password", label)
		}
	}
	for _, label := range []string{"login", "url"} {
		if secret[label] {
			t.Errorf("%q should not be masked; it is not a secret", label)
		}
	}
}

func TestDetailRowsIncludesNotesAndSkipsBlanks(t *testing.T) {
	entry := pass.ParseEntry("x", "pw\nfirst note\n\nsecond note\n")
	m := testModel()
	m.entry = &entry

	rows := m.detailRows()
	if len(rows) != 3 {
		t.Fatalf("rows = %d (%+v), want password plus two notes", len(rows), rows)
	}
	if rows[0].label != "password" {
		t.Errorf("first row is %q, want password", rows[0].label)
	}
}

func TestDetailRowsSurfacesAnAccountCarriedByTheName(t *testing.T) {
	entry := pass.ParseEntry("web/github.com/han@example.com", "hunter2\n")
	m := testModel()
	m.entry = &entry

	rows := m.detailRows()
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want password plus the derived account", rows)
	}
	if rows[1].label != "account" || rows[1].value != "han@example.com" {
		t.Errorf("second row = %+v, want the account from the entry name", rows[1])
	}
	if rows[1].secret {
		t.Error("an account identifier is not a secret and should not be masked")
	}
}

func TestDetailRowsDoesNotDuplicateALabelledAccount(t *testing.T) {
	// The field already has a row of its own; a second derived one would be noise.
	entry := pass.ParseEntry("web/github.com/han@example.com", "hunter2\nlogin: han\n")
	m := testModel()
	m.entry = &entry

	for _, r := range m.detailRows() {
		if r.label == "account" {
			t.Errorf("rows = %+v, want no derived account row when a field exists", m.detailRows())
		}
	}
}

func TestSetAccountPromptPrefillsTheInferredValue(t *testing.T) {
	// Pressing U on an entry whose account passer only inferred should offer
	// that value, so confirming it into a real field is one keystroke.
	entry := pass.ParseEntry("web/github.com/han@example.com", "hunter2\n")
	m := testModel("web/github.com/han@example.com")
	m.cfg.Keys.SetUsername = "U"
	m.entry = &entry
	m.mode = modeDetail

	out, _ := m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	got := out.(Model)
	if got.mode != modeEditField {
		t.Fatalf("mode = %v, want the account prompt", got.mode)
	}
	if got.input.Value() != "han@example.com" {
		t.Errorf("prompt = %q, want it pre-filled with the inferred account", got.input.Value())
	}
	if got.field == nil || got.field.from != modeDetail {
		t.Errorf("field = %+v, want a return to the detail view", got.field)
	}
}

func TestSetAccountEscapeReleasesPlaintextOpenedFromTheList(t *testing.T) {
	// A decrypt made only to serve the rewrite must not outlive the prompt.
	entry := pass.ParseEntry("wifi", "only-a-password\n")
	m := testModel("wifi")
	out, _ := m.openSetAccount(entry, modeList)

	back, _ := out.(Model).onKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := back.(Model)
	if got.mode != modeList {
		t.Errorf("mode = %v, want the list", got.mode)
	}
	if got.entry != nil {
		t.Error("cancelling a list-opened prompt must drop the decrypted entry")
	}
	if got.field != nil {
		t.Error("prompt state should be cleared")
	}
}

func TestSetAccountEscapeKeepsAnOpenEntry(t *testing.T) {
	entry := pass.ParseEntry("wifi", "only-a-password\n")
	m := testModel("wifi")
	out, _ := m.openSetAccount(entry, modeDetail)

	back, _ := out.(Model).onKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := back.(Model)
	if got.mode != modeDetail || got.entry == nil {
		t.Errorf("mode = %v, entry = %v; want the entry still open", got.mode, got.entry)
	}
}

func TestSetAccountSubmitWritesAndEmptyInputCancels(t *testing.T) {
	entry := pass.ParseEntry("wifi", "only-a-password\n")

	// A value: saves.
	m := testModel("wifi")
	out, _ := m.openSetAccount(entry, modeDetail)
	mm := out.(Model)
	mm.input.SetValue("han@example.com")
	saved, cmd := mm.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("submitting a new account should issue a write")
	}
	if got := saved.(Model); got.mode != modeDetail || !strings.Contains(got.status, "saving") {
		t.Errorf("mode = %v, status = %q; want a save in progress", got.mode, got.status)
	}

	// Nothing typed: cancels rather than clearing the field.
	blank := testModel("wifi")
	out2, _ := blank.openSetAccount(entry, modeDetail)
	mm2 := out2.(Model)
	mm2.input.SetValue("")
	done, cmd2 := mm2.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd2 != nil {
		t.Error("an empty prompt must not write anything")
	}
	if done.(Model).mode != modeDetail {
		t.Errorf("mode = %v, want the prompt closed", done.(Model).mode)
	}
}

func TestSetAccountConfirmingAnInferredValueStillWrites(t *testing.T) {
	// Accepting the pre-filled value is how an inferred account becomes a
	// recorded one, so it must not be short-circuited as "unchanged".
	for _, raw := range []struct{ name, text string }{
		{"web/reddit", "pw\nhan@example.com\n"}, // inferred from a note
		{"web/x/han@example.com", "pw\n"},       // inferred from the name
	} {
		entry := pass.ParseEntry(raw.name, raw.text)
		m := testModel(raw.name)
		m.store = nil
		out, _ := m.openSetAccount(entry, modeDetail)
		mm := out.(Model)
		if mm.input.Value() != "han@example.com" {
			t.Fatalf("%s: prompt = %q", raw.name, mm.input.Value())
		}
		// Press enter on the pre-filled value, changing nothing.
		saved, cmd := mm.keyEditField(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Errorf("%s: confirming an inferred account should write a field", raw.name)
		}
		if got := saved.(Model).status; !strings.Contains(got, "saving") {
			t.Errorf("%s: status = %q, want a save in progress", raw.name, got)
		}
	}

	// A value already in a field, resubmitted unchanged, writes nothing.
	entry := pass.ParseEntry("x", "pw\nlogin: han\n")
	m := testModel("x")
	out, _ := m.openSetAccount(entry, modeDetail)
	mm := out.(Model)
	done, cmd := mm.keyEditField(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("resubmitting an unchanged field should not rewrite the entry")
	}
	if got := done.(Model).status; !strings.Contains(got, "unchanged") {
		t.Errorf("status = %q, want it reported as unchanged", got)
	}
}

func TestDetailRowsFollowFileOrderAndCarryTheirLine(t *testing.T) {
	// Row order has to match the file, because that correspondence is what lets
	// `e` edit the line the cursor is actually on.
	entry := pass.ParseEntry("x", "pw\na note\nurl: https://example.com\nlogin: han\n")
	m := testModel("x")
	m.entry = &entry

	rows := m.detailRows()
	want := []struct {
		label string
		line  int
	}{
		{"password", 0},
		{"note 1", 1},
		{"url", 2},
		{"login", 3},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %d rows in file order", rows, len(want))
	}
	for i, w := range want {
		if rows[i].label != w.label || rows[i].line != w.line {
			t.Errorf("row %d = %q@%d, want %q@%d", i, rows[i].label, rows[i].line, w.label, w.line)
		}
	}
}

func TestEditKeyEditsTheFocusedField(t *testing.T) {
	entry := pass.ParseEntry("web/github", "hunter2\nlogin: han\nurl: https://github.com\n")
	m := testModel("web/github")
	m.cfg.Keys.Edit = "e"
	m.entry = &entry
	m.mode = modeDetail
	m.entryCursor = 2 // the url row

	out, _ := m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	got := out.(Model)
	if got.mode != modeEditField {
		t.Fatalf("mode = %v, want the field editor", got.mode)
	}
	if got.field == nil || got.field.label != "url" || got.field.line != 2 {
		t.Fatalf("field = %+v, want the url row at line 2", got.field)
	}
	if got.input.Value() != "https://github.com" {
		t.Errorf("prompt = %q, want the url pre-filled", got.input.Value())
	}

	got.input.SetValue("https://gitlab.com")
	saved, cmd := got.keyEditField(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !strings.Contains(saved.(Model).status, "saving") {
		t.Errorf("status = %q, cmd = %v; want a save", saved.(Model).status, cmd != nil)
	}
	// The write must change only that line.
	content, err := entry.SetValueAt(2, "https://gitlab.com")
	if err != nil {
		t.Fatal(err)
	}
	if want := "hunter2\nlogin: han\nurl: https://gitlab.com\n"; content != want {
		t.Errorf("content =\n%q\nwant\n%q", content, want)
	}
}

func TestEditKeyMasksASecretFieldAndGeneratesOnlyForThePassword(t *testing.T) {
	entry := pass.ParseEntry("x", "hunter2\notp: JBSWY3DPEHPK3PXP\n")
	m := testModel("x")
	m.cfg.Generate.Length = 24
	m.entry = &entry
	m.mode = modeDetail

	// The otp row is a secret, so typing into it stays masked.
	m.entryCursor = 1
	out, _ := m.openEditField(entry, m.detailRows()[1], modeDetail)
	secret := out.(Model)
	if !secret.field.secret || secret.input.EchoMode != textinput.EchoPassword {
		t.Errorf("otp row: secret = %v, echo = %v; want a masked prompt", secret.field.secret, secret.input.EchoMode)
	}
	// ctrl+g must not fabricate a value for a non-password line.
	noGen, _ := secret.keyEditField(tea.KeyMsg{Type: tea.KeyCtrlG})
	if noGen.(Model).input.Value() != "JBSWY3DPEHPK3PXP" {
		t.Errorf("ctrl+g changed a non-password field to %q", noGen.(Model).input.Value())
	}

	// On the password row it does generate, and reveals what it made.
	out2, _ := m.openEditField(entry, m.detailRows()[0], modeDetail)
	gen, _ := out2.(Model).keyEditField(tea.KeyMsg{Type: tea.KeyCtrlG})
	g := gen.(Model)
	if len(g.input.Value()) != 24 || g.input.Value() == "hunter2" {
		t.Errorf("generated password = %q, want 24 fresh characters", g.input.Value())
	}
	if g.input.EchoMode != textinput.EchoNormal {
		t.Error("a generated password should be shown so it can be checked")
	}
}

func TestEditKeyOnADerivedAccountRowRegistersIt(t *testing.T) {
	// That row has no line in the file, so editing it has to add a field.
	entry := pass.ParseEntry("web/x/han@example.com", "pw\n")
	m := testModel("web/x/han@example.com")
	m.entry = &entry
	m.mode = modeDetail

	rows := m.detailRows()
	out, _ := m.openEditField(entry, rows[1], modeDetail)
	got := out.(Model)
	if got.field == nil || !got.field.account {
		t.Fatalf("field = %+v, want the account path", got.field)
	}
	if got.input.Value() != "han@example.com" {
		t.Errorf("prompt = %q, want the derived account", got.input.Value())
	}
}

func TestEditKeyUnchangedValueWritesNothing(t *testing.T) {
	entry := pass.ParseEntry("x", "pw\nurl: https://example.com\n")
	m := testModel("x")
	m.entry = &entry
	out, _ := m.openEditField(entry, m.detailRows()[1], modeDetail)

	done, cmd := out.(Model).keyEditField(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("resubmitting an unchanged value should not rewrite the entry")
	}
	if got := done.(Model).status; !strings.Contains(got, "unchanged") {
		t.Errorf("status = %q, want it reported as unchanged", got)
	}
}

func TestEditFileKeyStillOpensEditorFromAnOpenEntry(t *testing.T) {
	// E must keep handing the whole file to $EDITOR now that e edits one field.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte("test@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := pass.Open(dir)
	if err != nil {
		t.Skip("pass is not installed")
	}

	entry := pass.ParseEntry("x", "pw\n")
	m := testModel("x")
	m.store = store
	m.cfg.Keys.Edit, m.cfg.Keys.EditFile = "e", "E"
	m.entry = &entry
	m.mode = modeDetail

	out, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if got := out.(Model); got.mode != modeDetail || got.field != nil {
		t.Errorf("mode = %v, field = %+v; E should hand off to $EDITOR, not prompt", got.mode, got.field)
	}
	if cmd == nil {
		t.Error("E should issue the $EDITOR hand-off")
	}
}

func TestIdleLockClosesTheAccountPrompt(t *testing.T) {
	// The prompt rewrites m.entry; once the lock drops that, the prompt cannot
	// be submitted and must not stay on screen.
	entry := pass.ParseEntry("wifi", "only-a-password\n")
	m := testModel("wifi")
	m.cfg.Security.LockSeconds = 1
	out, _ := m.openSetAccount(entry, modeDetail)
	mm := out.(Model)
	mm.lockLeft = 1

	locked, _ := mm.onTick()
	got := locked.(Model)
	if got.mode != modeList || got.entry != nil || got.field != nil {
		t.Errorf("mode = %v, entry = %v, field = %+v; want everything released",
			got.mode, got.entry, got.field)
	}
}

func TestCopyUsernameReportsTheFieldsThatWouldWork(t *testing.T) {
	entry := pass.ParseEntry("wifi", "only-a-password\n")
	m := testModel()

	out, _ := m.copyUsername(entry)
	got := out.(Model)
	if !got.statusErr {
		t.Fatalf("status = %q, want an error for an entry with no account", got.status)
	}
	for _, want := range []string{"wifi", "login", "email"} {
		if !strings.Contains(got.status, want) {
			t.Errorf("status = %q, want it to mention %q", got.status, want)
		}
	}
}

func TestUsernameLabelNamesAnInferredSource(t *testing.T) {
	for source, want := range map[pass.UsernameSource]string{
		pass.UsernameField: "username",
		pass.UsernameNote:  "username (from note)",
		pass.UsernameName:  "username (from name)",
	} {
		if got := usernameLabel(source); got != want {
			t.Errorf("usernameLabel(%v) = %q, want %q", source, got, want)
		}
	}
}

func TestDetailRowsEmptyWithoutAnEntry(t *testing.T) {
	m := testModel()
	if rows := m.detailRows(); rows != nil {
		t.Errorf("detailRows = %+v with no decrypted entry, want nil", rows)
	}
}

func TestMaskOfHidesLength(t *testing.T) {
	short := maskOf("abc")
	long := maskOf(strings.Repeat("x", 90))

	if len([]rune(short)) != 3 {
		t.Errorf("short mask = %q, want 3 dots", short)
	}
	if n := len([]rune(long)); n != 12 {
		t.Errorf("long mask has %d dots; it should cap so the length is not leaked", n)
	}
	if maskOf("") != "" {
		t.Error("an empty value should mask to nothing at all")
	}
}

func TestIsSecretField(t *testing.T) {
	for _, key := range []string{"otp", "TOTP", "api_token", "recovery", "pin", "secret key"} {
		if !isSecretField(key) {
			t.Errorf("isSecretField(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"login", "url", "email", "note"} {
		if isSecretField(key) {
			t.Errorf("isSecretField(%q) = true, want false", key)
		}
	}
}

// commandLineAfter types raw into the command line, then presses keys, and
// returns the model so both the input and the completion state can be checked.
func commandLineAfter(t *testing.T, raw string, keys ...tea.KeyMsg) Model {
	t.Helper()
	m := testModel("web/github")
	m.mode, m.prevMode = modeCommand, modeList
	m.input.SetValue(raw)
	m.input.CursorEnd()
	for _, key := range keys {
		model, _ := m.keyCommand(key)
		m = model.(Model)
	}
	return m
}

var (
	tabKey      = tea.KeyMsg{Type: tea.KeyTab}
	shiftTabKey = tea.KeyMsg{Type: tea.KeyShiftTab}
	backspace   = tea.KeyMsg{Type: tea.KeyBackspace}
)

func TestTabCompletesCommandNames(t *testing.T) {
	cases := []struct {
		typed string
		keys  []tea.KeyMsg
		want  string
	}{
		// Only one command starts with "hel", so Tab finishes it and leaves the
		// cursor past a space, ready for an argument.
		{"hel", []tea.KeyMsg{tabKey}, "help "},
		{"co", []tea.KeyMsg{tabKey}, "config "},
		{"zzz", []tea.KeyMsg{tabKey}, "zzz"},
		// Once an argument is being typed, the command name is settled.
		{"theme pur", []tea.KeyMsg{tabKey}, "theme pur"},
		// "re" is shared by reload and rename, so there is nothing to extend:
		// the first Tab offers a candidate and the next walks on.
		{"re", []tea.KeyMsg{tabKey}, "reload"},
		{"re", []tea.KeyMsg{tabKey, tabKey}, "rename"},
		{"re", []tea.KeyMsg{tabKey, tabKey, tabKey}, "reload"}, // wraps around
		{"re", []tea.KeyMsg{shiftTabKey}, "rename"},            // cycles backwards
		{"re", []tea.KeyMsg{tabKey, shiftTabKey}, "rename"},    // and wraps backwards
		{"a", []tea.KeyMsg{tabKey}, "account"},
		{"a", []tea.KeyMsg{tabKey, tabKey}, "add"},
		// Editing restarts completion rather than resuming the stale cycle:
		// "reloa" is unambiguous again, so this Tab completes instead of
		// stepping on to rename.
		{"re", []tea.KeyMsg{tabKey, backspace, tabKey}, "reload "},
		// Nothing typed at all: Tab offers the first command.
		{"", []tea.KeyMsg{tabKey}, "account"},
		{"", []tea.KeyMsg{shiftTabKey}, "theme"},
	}

	for _, c := range cases {
		m := commandLineAfter(t, c.typed, c.keys...)
		if got := m.input.Value(); got != c.want {
			t.Errorf("%q + %d key(s) = %q, want %q", c.typed, len(c.keys), got, c.want)
		}
	}
}

// A cycle must not outlive the prefix it was built for.
func TestTypingEndsTheCompletionCycle(t *testing.T) {
	if m := commandLineAfter(t, "re", tabKey); m.completion == nil {
		t.Fatal("Tab on an ambiguous prefix should leave a cycle to walk")
	}
	if m := commandLineAfter(t, "re", tabKey, backspace); m.completion != nil {
		t.Error("editing the command line should drop the candidate list")
	}
	// An unambiguous completion has nothing left to cycle through.
	if m := commandLineAfter(t, "hel", tabKey); m.completion != nil {
		t.Error("completing a single match should not leave a cycle behind")
	}
}

func TestCommonPrefix(t *testing.T) {
	cases := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"generate"}, "generate"},
		{[]string{"generate", "gen"}, "gen"},
		{[]string{"reload", "rename"}, "re"},
		{[]string{"add", "theme"}, ""},
	}
	for _, c := range cases {
		if got := commonPrefix(c.names); got != c.want {
			t.Errorf("commonPrefix(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}

// The candidate list shares the single prompt line with the input, so it must
// never widen that line past the terminal — the layout has no room to spare.
func TestCompletionMenuFitsThePromptLine(t *testing.T) {
	for _, width := range []int{14, 20, 40, 80, 120} {
		m := commandLineAfter(t, "", tabKey)
		m.width = width
		if m.completion == nil {
			t.Fatal("Tab on an empty command line should list every command")
		}
		line := m.renderCommandLine()
		if got := lipgloss.Width(line); got > width-2 {
			t.Errorf("width %d: command line renders %d cells wide", width, got)
		}
	}

	// With room to spare, every candidate is listed and the active one is on
	// the line exactly once more than the others (it is in the input too).
	m := commandLineAfter(t, "re", tabKey)
	m.width = 120
	plain := stripANSI(m.renderCommandLine())
	for _, name := range []string{"reload", "rename"} {
		if !strings.Contains(plain, name) {
			t.Errorf("candidate %q missing from %q", name, plain)
		}
	}
	if n := strings.Count(plain, "reload"); n != 2 {
		t.Errorf("active candidate appears %d times in %q, want twice", n, plain)
	}
}

func TestListRowsLeavesRoomForChrome(t *testing.T) {
	m := testModel("a")
	m.height = 30

	plain := m.listRows()
	m.query = "a"
	filtered := m.listRows()

	if filtered != plain-1 {
		t.Errorf("listRows with a filter = %d, want one less than %d (the prompt takes a line)", filtered, plain)
	}
	if plain >= m.height {
		t.Errorf("listRows = %d, which leaves no room for the header, hints, and status bar", plain)
	}
}

// Every screen must fit the terminal exactly. The prompt line is the last
// thing drawn above the hint bar, so a layout that over-fills the screen loses
// it first — which once made the delete confirmation invisible.
func TestViewsFitTheTerminalAndKeepThePrompt(t *testing.T) {
	names := make([]string, 40)
	for i := range names {
		names[i] = fmt.Sprintf("folder%d/entry%d", i/5, i)
	}
	entry := pass.ParseEntry("folder0/entry0", "pw\nlogin: han\nurl: https://x\nnote\n")

	cases := []struct {
		name   string
		setup  func(m *Model)
		prompt string
	}{
		{"list", func(m *Model) {}, ""},
		{"filter", func(m *Model) { m.query = "entry"; m.applyFilter() }, "(esc to clear)"},
		{"delete", func(m *Model) { m.mode = modeConfirmDelete; m.confirmDelete = "folder0/entry0" }, "y to confirm"},
		{"rename", func(m *Model) { m.mode = modeRename }, "rename →"},
		{"command", func(m *Model) { m.mode = modeCommand; m.prevMode = modeList }, " : "},
		{"detail", func(m *Model) { m.entry = &entry; m.mode = modeDetail }, ""},
		{"detail command", func(m *Model) {
			m.entry = &entry
			m.mode, m.prevMode = modeCommand, modeDetail
		}, " : "},
		{"set account", func(m *Model) {
			m.entry = &entry
			m.mode = modeEditField
			m.field = &fieldPrompt{name: entry.Name, from: modeDetail, label: "account", line: accountRow, account: true}
		}, "account →"},
		{"edit field", func(m *Model) {
			m.entry = &entry
			m.mode = modeEditField
			m.field = &fieldPrompt{name: entry.Name, from: modeDetail, label: "url", line: 2}
		}, "url →"},
		{"edit password", func(m *Model) {
			m.entry = &entry
			m.mode = modeEditField
			m.field = &fieldPrompt{name: entry.Name, from: modeDetail, label: "password", line: 0, secret: true}
		}, "ctrl+g generates"},
	}
	for _, c := range cases {
		for _, h := range []int{12, 30} {
			m := testModel(names...)
			m.height = h
			m.cursor = 30
			c.setup(&m)

			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Errorf("%s at height %d: view is %d lines", c.name, h, len(lines))
			}
			plain := stripANSI(m.View())
			if c.prompt != "" && !strings.Contains(plain, c.prompt) {
				t.Errorf("%s at height %d: prompt %q is not on screen", c.name, h, c.prompt)
			}
			// Cases that open the detail panel give up list rows for it, so the
			// cursor's entry is not guaranteed to still be on screen.
			if !strings.HasPrefix(c.name, "edit") && c.name != "set account" &&
				c.name != "detail" && c.name != "detail command" && !strings.Contains(plain, "entry30") {
				t.Errorf("%s at height %d: the cursor's entry is not on screen", c.name, h)
			}
		}
	}
}

func TestListNamesEachFolderOncePerGroup(t *testing.T) {
	m := testModel("bank/chase", "bank/schwab", "web/github", "wifi")
	m.cursor = 3
	plain := stripANSI(m.View())

	if n := strings.Count(plain, "bank"); n != 1 {
		t.Errorf("bank is named %d times, want once for its group", n)
	}
	if !strings.Contains(plain, "–") {
		t.Error("a root entry should show a dash, not a blank that reads as 'same folder'")
	}

	// Filtered results are ranked, not grouped: every row names its folder.
	m.query = "a"
	m.applyFilter()
	if n := strings.Count(stripANSI(m.View()), "bank"); n != 2 {
		t.Errorf("bank is named %d times in search results, want every row", n)
	}
}

func TestSplitName(t *testing.T) {
	cases := map[string][2]string{
		"wifi":            {"", "wifi"},
		"web/github":      {"web", "github"},
		"web/deep/nested": {"web/deep", "nested"},
	}
	for in, want := range cases {
		if f, e := splitName(in); f != want[0] || e != want[1] {
			t.Errorf("splitName(%q) = %q, %q; want %q, %q", in, f, e, want[0], want[1])
		}
	}
}

func TestTextOnPicksAReadableColor(t *testing.T) {
	if got := textOn("#F2CC60"); got == "#FFFFFF" {
		t.Error("white text on dark's pale yellow accent is unreadable")
	}
	if got := textOn("#0F766E"); got != "#FFFFFF" {
		t.Errorf("textOn(deep teal) = %s, want white", got)
	}
	if got := textOn("bogus"); got != "#FFFFFF" {
		t.Errorf("textOn(bogus) = %s, want the white fallback", got)
	}
}

func stripANSI(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]").ReplaceAllString(s, "")
}

func TestListRowsStaysPositiveOnTinyTerminals(t *testing.T) {
	m := testModel("a")
	for _, h := range []int{0, 1, 3, 5} {
		m.height = h
		if got := m.listRows(); got < 1 {
			t.Errorf("listRows() = %d at height %d, want at least 1", got, h)
		}
	}
}

func TestHumanTime(t *testing.T) {
	if got := humanTime(time.Time{}); got != "" {
		t.Errorf("humanTime(zero) = %q, want empty", got)
	}
	if got := humanTime(time.Now()); !strings.Contains(got, ":") {
		t.Errorf("humanTime(now) = %q, want a clock time for today", got)
	}
	old := time.Date(2001, 3, 4, 5, 6, 0, 0, time.Local)
	if got := humanTime(old); got != "2001-03-04" {
		t.Errorf("humanTime(2001) = %q, want a full date", got)
	}
}

func TestDetailRowsMasksOTPAuthURI(t *testing.T) {
	// pass-otp's on-disk format: a bare URI whose query carries the seed.
	entry := pass.ParseEntry("x", "pw\notpauth://totp/GitHub:han?secret=JBSWY3DPEHPK3PXP\n")
	m := testModel()
	m.entry = &entry

	rows := m.detailRows()
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want password plus the otp URI", rows)
	}
	if !rows[1].secret {
		t.Errorf("otpauth URI row %+v is not masked; it contains the TOTP seed", rows[1])
	}
}

func TestClearOnExitWipesAPendingSecret(t *testing.T) {
	m := testModel()
	m.clipValue = "hunter2"

	var cleared []string
	record := func(expect string) error { cleared = append(cleared, expect); return nil }

	if err := clearOnExit(m, record); err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 1 || cleared[0] != "hunter2" {
		t.Errorf("cleared = %q, want the pending secret wiped once", cleared)
	}

	// Nothing pending (never copied, already cleared, or auto-clear disabled).
	cleared = nil
	if err := clearOnExit(testModel(), record); err != nil || len(cleared) != 0 {
		t.Errorf("cleared = %q, err = %v; want no clipboard access", cleared, err)
	}
	if err := clearOnExit(nil, record); err != nil || len(cleared) != 0 {
		t.Errorf("a failed run must not touch the clipboard: cleared = %q, err = %v", cleared, err)
	}
}

func TestStaleOpenDoesNotLeaveADialog(t *testing.T) {
	m := testModel("a")
	m.mode = modeAdd
	m.add = &addState{}

	next, _ := m.Update(entryLoadedMsg{name: "a", entry: pass.Entry{Name: "a", Password: "pw"}, intent: intentOpen})
	got := next.(Model)
	if got.mode != modeAdd || got.entry != nil {
		t.Errorf("mode = %v, entry = %v; a late decrypt must not replace the add dialog", got.mode, got.entry)
	}
}

func TestStaleRefreshDoesNotReopenAClosedEntry(t *testing.T) {
	m := testModel("a")
	m.mode = modeList // the user closed the entry while `pass edit` was saving

	next, _ := m.Update(entryLoadedMsg{name: "a", entry: pass.Entry{Name: "a", Password: "pw"}, intent: intentRefresh})
	got := next.(Model)
	if got.mode != modeList || got.entry != nil {
		t.Errorf("mode = %v, entry = %v; a refresh must not bring plaintext back", got.mode, got.entry)
	}
}

func TestOpenShowsTheEntry(t *testing.T) {
	m := testModel("a")
	next, _ := m.Update(entryLoadedMsg{name: "a", entry: pass.Entry{Name: "a", Password: "pw"}, intent: intentOpen})
	got := next.(Model)
	if got.mode != modeDetail || got.entry == nil || got.entry.Name != "a" {
		t.Errorf("mode = %v, entry = %v; want the detail view for a", got.mode, got.entry)
	}
}

// Exercise ANSI output: plain-text snapshots cannot detect conflicting colors.
func TestThemeCanvasAndSelectedSearchContrast(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, name := range config.ThemePresetNames() {
		t.Run(name, func(t *testing.T) {
			theme, _ := config.PresetTheme(name)
			m := testModel("web/github")
			m.cfg.Theme = theme
			m.styles = buildStyles(theme)
			m.query = "git"
			m.applyFilter()
			row, _ := m.renderListRow(0, m.listColumns(), m.panelInnerWidth(), false, "")
			// Search hits keep the selection foreground instead of restoring the accent.
			accent := bgOpenSeq(m.styles.Accent)
			if accent != "" && strings.Contains(row, accent) {
				t.Fatal("selected search result contains the normal accent color")
			}
			if !strings.Contains(row, bgOpenSeq(m.styles.Selection)) {
				t.Fatal("selected row has no selection colors")
			}
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != m.height {
				t.Fatalf("height = %d, want %d", len(lines), m.height)
			}
			for _, line := range lines {
				if lipgloss.Width(line) != m.width-2 {
					t.Fatalf("line width = %d", lipgloss.Width(line))
				}
				if !strings.Contains(line, bgOpenSeq(m.styles.Base)) {
					t.Fatal("canvas colors missing")
				}
			}
		})
	}
}
