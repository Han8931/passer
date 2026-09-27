package pass

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestStore builds a Store over a temporary directory without requiring a
// real pass installation. Only the filesystem-facing methods (List, Exists) are
// exercised; anything that decrypts needs gpg and belongs in manual testing.
func newTestStore(t *testing.T, entries ...string) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte("test@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range entries {
		path := filepath.Join(dir, filepath.FromSlash(name)+".gpg")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("ciphertext"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &Store{bin: "pass", dir: dir}
}

func TestListFindsNestedEntries(t *testing.T) {
	s := newTestStore(t, "github", "web/example.com", "web/deep/nested")

	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"github", "web/deep/nested", "web/example.com"}
	if len(items) != len(want) {
		t.Fatalf("List() returned %d items: %+v", len(items), items)
	}
	for i, name := range want {
		if items[i].Name != name {
			t.Errorf("item %d = %q, want %q (results must be sorted)", i, items[i].Name, name)
		}
	}
}

func TestListSkipsPlumbing(t *testing.T) {
	s := newTestStore(t, "real")
	// A .git directory holding blobs that happen to end in .gpg must not be
	// mistaken for entries.
	gitDir := filepath.Join(s.Dir(), ".git", "objects")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "blob.gpg"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Non-.gpg files in the store are not entries either.
	if err := os.WriteFile(filepath.Join(s.Dir(), "README.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "real" {
		t.Errorf("List() = %+v, want just the real entry", items)
	}
}

func TestListRecordsModifiedTime(t *testing.T) {
	s := newTestStore(t, "github")
	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Modified.IsZero() {
		t.Error("Modified is zero; the list view has nothing to show in its Updated column")
	}
}

func TestExists(t *testing.T) {
	s := newTestStore(t, "web/example.com")

	if !s.Exists("web/example.com") {
		t.Error("Exists() = false for an entry that is on disk")
	}
	if s.Exists("web/missing") {
		t.Error("Exists() = true for an entry that is not on disk")
	}
	// A traversal attempt must not be able to probe outside the store, even
	// when the target exists.
	if s.Exists("../../etc/passwd") {
		t.Error("Exists() followed a path outside the store")
	}
}

func TestOpenRejectsUninitialisedStore(t *testing.T) {
	if _, err := exists(t); err != nil {
		t.Skip("pass is not installed")
	}
	dir := t.TempDir() // exists, but has no .gpg-id
	if _, err := Open(dir); err == nil {
		t.Error("Open() accepted a directory with no .gpg-id")
	}
}

// exists reports whether the pass binary is available, so tests that need it
// can skip rather than fail on machines without it.
func exists(t *testing.T) (bool, error) {
	t.Helper()
	if _, err := Open(DefaultDir()); err != nil && err.Error() == "`pass` was not found on PATH; install the standard unix password manager first" {
		return false, err
	}
	return true, nil
}

func TestMoveRefusesAnExistingFolder(t *testing.T) {
	// pass mv would turn "github -> web" into web/github, overwriting the
	// entry already there. The guard must fire before pass is ever run.
	s := newTestStore(t, "github", "web/github")
	s.bin = filepath.Join(t.TempDir(), "pass-must-not-run")

	for _, dest := range []string{"web", "web/"} {
		err := s.Move("github", dest)
		if err == nil || !strings.Contains(err.Error(), "is a folder") {
			t.Errorf("Move(github, %q) = %v, want a folder error", dest, err)
		}
	}
}
