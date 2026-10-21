package pass

import (
	"fmt"
	"strings"
)

// Field is one "key: value" metadata line below an entry's password line.
type Field struct {
	Key   string
	Value string
}

// Line is one line of an entry's body — everything below the password.
//
// Index is the line's position in the raw text, which is what lets an in-place
// edit target the user's actual line instead of a re-composed approximation.
// Key is empty for a line that is not "key: value", and Value then holds the
// whole line.
type Line struct {
	Index int
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
	// Lines is the body in file order, with each line's position retained.
	// Fields and Notes are views over the same content, grouped by kind; Lines
	// is what an edit addresses.
	Lines []Line
	// Raw is the decrypted text the entry was parsed from, newline-trimmed. It
	// is kept so an in-place edit can rewrite the user's own lines instead of
	// re-composing them from Fields and Notes, which would reorder the file.
	// Like every other field here it is plaintext, and shares its lifetime.
	Raw string
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
	e.Raw = raw
	if raw == "" {
		return e
	}

	lines := strings.Split(raw, "\n")
	e.Password = strings.TrimRight(lines[0], "\r")

	for i, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		index := i + 1
		if key, value, ok := splitField(line); ok {
			e.Fields = append(e.Fields, Field{Key: key, Value: value})
			e.Lines = append(e.Lines, Line{Index: index, Key: key, Value: value})
			continue
		}
		e.Notes = append(e.Notes, line)
		e.Lines = append(e.Lines, Line{Index: index, Value: line})
	}
	return e
}

// SetValueAt returns the entry's file content with the value on raw line index
// replaced, ready to hand to Insert.
//
// Line 0 is the password. On a "key: value" line only the value changes — the
// key is the user's and stays exactly as they spelled it. Any other line is
// replaced whole, since a note has no structure to preserve. Every untouched
// line, and the order of all of them, survives verbatim; see SetUsername for
// why that matters.
func (e Entry) SetValueAt(index int, value string) (string, error) {
	raw := strings.TrimRight(e.Raw, "\r\n")
	if raw == "" {
		raw = strings.TrimRight(e.Compose(), "\n")
	}
	lines := strings.Split(raw, "\n")
	if index < 0 || index >= len(lines) {
		return "", fmt.Errorf("line %d is outside %s", index, e.Name)
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}

	switch key, _, ok := splitField(lines[index]); {
	case index == 0:
		lines[0] = value
	case ok:
		lines[index] = key + ": " + value
	default:
		lines[index] = value
	}
	return strings.Join(lines, "\n") + "\n", nil
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

// UsernameSource says where an account identifier was found. Copying something
// passer inferred rather than read from a labelled field is a surprise worth
// reporting, so the UI tells the user which it was.
type UsernameSource int

const (
	UsernameNone  UsernameSource = iota
	UsernameField                // a "key: value" metadata line
	UsernameNote                 // a bare line holding nothing but an address
	UsernameName                 // the last segment of the entry's own name
)

// UsernameKeys is the full list of field names Username will accept, in the
// order it tries them. The UI reports it when no account identifier is found,
// so the message names the fields the user could add.
func UsernameKeys(extra []string) []string {
	keys := append([]string{}, extra...)
	for _, k := range usernameKeys {
		if !containsFold(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}

// Username returns the entry's account identifier.
//
// extra lets the user's configured field names take priority over the built-in
// conventions, so a store that standardised on e.g. "id" still works.
//
// Only a minority of real stores label the account with a "key: value" line, so
// a field miss falls back to the two other conventions in the wild: the address
// on a bare line of its own, and the "site/user@host" entry name from pass's own
// docs. Both fallbacks demand something that looks like an email address —
// see looksLikeAddress — because that is what separates an account from the
// free-form prose and bare site names those same positions usually hold.
func (e Entry) Username(extra []string) (string, bool) {
	value, _, ok := e.UsernameFrom(extra)
	return value, ok
}

// UsernameFrom is Username, also reporting which convention matched.
func (e Entry) UsernameFrom(extra []string) (string, UsernameSource, bool) {
	if value, ok := e.Lookup(UsernameKeys(extra)...); ok {
		return value, UsernameField, true
	}
	for _, note := range e.Notes {
		if candidate := strings.TrimSpace(note); looksLikeAddress(candidate) {
			return candidate, UsernameNote, true
		}
	}
	// The name's last segment: "web/github.com/han@example.com" is the layout
	// pass's own documentation suggests for a site with several accounts.
	leaf := e.Name
	if idx := strings.LastIndex(leaf, "/"); idx >= 0 {
		leaf = leaf[idx+1:]
	}
	if looksLikeAddress(leaf) {
		return leaf, UsernameName, true
	}
	return "", UsernameNone, false
}

// looksLikeAddress reports whether s is, on its own, an email-style account
// identifier.
//
// The test is deliberately narrow, because both callers look at positions that
// normally hold something else entirely. Requiring a single "@" with text on
// either side rejects a bare site name ("github.com"), and rejecting
// whitespace rejects prose that merely mentions an address. The punctuation
// bans are what keep a URI out: pass-otp stores its seed as a bare
// "otpauth://totp/GitHub:han@example.com?secret=…" note, which has an "@" and
// no spaces, and copying a TOTP secret as the username would be a real leak.
func looksLikeAddress(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t:/?\\,;\"'<>") {
		return false
	}
	at := strings.Index(s, "@")
	if at <= 0 || at == len(s)-1 {
		return false
	}
	domain := s[at+1:]
	if strings.Contains(domain, "@") {
		return false
	}
	return !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// URL returns the entry's site address, if it carries one.
func (e Entry) URL() (string, bool) {
	return e.Lookup(urlKeys...)
}

// SetUsername returns the entry's file content with its account identifier set
// to value, ready to hand to Insert.
//
// It edits the raw decrypted lines rather than re-composing from Fields and
// Notes. Compose groups every field above every note, so composing a parsed
// entry would silently reorder a hand-written file — a password store is the
// user's own data, and changing one field must leave every other line, and
// their order, exactly as they wrote them.
//
// An existing account field keeps its own key, so a store that writes
// "username:" is not quietly converted to "login:". Only when there is no such
// field is a new line added, using the first of keys — the user's configured
// preference, falling back to pass's common "login".
func (e Entry) SetUsername(value string, keys []string) string {
	value = strings.TrimSpace(value)

	raw := strings.TrimRight(e.Raw, "\r\n")
	if raw == "" {
		// An entry built in memory rather than parsed has no raw text; there is
		// no user formatting to preserve, so composing is exactly right.
		raw = strings.TrimRight(e.Compose(), "\n")
	}
	lines := strings.Split(raw, "\n")

	out := make([]string, 0, len(lines)+1)
	out = append(out, strings.TrimRight(lines[0], "\r")) // the password line is never touched

	replaced := false
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if key, _, ok := splitField(line); ok && !replaced && containsFold(keys, key) {
			out = append(out, key+": "+value)
			replaced = true
			continue
		}
		// A bare line holding exactly this address was the account all along.
		// It is about to be carried by a labelled field, so keeping it would
		// leave the same value in the file twice. Nothing is lost: the value
		// survives, now labelled.
		if strings.TrimSpace(line) == value && looksLikeAddress(value) {
			continue
		}
		out = append(out, line)
	}

	if !replaced {
		// Directly under the password, matching the order the New Entry dialog
		// writes and keeping the account where the eye looks for it.
		withField := make([]string, 0, len(out)+1)
		withField = append(withField, out[0], accountKey(keys)+": "+value)
		out = append(withField, out[1:]...)
	}
	return strings.Join(out, "\n") + "\n"
}

// accountKey is the field name to write a brand-new account line under.
func accountKey(keys []string) string {
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			return k
		}
	}
	return "login"
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
