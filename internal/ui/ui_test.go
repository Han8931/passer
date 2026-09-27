package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"passer/internal/config"
	"passer/internal/pass"
)

func testModel(names ...string) Model {
	cfg := config.Config{}
	items := make([]pass.Item, len(names))
	for i, n := range names {
		items[i] = pass.Item{Name: n, Modified: time.Now()}
	}
	m := Model{
		cfg:    cfg,
		items:  items,
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

func TestCompleteCommand(t *testing.T) {
	if got := completeCommand("hel"); got != "help " {
		t.Errorf("completeCommand(hel) = %q, want %q", got, "help ")
	}
	// "re" is ambiguous between reload and rename, so it extends only to the
	// shared prefix.
	if got := completeCommand("re"); got != "re" {
		t.Errorf("completeCommand(re) = %q, want the ambiguous prefix unchanged", got)
	}
	if got := completeCommand("zzz"); got != "zzz" {
		t.Errorf("completeCommand(zzz) = %q, want it left alone", got)
	}
	// Once an argument is being typed, the command name is settled.
	if got := completeCommand("theme pur"); got != "theme pur" {
		t.Errorf("completeCommand(theme pur) = %q, want it left alone", got)
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
			if c.name != "detail" && c.name != "detail command" && !strings.Contains(plain, "entry30") {
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
