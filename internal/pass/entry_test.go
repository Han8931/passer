package pass

import "testing"

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

	invalid := []string{"", "   ", "-rf", "/etc/passwd", "../escape", "web/../../etc/x", "nul\x00byte"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want an error", name)
		}
	}
}
