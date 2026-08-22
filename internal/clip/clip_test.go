package clip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBackend stands in for a real clipboard tool with a file. It exercises the
// genuine exec paths — including the stderr handling that a daemonising tool
// like xclip would otherwise deadlock on.
func fakeBackend(t *testing.T) (Backend, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clipboard")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return Backend{
		Name:  "fake",
		copy:  []string{"sh", "-c", "cat > " + path},
		paste: []string{"cat", path},
	}, path
}

func TestCopyReadRoundTrip(t *testing.T) {
	b, _ := fakeBackend(t)

	if err := b.Copy("hunter2"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	got, err := b.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != "hunter2" {
		t.Errorf("Read = %q, want hunter2", got)
	}
}

func TestCopyReportsToolFailure(t *testing.T) {
	b := Backend{
		Name: "broken",
		copy: []string{"sh", "-c", "echo 'cannot open display' >&2; exit 1"},
	}
	err := b.Copy("secret")
	if err == nil {
		t.Fatal("Copy of a failing tool returned nil")
	}
	if !strings.Contains(err.Error(), "cannot open display") {
		t.Errorf("error = %q, want the tool's own diagnostic", err)
	}
}

func TestClearIfUnchangedWipesOurOwnSecret(t *testing.T) {
	b, path := fakeBackend(t)

	if err := b.Copy("hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := b.ClearIfUnchanged("hunter2"); err != nil {
		t.Fatalf("ClearIfUnchanged: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Errorf("clipboard still holds %q after auto-clear", data)
	}
}

func TestClearIfUnchangedLeavesLaterCopiesAlone(t *testing.T) {
	// The whole point of the check: an auto-clear firing 45 seconds after a
	// copy must not destroy something the user copied in the meantime.
	b, path := fakeBackend(t)

	if err := b.Copy("something the user copied later"); err != nil {
		t.Fatal(err)
	}
	if err := b.ClearIfUnchanged("the old password"); err != nil {
		t.Fatalf("ClearIfUnchanged: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "something the user copied later" {
		t.Errorf("clipboard = %q; a later copy was wiped", data)
	}
}

func TestClearIfUnchangedIgnoresTrailingNewline(t *testing.T) {
	// Paste tools differ on whether they return the trailing newline, and a
	// mismatch there would silently disable auto-clear.
	b, path := fakeBackend(t)

	if err := b.Copy("hunter2\n"); err != nil {
		t.Fatal(err)
	}
	if err := b.ClearIfUnchanged("hunter2"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if len(data) != 0 {
		t.Errorf("clipboard still holds %q; the newline defeated the comparison", data)
	}
}

func TestUnavailableBackend(t *testing.T) {
	var b Backend
	if b.Available() {
		t.Error("a zero Backend should not report itself as available")
	}
	if err := b.Copy("x"); err == nil {
		t.Error("Copy on an unavailable backend should error")
	}
	// Clearing is a no-op rather than an error: the auto-clear path fires
	// unconditionally and must not report a failure that never mattered.
	if err := b.ClearIfUnchanged("x"); err != nil {
		t.Errorf("ClearIfUnchanged on an unavailable backend = %v, want nil", err)
	}
}

func TestDetectSkipsWaylandToolOutsideWayland(t *testing.T) {
	// wl-copy may exist on an X11 session via a Wayland-capable install;
	// copying with it there silently targets a clipboard nothing reads.
	t.Setenv("WAYLAND_DISPLAY", "")

	b, err := Detect()
	if err != nil {
		t.Skip("no clipboard tool installed")
	}
	if b.Name == "wl-copy" {
		t.Error("Detect chose wl-copy with no Wayland session")
	}
}
