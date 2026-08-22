// Package clip copies short-lived secrets to the system clipboard.
//
// passer does its own clipboard handling instead of delegating to `pass -c`,
// for two reasons. `pass -c<n>` copies a whole line, so copying the value of a
// "login: han" field would put "login: han" on the clipboard. And pass forks a
// detached process to clear the clipboard later, which the TUI cannot observe —
// owning the timer ourselves is what lets the status bar show a live countdown.
package clip

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Backend is a detected clipboard tool.
type Backend struct {
	Name  string
	copy  []string
	paste []string
}

// Available reports whether a usable clipboard tool was found.
func (b Backend) Available() bool { return len(b.copy) > 0 }

// candidates are probed in order. Wayland is listed first but only wins when
// the session is actually Wayland: many Wayland desktops also run XWayland, so
// both wl-copy and xclip resolve and copying with the wrong one silently
// targets a clipboard nothing is reading.
var candidates = []Backend{
	{Name: "wl-copy", copy: []string{"wl-copy"}, paste: []string{"wl-paste", "--no-newline"}},
	{Name: "pbcopy", copy: []string{"pbcopy"}, paste: []string{"pbpaste"}},
	{Name: "xclip", copy: []string{"xclip", "-selection", "clipboard"}, paste: []string{"xclip", "-selection", "clipboard", "-o"}},
	{Name: "xsel", copy: []string{"xsel", "--clipboard", "--input"}, paste: []string{"xsel", "--clipboard", "--output"}},
	{Name: "termux-clipboard-set", copy: []string{"termux-clipboard-set"}, paste: []string{"termux-clipboard-get"}},
}

// Detect picks the first clipboard tool present on this system.
func Detect() (Backend, error) {
	wayland := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != ""
	for _, c := range candidates {
		if c.Name == "wl-copy" && !wayland {
			continue
		}
		if _, err := exec.LookPath(c.copy[0]); err != nil {
			continue
		}
		return c, nil
	}
	return Backend{}, errors.New("no clipboard tool found; install wl-clipboard, xclip, or xsel")
}

// Copy places text on the clipboard.
//
// The secret is written to the tool's stdin, never passed as an argument, so it
// never appears in the process list.
//
// Stderr goes to a temporary file rather than a bytes.Buffer, which looks like
// the long way round but is load-bearing. X11 has no clipboard daemon: xclip
// forks a background process that survives to serve the selection. Assigning
// any non-*os.File to cmd.Stderr makes os/exec create an OS pipe and copy from
// it in a goroutine, and Run then waits for that pipe to reach EOF — but the
// forked xclip inherits the write end and holds it open for as long as it owns
// the selection. Run would block until the clipboard was overwritten, so every
// copy hung. Handing over a real file passes the descriptor straight to the
// child and leaves Wait with nothing to drain.
func (b Backend) Copy(text string) error {
	if !b.Available() {
		return errors.New("no clipboard backend")
	}

	cmd := exec.Command(b.copy[0], b.copy[1:]...)
	cmd.Stdin = strings.NewReader(text)

	// The file only ever receives the tool's diagnostics; the secret goes to
	// stdin, so nothing sensitive is written to disk here.
	if errFile, err := os.CreateTemp("", "passer-clip-*.err"); err == nil {
		defer os.Remove(errFile.Name())
		defer errFile.Close()
		cmd.Stderr = errFile

		if runErr := cmd.Run(); runErr != nil {
			if msg := readStderr(errFile); msg != "" {
				return errors.New(msg)
			}
			return runErr
		}
		return nil
	}

	// Without a temp file we still must not hand os/exec a pipe, so we give up
	// the tool's message and report the exit status alone.
	return cmd.Run()
}

// readStderr rewinds the captured stderr file and returns its first line.
func readStderr(f *os.File) string {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	msg := strings.TrimSpace(string(data))
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		msg = strings.TrimSpace(msg[:idx])
	}
	return msg
}

// Read returns the current clipboard contents.
//
// Paste tools print and exit, so capturing stdout in a buffer is safe here —
// unlike the copy path. The timeout is belt and braces: this runs on the
// auto-clear path, and a wedged tool must not be able to strand a secret on the
// clipboard.
func (b Backend) Read() (string, error) {
	if len(b.paste) == 0 {
		return "", errors.New("no clipboard paste backend")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, b.paste[0], b.paste[1:]...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// ClearIfUnchanged wipes the clipboard, but only while it still holds expect.
//
// Without the check, an auto-clear firing 45 seconds after a copy would destroy
// whatever the user copied in the meantime. If the clipboard can't be read back
// we clear anyway: leaving a password sitting there is the worse failure.
func (b Backend) ClearIfUnchanged(expect string) error {
	if !b.Available() {
		return nil
	}
	if current, err := b.Read(); err == nil {
		if strings.TrimRight(current, "\r\n") != strings.TrimRight(expect, "\r\n") {
			return nil
		}
	}
	return b.Copy("")
}
