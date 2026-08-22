package pass

import (
	"strings"
)

// Field is one "key: value" metadata line below an entry's password line.
type Field struct {
	Key   string
	Value string
}

// Entry is a decrypted password-store entry.
//
// pass only fixes one convention: the first line is the password and the rest
// is free-form. The widely used elaboration is "key: value" metadata lines, so
// we parse those into Fields — but anything that doesn't follow the convention
// is kept verbatim in Notes rather than dropped. A user's file should always be
// fully visible in the UI, even the parts passer doesn't understand.
//
// An Entry holds plaintext. passer never writes it to disk and releases it when
// the detail view closes; Go strings can't be zeroed, so in-memory lifetime is
// the only control we have.
type Entry struct {
	Name     string
	Password string
	Fields   []Field
	Notes    []string
}

// usernameKeys are the field names commonly used for the account identifier,
// in the order we prefer them.
var usernameKeys = []string{"login", "username", "user", "email", "account"}

// urlKeys are the field names commonly used for the site the entry belongs to.
var urlKeys = []string{"url", "uri", "site", "host", "website"}

// ParseEntry splits raw decrypted text into a password plus metadata.
//
// A trailing newline is normal in GPG-decrypted files and must not become an
// empty note line, so the raw text is right-trimmed before splitting.
func ParseEntry(name, raw string) Entry {
	e := Entry{Name: name}
	raw = strings.TrimRight(raw, "\r\n")
	if raw == "" {
		return e
	}

	lines := strings.Split(raw, "\n")
	e.Password = strings.TrimRight(lines[0], "\r")

	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if key, value, ok := splitField(line); ok {
			e.Fields = append(e.Fields, Field{Key: key, Value: value})
			continue
		}
		e.Notes = append(e.Notes, line)
	}
	return e
}

// splitField recognises a "key: value" metadata line.
//
// The hard case is a bare URL: "https://example.com" contains a colon and would
// otherwise parse as key "https". We accept a line as a field when the text
// before the first colon looks like a label — non-empty, no slash, not absurdly
// long — and the remainder doesn't open with "//", which is what separates a
// URL scheme from a real label. Keys containing spaces ("recovery code") are
// allowed only with the conventional ": " separator, so prose that happens to
// contain a colon stays a note.
func splitField(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	rest := line[idx+1:]
	if key == "" || len(key) > 40 || strings.Contains(key, "/") {
		return "", "", false
	}
	if strings.HasPrefix(rest, "//") {
		return "", "", false
	}
	if strings.ContainsAny(key, " \t") && !strings.HasPrefix(rest, " ") && rest != "" {
		return "", "", false
	}
	return key, strings.TrimSpace(rest), true
}

// Lookup returns the first field whose key matches any of keys, case-insensitively.
func (e Entry) Lookup(keys ...string) (string, bool) {
	for _, want := range keys {
		for _, f := range e.Fields {
			if strings.EqualFold(f.Key, want) && f.Value != "" {
				return f.Value, true
			}
		}
	}
	return "", false
}

// Username returns the entry's account identifier.
//
// extra lets the user's configured field names take priority over the built-in
// conventions, so a store that standardised on e.g. "id" still works.
func (e Entry) Username(extra []string) (string, bool) {
	return e.Lookup(append(append([]string{}, extra...), usernameKeys...)...)
}

// URL returns the entry's site address, if it carries one.
func (e Entry) URL() (string, bool) {
	return e.Lookup(urlKeys...)
}

// Compose renders the entry back into pass's on-disk format: password first,
// then metadata, then notes. Used when writing a new entry.
func (e Entry) Compose() string {
	var b strings.Builder
	b.WriteString(e.Password)
	b.WriteString("\n")
	for _, f := range e.Fields {
		if strings.TrimSpace(f.Value) == "" {
			continue
		}
		b.WriteString(f.Key)
		b.WriteString(": ")
		b.WriteString(f.Value)
		b.WriteString("\n")
	}
	for _, n := range e.Notes {
		b.WriteString(n)
		b.WriteString("\n")
	}
	return b.String()
}
