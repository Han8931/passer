package ui

import (
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
