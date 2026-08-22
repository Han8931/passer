package pass

import (
	"strings"
	"testing"
)

func TestNewPasswordLength(t *testing.T) {
	for _, n := range []int{1, 8, 24, 64} {
		pw, err := NewPassword(n, true)
		if err != nil {
			t.Fatalf("NewPassword(%d) error: %v", n, err)
		}
		if len(pw) != n {
			t.Errorf("NewPassword(%d) produced %d characters", n, len(pw))
		}
	}
}

func TestNewPasswordNoSymbols(t *testing.T) {
	// Sample enough characters that an accidental symbol would show up.
	pw, err := NewPassword(256, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(pw, punct) {
		t.Errorf("NewPassword(symbols=false) produced punctuation: %q", pw)
	}
}

func TestNewPasswordUsesSymbols(t *testing.T) {
	// With 256 characters drawn from a 94-character alphabet, the chance of no
	// punctuation appearing is vanishingly small, so this is not flaky.
	pw, err := NewPassword(256, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsAny(pw, punct) {
		t.Errorf("NewPassword(symbols=true) produced no punctuation: %q", pw)
	}
}

func TestNewPasswordRejectsBadLengths(t *testing.T) {
	for _, n := range []int{0, -1, MaxPasswordLength + 1} {
		if _, err := NewPassword(n, true); err == nil {
			t.Errorf("NewPassword(%d) = nil error, want a rejection", n)
		}
	}
}

func TestNewPasswordDiffers(t *testing.T) {
	a, _ := NewPassword(32, true)
	b, _ := NewPassword(32, true)
	if a == b {
		t.Error("two generated passwords are identical; the source is not random")
	}
}
