package pass

import (
	"strings"
	"testing"
)

func TestParseEntry(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		password string
		fields   []Field
		notes    []string
	}{
		{
			name:     "password only",
			raw:      "hunter2\n",
			password: "hunter2",
		},
		{
			name:     "password with metadata",
			raw:      "hunter2\nlogin: han\nurl: https://example.com\n",
			password: "hunter2",
			fields:   []Field{{"login", "han"}, {"url", "https://example.com"}},
		},
		{
			name:     "bare url is a note, not a field named https",
			raw:      "hunter2\nhttps://example.com\n",
			password: "hunter2",
			notes:    []string{"https://example.com"},
		},
		{
			name:     "key without space separator",
			raw:      "hunter2\nlogin:han\n",
			password: "hunter2",
			fields:   []Field{{"login", "han"}},
		},
		{
			name:     "multi-word key needs the conventional separator",
			raw:      "hunter2\nrecovery code: abcd\n",
			password: "hunter2",
			fields:   []Field{{"recovery code", "abcd"}},
		},
		{
			name:     "prose with a colon stays a note",
			raw:      "hunter2\nremember this: the answer is no\n",
			password: "hunter2",
			fields:   []Field{{"remember this", "the answer is no"}},
		},
		{
			name:     "path-like prefix is a note",
			raw:      "hunter2\nsee /etc/hosts: line 3\n",
			password: "hunter2",
			notes:    []string{"see /etc/hosts: line 3"},
		},
		{
			name:     "trailing newline does not become an empty note",
			raw:      "hunter2\nlogin: han\n\n",
			password: "hunter2",
			fields:   []Field{{"login", "han"}},
		},
		{
			name: "empty input",
			raw:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseEntry("x", tc.raw)
			if got.Password != tc.password {
				t.Errorf("password = %q, want %q", got.Password, tc.password)
			}
			if len(got.Fields) != len(tc.fields) {
				t.Fatalf("fields = %+v, want %+v", got.Fields, tc.fields)
			}
			for i, f := range got.Fields {
				if f != tc.fields[i] {
					t.Errorf("field %d = %+v, want %+v", i, f, tc.fields[i])
				}
			}
			if len(got.Notes) != len(tc.notes) {
				t.Fatalf("notes = %q, want %q", got.Notes, tc.notes)
			}
			for i, n := range got.Notes {
				if n != tc.notes[i] {
					t.Errorf("note %d = %q, want %q", i, n, tc.notes[i])
				}
			}
		})
	}
}

func TestEntryUsernamePrefersConfiguredField(t *testing.T) {
	e := ParseEntry("x", "pw\nlogin: fallback\nid: preferred\n")

	if got, _ := e.Username(nil); got != "fallback" {
		t.Errorf("Username(nil) = %q, want the built-in login field", got)
	}
	if got, _ := e.Username([]string{"id"}); got != "preferred" {
		t.Errorf("Username([id]) = %q, want the configured field to win", got)
	}
}

func TestEntryLookupSkipsEmptyValues(t *testing.T) {
	e := ParseEntry("x", "pw\nlogin:\nusername: han\n")
	got, ok := e.Username(nil)
	if !ok || got != "han" {
		t.Errorf("Username = %q/%v, want han/true", got, ok)
	}
}

func TestEntryUsernameFallsBackToABareAddressLine(t *testing.T) {
	e := ParseEntry("email/fastmail", "s3cret\nhan@example.com\nThis line is a free-form note.\n")

	got, source, ok := e.UsernameFrom(nil)
	if !ok || got != "han@example.com" || source != UsernameNote {
		t.Errorf("UsernameFrom = %q/%v/%v, want han@example.com/UsernameNote/true", got, source, ok)
	}
}

func TestEntryUsernameFallsBackToTheEntryName(t *testing.T) {
	e := ParseEntry("web/github.com/han@example.com", "hunter2\n")

	got, source, ok := e.UsernameFrom(nil)
	if !ok || got != "han@example.com" || source != UsernameName {
		t.Errorf("UsernameFrom = %q/%v/%v, want han@example.com/UsernameName/true", got, source, ok)
	}
}

func TestEntryUsernamePrefersAFieldOverEveryFallback(t *testing.T) {
	// All three conventions present at once: the labelled field is the only one
	// the user stated outright, so it has to win.
	e := ParseEntry("web/github.com/wrong@example.com", "hunter2\nlogin: right\nalso@example.com\n")

	got, source, ok := e.UsernameFrom(nil)
	if !ok || got != "right" || source != UsernameField {
		t.Errorf("UsernameFrom = %q/%v/%v, want right/UsernameField/true", got, source, ok)
	}
}

// The fallbacks must not turn a TOTP seed into "the username": pass-otp stores
// it as a bare note that can carry an "@" and never any whitespace.
func TestEntryUsernameIgnoresAnOTPAuthURI(t *testing.T) {
	e := ParseEntry("web/github", "hunter2\notpauth://totp/GitHub:han@example.com?secret=JBSWY3DPEHPK3PXP&issuer=GitHub\n")

	if got, source, ok := e.UsernameFrom(nil); ok {
		t.Errorf("UsernameFrom = %q/%v/%v, want no match", got, source, ok)
	}
}

func TestLooksLikeAddress(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"han@example.com", true},
		{"han.doe+tag@mail.example.co.uk", true},
		{"han@localhost", true},
		{"", false},
		{"han", false},              // no @ at all
		{"github.com", false},       // a bare site name, the usual leaf
		{"@example.com", false},     // nothing before the @
		{"han@", false},             // nothing after it
		{"a@b@c", false},            // two @, not an address
		{"han@.example.com", false}, // malformed domain
		{"han@example.com.", false},
		{"mail han@example.com now", false}, // prose mentioning an address
		{"https://u@example.com/x", false},  // a URL, not an account
		{"GitHub:han@example.com", false},   // an otpauth label
	} {
		if got := looksLikeAddress(tc.in); got != tc.want {
			t.Errorf("looksLikeAddress(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestUsernameKeysPutsConfiguredFieldsFirstWithoutDuplicates(t *testing.T) {
	got := UsernameKeys([]string{"id", "Login"})
	want := []string{"id", "Login", "username", "user", "email", "account"}

	if len(got) != len(want) {
		t.Fatalf("UsernameKeys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("UsernameKeys = %v, want %v", got, want)
		}
	}
}

func TestEntryURL(t *testing.T) {
	e := ParseEntry("x", "pw\nsite: https://example.com\n")
	if got, ok := e.URL(); !ok || got != "https://example.com" {
		t.Errorf("URL = %q/%v", got, ok)
	}

	none := ParseEntry("x", "pw\n")
	if _, ok := none.URL(); ok {
		t.Error("URL should report false when no url-like field exists")
	}
}

func TestSetUsernameAddsAFieldUnderThePassword(t *testing.T) {
	e := ParseEntry("wifi", "only-a-password\n")

	got := e.SetUsername("han@example.com", UsernameKeys(nil))
	want := "only-a-password\nlogin: han@example.com\n"
	if got != want {
		t.Errorf("SetUsername =\n%q\nwant\n%q", got, want)
	}
}

func TestSetUsernameUpdatesAnExistingFieldInPlace(t *testing.T) {
	// The key the user chose is theirs: "username:" must not become "login:",
	// and the line must stay where it was.
	e := ParseEntry("x", "pw\nurl: https://example.com\nusername: old\nnote\n")

	got := e.SetUsername("new@example.com", UsernameKeys(nil))
	want := "pw\nurl: https://example.com\nusername: new@example.com\nnote\n"
	if got != want {
		t.Errorf("SetUsername =\n%q\nwant\n%q", got, want)
	}
}

func TestSetUsernameUsesTheFirstConfiguredKey(t *testing.T) {
	e := ParseEntry("x", "pw\n")

	got := e.SetUsername("han", UsernameKeys([]string{"acct"}))
	if want := "pw\nacct: han\n"; got != want {
		t.Errorf("SetUsername = %q, want %q", got, want)
	}
}

// The whole point of rewriting raw lines: a hand-written file keeps its shape.
// Compose would group both fields above the note and reorder the entry.
func TestSetUsernameKeepsTheUsersLineOrder(t *testing.T) {
	raw := "pw\na free-form note\nurl: https://example.com\nanother note\n"
	e := ParseEntry("x", raw)

	got := e.SetUsername("han", UsernameKeys(nil))
	want := "pw\nlogin: han\na free-form note\nurl: https://example.com\nanother note\n"
	if got != want {
		t.Errorf("SetUsername =\n%q\nwant\n%q", got, want)
	}
	if composed := e.Compose(); composed == want {
		t.Error("test is vacuous: Compose would have produced the same thing")
	}
}

func TestSetUsernamePromotesABareAddressLine(t *testing.T) {
	// Registering the address that was already sitting on its own line should
	// label it, not duplicate it.
	e := ParseEntry("x", "pw\nhan@example.com\nkeep this note\n")

	got := e.SetUsername("han@example.com", UsernameKeys(nil))
	want := "pw\nlogin: han@example.com\nkeep this note\n"
	if got != want {
		t.Errorf("SetUsername =\n%q\nwant\n%q", got, want)
	}
}

func TestSetUsernameKeepsAnUnrelatedBareAddress(t *testing.T) {
	// Only the line that *is* the new value is absorbed; a different address is
	// someone else's data and stays put.
	e := ParseEntry("x", "pw\nbilling@example.com\n")

	got := e.SetUsername("han@example.com", UsernameKeys(nil))
	want := "pw\nlogin: han@example.com\nbilling@example.com\n"
	if got != want {
		t.Errorf("SetUsername =\n%q\nwant\n%q", got, want)
	}
}

func TestSetUsernamePreservesTheOTPSeed(t *testing.T) {
	seed := "otpauth://totp/GitHub:han@example.com?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"
	e := ParseEntry("web/github", "pw\n"+seed+"\n")

	got := e.SetUsername("han@example.com", UsernameKeys(nil))
	if !strings.Contains(got, seed) {
		t.Errorf("SetUsername dropped the TOTP seed:\n%q", got)
	}
}

func TestSetUsernameRoundTripsThroughParse(t *testing.T) {
	e := ParseEntry("x", "hunter2\nurl: https://example.com\nfree text\n")

	back := ParseEntry("x", e.SetUsername("han@example.com", UsernameKeys(nil)))
	if back.Password != "hunter2" {
		t.Errorf("password = %q, want it untouched", back.Password)
	}
	got, source, ok := back.UsernameFrom(nil)
	if !ok || got != "han@example.com" || source != UsernameField {
		t.Errorf("UsernameFrom = %q/%v/%v, want the registered field", got, source, ok)
	}
	if url, _ := back.URL(); url != "https://example.com" {
		t.Errorf("url = %q, want it preserved", url)
	}
	if len(back.Notes) != 1 || back.Notes[0] != "free text" {
		t.Errorf("notes = %q, want the free text preserved", back.Notes)
	}
}

func TestSetValueAt(t *testing.T) {
	raw := "hunter2\na free-form note\nurl: https://example.com\nlogin: han\n"

	for _, tc := range []struct {
		name  string
		index int
		value string
		want  string
	}{
		{"password", 0, "new-password",
			"new-password\na free-form note\nurl: https://example.com\nlogin: han\n"},
		{"note replaced whole", 1, "rewritten note",
			"hunter2\nrewritten note\nurl: https://example.com\nlogin: han\n"},
		{"field keeps its key", 2, "https://gitlab.com",
			"hunter2\na free-form note\nurl: https://gitlab.com\nlogin: han\n"},
		{"last line", 3, "someone@example.com",
			"hunter2\na free-form note\nurl: https://example.com\nlogin: someone@example.com\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEntry("x", raw).SetValueAt(tc.index, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("SetValueAt(%d, %q) =\n%q\nwant\n%q", tc.index, tc.value, got, tc.want)
			}
		})
	}
}

func TestSetValueAtRejectsALineOutsideTheEntry(t *testing.T) {
	e := ParseEntry("x", "pw\nlogin: han\n")
	for _, index := range []int{-1, 2, 99} {
		if _, err := e.SetValueAt(index, "whatever"); err == nil {
			t.Errorf("SetValueAt(%d) accepted a line the entry does not have", index)
		}
	}
}

// A field's key may be spelled with a space before the colon; only the value is
// the user's to change here, so the key survives in its own spelling.
func TestSetValueAtPreservesAnOddKeySpelling(t *testing.T) {
	e := ParseEntry("x", "pw\nrecovery codes: 1111 2222\n")

	got, err := e.SetValueAt(1, "3333 4444")
	if err != nil {
		t.Fatal(err)
	}
	if want := "pw\nrecovery codes: 3333 4444\n"; got != want {
		t.Errorf("SetValueAt = %q, want %q", got, want)
	}
}

func TestParseEntryRecordsLinePositions(t *testing.T) {
	e := ParseEntry("x", "pw\nnote\nurl: https://example.com\n")

	want := []Line{
		{Index: 1, Value: "note"},
		{Index: 2, Key: "url", Value: "https://example.com"},
	}
	if len(e.Lines) != len(want) {
		t.Fatalf("Lines = %+v, want %+v", e.Lines, want)
	}
	for i := range want {
		if e.Lines[i] != want[i] {
			t.Errorf("Lines[%d] = %+v, want %+v", i, e.Lines[i], want[i])
		}
	}
}

func TestComposeRoundTrips(t *testing.T) {
	original := "hunter2\nlogin: han\nurl: https://example.com\nfree text\n"
	got := ParseEntry("x", original).Compose()
	if got != original {
		t.Errorf("Compose() = %q, want %q", got, original)
	}
}

func TestComposeDropsEmptyFields(t *testing.T) {
	e := Entry{
		Password: "pw",
		Fields:   []Field{{"login", "han"}, {"url", "  "}},
	}
	want := "pw\nlogin: han\n"
	if got := e.Compose(); got != want {
		t.Errorf("Compose() = %q, want %q", got, want)
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"github", "web/github.com", "a/b/c", "with space"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{"", "   ", "-rf", "/etc/passwd", "../escape", "web/../../etc/x", "nul\x00byte", " -rf", "trailing ", "\tgithub"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want an error", name)
		}
	}
}
