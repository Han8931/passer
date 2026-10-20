package pass

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrNoStore reports a password store that has not been initialised. It is
// surfaced verbatim so the UI can suggest `pass init` rather than showing a
// bare stat error.
var ErrNoStore = errors.New("password store not initialised")

// Store drives the `pass` command line tool.
//
// passer deliberately shells out instead of talking to GPG directly: pass owns
// the .gpg-id resolution, per-directory keys, git integration, and — most
// importantly — the pinentry/gpg-agent handshake. Reimplementing that would
// mean reimplementing the security decisions of the tool we're a front end for.
type Store struct {
	bin string // resolved path to the pass executable
	dir string // password store root
}

// Item is one entry as seen on disk, before any decryption.
//
// Modified comes from the file's mtime, which lets the list view show a useful
// column without decrypting anything — moving the cursor must never trigger a
// GPG prompt.
type Item struct {
	Name     string // store-relative name, e.g. "web/github.com"
	Modified time.Time
}

// Open locates the pass binary and the store directory.
func Open(dir string) (*Store, error) {
	bin, err := exec.LookPath("pass")
	if err != nil {
		return nil, errors.New("`pass` was not found on PATH; install the standard unix password manager first")
	}

	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = DefaultDir()
	}
	dir = expandHome(dir)

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s does not exist; run `pass init <gpg-id>` to create it", ErrNoStore, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gpg-id")); err != nil {
		return nil, fmt.Errorf("%w: %s has no .gpg-id; run `pass init <gpg-id>`", ErrNoStore, dir)
	}

	return &Store{bin: bin, dir: dir}, nil
}

// DefaultDir mirrors pass's own default: PASSWORD_STORE_DIR, else
// ~/.password-store.
func DefaultDir() string {
	if dir := strings.TrimSpace(os.Getenv("PASSWORD_STORE_DIR")); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".password-store")
	}
	return ".password-store"
}

// Dir is the resolved password store root.
func (s *Store) Dir() string { return s.dir }

// List walks the store for entries, sorted by name.
//
// We read the filesystem rather than parsing `pass ls`, whose tree output is
// built for human eyes: it wraps, colours, and gives no way to distinguish a
// directory named "a.b" from a nested entry. Walking also gives us mtimes for
// free.
func (s *Store) List() ([]Item, error) {
	var items []Item
	err := filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree shouldn't abort the whole listing; skip it
			// and keep whatever else the store holds.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			// .git, .gpg-keys and friends are store plumbing, not entries.
			if path != s.dir && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".gpg") {
			return nil
		}
		rel, relErr := filepath.Rel(s.dir, path)
		if relErr != nil {
			return nil
		}
		item := Item{Name: strings.TrimSuffix(filepath.ToSlash(rel), ".gpg")}
		if info, statErr := d.Info(); statErr == nil {
			item.Modified = info.ModTime()
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// Exists reports whether an entry is already in the store. Callers use it to
// refuse an overwrite before running a command with --force.
func (s *Store) Exists(name string) bool {
	if err := validateName(name); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(s.dir, filepath.FromSlash(name)+".gpg"))
	return err == nil
}

// Show decrypts a single entry. This is the only operation that can block on a
// pinentry prompt, so the UI always runs it as a background command.
func (s *Store) Show(name string) (Entry, error) {
	if err := validateName(name); err != nil {
		return Entry{}, err
	}
	out, err := s.run(nil, "show", "--", name)
	if err != nil {
		return Entry{}, err
	}
	return ParseEntry(name, out), nil
}

// Insert writes content as a new entry, or replaces an existing one.
//
// --multiline makes pass read the whole body from stdin, which keeps the
// plaintext off disk and out of the process table — unlike the single-line form
// or any approach that passes the password as an argument.
func (s *Store) Insert(name, content string, overwrite bool) error {
	if err := validateName(name); err != nil {
		return err
	}
	if !overwrite && s.Exists(name) {
		return fmt.Errorf("%s already exists", name)
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	_, err := s.run(strings.NewReader(content), "insert", "--multiline", "--force", "--", name)
	return err
}

// Remove deletes an entry.
func (s *Store) Remove(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	_, err := s.run(nil, "rm", "--force", "--", name)
	return err
}

// Move renames an entry, re-encrypting it if the destination sits under a
// different .gpg-id.
func (s *Store) Move(oldName, newName string) error {
	if err := validateName(oldName); err != nil {
		return err
	}
	if err := validateName(newName); err != nil {
		return err
	}
	if s.Exists(newName) {
		return fmt.Errorf("%s already exists", newName)
	}
	// `pass mv` treats an existing directory as "move into it", so renaming
	// github to web would really produce web/github — and with --force, replace
	// a web/github that was already there. Refuse instead of guessing.
	if info, err := os.Stat(filepath.Join(s.dir, filepath.FromSlash(newName))); err == nil && info.IsDir() {
		return fmt.Errorf("%s is a folder; give the full new name, e.g. %s/%s", newName, strings.TrimSuffix(newName, "/"), path.Base(oldName))
	}
	_, err := s.run(nil, "mv", "--force", "--", oldName, newName)
	return err
}

// ErrUnchanged reports a `pass edit` that ended without changes. pass exits
// non-zero for it, but for the user it is an outcome, not a failure.
var ErrUnchanged = errors.New("no changes")

// EditCmd builds the `pass edit` invocation for an entry.
//
// It is returned rather than run because `pass edit` hands the terminal to
// $EDITOR: the caller must suspend the TUI around it, the same way bada
// suspends around `:config`.
//
// finish must be called with the command's exit error once it has run. It
// turns the exit into pass's own message: pass writes it to stderr, which
// would otherwise land on the alternate screen the TUI is about to repaint,
// leaving nothing but "exit status 1". stderr goes to a temporary file, not a
// pipe, for the reason given in clip.Copy: gpg may leave gpg-agent running with
// the write end open, and waiting for a pipe to drain would hang the edit.
func (s *Store) EditCmd(name string) (cmd *exec.Cmd, finish func(error) error, err error) {
	if err := validateName(name); err != nil {
		return nil, nil, err
	}
	// No "--" here, unlike every other call site: cmd_edit takes exactly one
	// argument and doesn't parse options, so it would read "--" as the name and
	// die with a usage error. validateName's leading-dash rule is what keeps the
	// name from being taken as a flag.
	cmd = exec.Command(s.bin, "edit", name)
	cmd.Env = s.env()

	errFile, tmpErr := os.CreateTemp("", "passer-edit-*.err")
	if tmpErr != nil {
		// Without a file, pass's message goes to the terminal as it always did.
		return cmd, func(runErr error) error { return runErr }, nil
	}
	cmd.Stderr = errFile
	finish = func(runErr error) error {
		defer os.Remove(errFile.Name())
		defer errFile.Close()
		if runErr == nil {
			return nil
		}
		return editError(runErr, readAll(errFile))
	}
	return cmd, finish, nil
}

// editError picks the message that explains a failed `pass edit`. See lastLine
// for why it is the last line.
func editError(runErr error, stderr string) error {
	msg := lastLine(stderr)
	switch {
	case msg == "Password unchanged.":
		return ErrUnchanged
	case msg != "":
		return errors.New(msg)
	}
	return runErr
}

func readAll(f *os.File) string {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(data)
}

// lastLine returns the line of pass's stderr that explains a failure. pass and
// gpg die with their reason after whatever warnings came first — macOS's
// hdiutil deprecation notice, gpg's "unsafe permissions on homedir" — so the
// first line is usually noise and the last is the cause.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// pipeDrainDelay bounds how long run waits for pass's output pipes to close
// after pass itself has exited.
const pipeDrainDelay = 2 * time.Second

// run executes pass and returns stdout, folding stderr into the error so the
// UI can show pass's own diagnostics instead of "exit status 1".
//
// Output is captured through pipes, which carries the hazard described at
// EditCmd: a gpg-agent that gpg spawned can inherit the write ends and hold
// them open long after pass exits. EditCmd sidesteps it with a temporary file,
// but stdout here carries decrypted plaintext, which must never touch disk. So
// we keep the pipes and set WaitDelay instead: once pass has exited, Wait
// collects what is already buffered — pass writes everything before exiting —
// and then closes the pipes rather than waiting for EOF.
func (s *Store) run(stdin *strings.Reader, args ...string) (string, error) {
	cmd := exec.Command(s.bin, args...)
	cmd.Env = s.env()
	cmd.WaitDelay = pipeDrainDelay
	if stdin != nil {
		cmd.Stdin = stdin
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// ErrWaitDelay means pass exited cleanly and only a straggler was holding
	// a pipe; the output is complete.
	if err := cmd.Run(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		if msg := lastLine(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

// env pins PASSWORD_STORE_DIR to the store we resolved, so a configured
// store_dir wins over whatever the surrounding shell exported.
func (s *Store) env() []string {
	return append(os.Environ(), "PASSWORD_STORE_DIR="+s.dir)
}

// validateName rejects entry names that would escape the store or confuse the
// shell-facing tool. pass takes names as paths relative to the store root, so
// "..", absolute paths, and leading dashes are the cases that matter — the
// latter because a name beginning with "-" would otherwise be read as a flag
// (we also pass "--" as a second line of defence wherever pass accepts it,
// which is everywhere except `pass edit`).
//
// Surrounding whitespace is rejected rather than trimmed: callers pass the name
// on to pass untouched, so the string we check must be the string we use.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("entry name is empty")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("entry name may not start or end with whitespace")
	}
	if strings.HasPrefix(name, "-") {
		return errors.New("entry name may not start with '-'")
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return errors.New("entry name must be relative to the store")
	}
	if strings.ContainsRune(name, 0) {
		return errors.New("entry name contains a null byte")
	}
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".." {
			return errors.New("entry name may not contain '..'")
		}
	}
	return nil
}

// ValidateName exposes the name rules to the UI so a bad name is reported in
// the add/rename dialog rather than after a failed command.
func ValidateName(name string) error { return validateName(name) }

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	return path
}
