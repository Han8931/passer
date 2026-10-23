# Passer User Guide

This guide covers installation, everyday keyboard workflows, the views, adding
and generating entries, the security model, themes, and maintenance. For a short
project overview, see the [README](README.md).

## Contents

- [Quick Start](#quick-start)
- [Views](#views)
- [Daily Use](#daily-use)
- [Adding entries](#adding-entries)
- [Editing, renaming, deleting](#editing-renaming-deleting)
- [Commands](#commands)
- [Security model](#security-model)
- [Theme](#theme)
- [Configuration](#configuration)
- [Install (Linux)](#install-linux)
- [Uninstall (Linux)](#uninstall-linux)
- [Testing](#testing)

## Quick Start

Passer needs Go 1.25 or newer to build, and `pass` at runtime. From a source
checkout:

```bash
./install.sh
passer
```

To run it without installing:

```bash
go build -o bin/passer ./cmd/passer
./bin/passer
```

Passer refuses to start if `pass` is missing from `$PATH`, or if the store has
no `.gpg-id`. Both cases print what to do — usually `pass init <your-gpg-id>`.

### Where the store comes from

Passer resolves the store the same way `pass` does: `$PASSWORD_STORE_DIR` if it
is set, `~/.password-store` otherwise. Setting `store_dir` in the config pins it
to one location regardless of the surrounding shell, which is useful if you run
passer from a launcher rather than a terminal.

## Views

### List

The opening view: every entry in the store, sorted by path, in three aligned
columns: `Folder`, `Entry`, and `Updated` (the file's modification time). Each
folder is named once at the top of its run of entries, so the store reads like
a tree while every entry name still starts in the same column. Entries at the
top of the store show `–` in the folder column. Search results are ranked
rather than grouped, so there every row names its folder.

The list is deliberately flat rather than an expandable tree. A tree needs you
to know which folder an entry is in before you can find it; a flat list plus
fuzzy search does not.

The status bar shows the mode (`LIST`, `SEARCH`, `ENTRY`, …), your position in
the list, and any active filter. Copy and reveal countdowns are pinned to its
right edge.

**Nothing in this view is decrypted.** Moving the cursor reads no GPG files and
triggers no pinentry prompt — the entry names and mtimes all come from the
filesystem. Decryption happens only when you open or copy from a specific entry.

### Detail

`Enter` decrypts one entry and opens it in a panel under the list, which stays
on screen so you keep your place. The entry shows as numbered rows: the
password first, then each `key: value` metadata line, then any free-form
notes.

Secrets are masked. The password is always masked, and so is any field whose
name looks like a second secret — `otp`, `totp`, `token`, `pin`, `recovery`,
`seed`, `secret`, `key`, as well as a pass-otp `otpauth://` line. Everything else (`login`, `url`, and friends) is shown
in clear, because that is what makes the view worth opening.

`space` reveals the masked values; the panel title gains a `• REVEALED` marker
and the status bar counts down to when they will be hidden again.

Passer parses metadata leniently. A line only becomes a `key: value` field if
the part before the colon actually looks like a label, so a bare
`https://example.com` stays a note instead of turning into a field named
`https`. Anything passer doesn't recognise is still displayed — nothing in your
file is hidden from you.

### Help

`?` opens the complete keybinding reference, rendered from your *current*
bindings rather than the defaults. `j`/`k` scroll, `q` returns.

## Daily Use

### Moving around

- `j` / `k` — down / up (also `↓`/`↑`, `ctrl+n`/`ctrl+p`)
- `gg` / `G` — jump to top / bottom
- `ctrl+d` / `ctrl+u` — half page down / up
- `Enter` — open the entry under the cursor
- `q` — back out of a filter or view; from the plain unfiltered list, quit

`q` never quits while a filter is active. An active search looks enough like the
whole store that quitting from it would be a surprise, so the first `q` clears
the filter and the second one exits.

### Searching

`/` opens the fuzzy filter and narrows the list as you type. Matching is a
case-insensitive subsequence, ranked so that consecutive runs and matches at a
path boundary win — `ghub` puts `web/github.com` above `misc/graph-hub-notes`.
Matched characters are highlighted in place.

Two ways out of the prompt:

- `Enter` — open the entry under the cursor. This is the fast path:
  `/gith` `Enter` goes from anywhere to an open entry.
- `Esc` — leave the prompt but **keep** the filter, so the list keys (`y`, `a`,
  `D`, …) act on the narrowed set. A second `Esc` clears the filter.

The title bar shows `Passer · 3/47` while a filter is active.

### Copying

Copy keys work from both the list and the detail view. From the list they
decrypt the entry, take the one field, and discard the rest — the secret is
never displayed.

- `y` — the password
- `u` — the username or email address (see below)
- `b` — the URL: the first `url`, `uri`, `site`, `host`, or `website` field

#### Where `u` looks for your account

Most stores don't label the account with a `key: value` line, so `u` tries three
conventions in order and stops at the first that matches:

| | Looks like | Copied as |
|---|---|---|
| 1. A labelled field | `login: han@example.com` | `username` |
| 2. A bare address line | `han@example.com` on its own line | `username (from note)` |
| 3. The entry name | `web/github.com/han@example.com` | `username (from name)` |

A field always wins, since it is the only one you stated outright. The status bar
names the source so a value Passer *inferred* is never mistaken for one you
labelled yourself.

Both fallbacks require something that looks like an email address — one `@`, no
spaces, no `:` or `/`. That is what separates an account from the free-form prose
and bare site names those same positions usually hold, and it is what keeps a
`pass-otp` seed (`otpauth://totp/GitHub:han@example.com?secret=…`, which has an
`@` and no spaces) from being copied as your username.

So a non-email account in the entry name — `web/github.com/han` — is *not*
picked up: it can't be told apart from a site name like `web/github.com`. Add a
`login:` line to those entries, or set `username_fields` if your store uses a
key Passer doesn't know:

```toml
username_fields = ['acct', 'login', 'username', 'user', 'email']
```

When nothing matches, `u` names the fields that would work rather than just
reporting the absence.

In the detail view you can also copy any field by position:

- `1`–`9` — copy the nth row
- `c` (or `Enter`) — copy the row under the cursor

Copying a field that doesn't exist reports it rather than silently doing
nothing.

Passer copies the field's *value*, not the line it came from. This is the reason
it handles the clipboard itself rather than delegating to `pass -c`, which
copies whole lines and would put `login: han` on your clipboard.

## Adding entries

`a` opens the **New Entry** dialog, centered over the list. It starts with
**Name**, **Password**, **Username**, and **URL**, plus a `▸ More details` row
that reveals **Notes**.

- Move between fields with `Tab`/`Shift+Tab` or `↑`/`↓`; type straight into
  them.
- `Ctrl+S` saves from anywhere. `Enter` also saves, except on **More details**,
  where it expands the section.
- `Esc` cancels and discards.
- `Ctrl+G` generates a password into the field and reveals it, so you can check
  it against a site's character rules before committing.
- `Ctrl+R` toggles the password field between masked and clear.

**Leaving the password blank generates one** at your configured length. The
footer states the length so this is never a surprise.

If the cursor was on `web/github.com` when you pressed `a`, the Name field is
prefilled with `web/` — adding a second entry to a folder shouldn't mean
retyping the prefix.

Passer refuses to overwrite an existing entry from this dialog. Silently
replacing an entry would destroy a password with no way back, so the name is
validated against the store before anything is written.

Everything is written in a single `pass insert --multiline`, which means the
plaintext goes over stdin and the store's git repository (if it has one) gets
one commit per entry.

### Generating without the dialog

`:generate <name> [length]` adds an entry that is nothing but a password —
the common case for a throwaway account.

### How generation works

Passer generates in-process with `crypto/rand`, rather than shelling out to
`pass generate`, because the dialog needs the value in hand to display it and to
write it alongside the other fields in one operation. The character set is the
same one `pass` uses: `[:alnum:]`, plus `[:punct:]` unless you set
`generate.symbols = false`.

## Editing, renaming, deleting

### Editing one value — `e`

With an entry open, `e` edits the value of the **row under the cursor**, in a
one-line prompt under the panel. It works on every row: the password, any
`key: value` field, and any free-form note.

```
ENTRY  web/github
  1  password   ••••••••
▸ 2  login      han          ← e edits this
  3  url        https://github.com

 login → han▌   (enter saves, esc cancels)
```

- `Enter` saves; `Esc` cancels. An **empty prompt cancels** rather than clearing
  the value — losing a password should never be one keystroke away. Removing a
  line is what `E` is for.
- On a field, only the value changes. The key keeps your spelling: a
  `username:` line is never silently rewritten to `login:`.
- On the password row, `ctrl+g` generates a fresh password and shows it; `ctrl+r`
  toggles reveal. Masked rows (otp, recovery codes, anything matching the secret
  field names) stay masked while you type, with `ctrl+r` to reveal.
- Saving rewrites **your** lines. Every other line, and the order of all of
  them, survives byte for byte — passer edits the decrypted text rather than
  re-composing the file from what it parsed.

Detail rows are listed in **file order**, so the row you are on is the line you
are editing.

### Registering an account — `U`

`U` sets the account identifier on an entry, from the list or from an open
entry, without going through `$EDITOR`. It is pre-filled with whatever `u` would
copy, so an account passer only *inferred* — from a bare address line or from
the entry's name — becomes a real `login:` field by pressing `U` then `Enter`.

Registering an address that was already sitting on its own line **absorbs** that
line rather than duplicating the value. If no account field exists, the new line
goes directly under the password, using the first of your `username_fields`
(`login` by default).

### The whole file — `E`

- `E` — opens the entry in `$EDITOR` via `pass edit`. Passer suspends itself,
  hands over the terminal, and on return re-reads the store and re-decrypts the
  entry if it is still on screen. This is the way to add or remove lines, which
  the one-line editor deliberately won't do. From the list, where no row is
  focused, `e` does this too.
- `m` — rename / move. `pass mv` re-encrypts the entry if the destination sits
  under a different `.gpg-id`. Refuses to clobber an existing name, and
  refuses a destination that is a folder — give the full path instead
  (`web/github`, not `web`).
- `D` — delete, with a confirmation line. `y` confirms; any other key cancels.

After a rename or an add, the cursor follows the entry you just touched rather
than jumping back to the top.

## Commands

`:` opens the command line. `↑`/`↓` walk previously run commands, and `Esc`
cancels.

`Tab` completes the command name. When only one command matches what you have
typed it is filled in whole, ready for an argument; otherwise `Tab` fills in as
much as every candidate shares and then steps through them one press at a time,
with `Shift+Tab` going back. The candidates are listed beside the prompt while
you cycle, and the one in the input is highlighted. Pressing `Tab` on an empty
command line walks the whole set, so you can browse the commands without
remembering any of them.

- `:add [name]` — open the New Entry dialog, optionally prefilled
- `:generate <name> [length]` — add a password-only entry
- `:account` — register the account on the selected entry (same as `U`)
- `:rename <old> <new>` — move an entry
- `:delete [name]` — delete an entry (defaults to the one under the cursor)
- `:theme [name]` — list the palettes, or switch to one
- `:config` — open the config in `$EDITOR`; passer reloads it on save
- `:reload` — re-read the store from disk
- `:help` — the keybinding reference
- `:quit` — exit

## Security model

What passer guarantees, and what it doesn't.

### What it does

- **Decrypts lazily.** Browsing and searching never invoke GPG. Only opening or
  copying a specific entry does.
- **Never writes plaintext to disk.** Decrypted entries exist in memory, and
  only while the detail view or an edit prompt is open. An entry decrypted from
  the list purely to serve an edit is released as soon as that prompt closes,
  and the idle lock drops it either way.
- **Keeps secrets out of the process list.** Passwords reach `pass insert` and
  the clipboard tool through stdin, never as command-line arguments.
- **Masks by default.** Revealing is always a deliberate keystroke.
- **Bounds every exposure with a visible timer.** All three are configurable,
  and each is shown in the status bar while it runs.

| Timer | Default | What it does |
| --- | --- | --- |
| `clip_clear_seconds` | 45s | Wipes the copied secret from the clipboard |
| `reveal_seconds` | 30s | Re-masks a revealed password |
| `lock_seconds` | 120s | Drops the decrypted entry and returns to the list |

The clipboard is only wiped **if it still holds what passer put there**. Without
that check, an auto-clear firing 45 seconds after a copy would destroy whatever
you copied in the meantime. If the clipboard can't be read back, passer clears
it anyway — leaving a password sitting there is the worse failure.

Setting `clip_clear_seconds = 0` disables auto-clear. Passer says so explicitly
when you copy, rather than implying a timer that will never fire.

### What it doesn't

- **Passer cannot scrub your scrollback.** A revealed password has been written
  to the terminal, and re-masking it does not un-write it. Passer runs in the
  alternate screen buffer, which keeps it out of your main scrollback in most
  terminals, but this is a property of your terminal, not a guarantee passer can
  make. Prefer copying over revealing.
- **Go strings cannot be zeroed.** Dropping an entry removes passer's reference
  to the plaintext and makes it collectable, but the bytes may persist in the
  heap until the process exits. This is a limitation of the runtime, not a
  design choice, and it applies equally to any Go password manager.
- **The clipboard is a shared, unprivileged resource.** Anything running as your
  user can read it during the window before it clears.
- **Passer inherits pass's trust model.** It adds no cryptography of its own,
  and can be no more secure than your GPG key and agent configuration.

## Theme

Passer ships seven palettes with explicit canvas and text colors:
**light** (default), **dark**, **purple**, **ocean**, **forest**, **rose**, and
**graphite**. There are three ways to switch:

- **`:theme` command** — run `:theme` to list them, or `:theme purple` to
  switch. The change applies immediately and is saved to your config.
- **Cycle live** — press `t` to rotate through the presets, applied and saved as
  you go.

Switching this way rewrites only the `[theme]` table of your config, replacing
it with the new `preset` line; the rest of the file, comments included, is
left as it is. Any color overrides in `[theme]` are dropped with the old
palette.
- **Hand-tune** — set `preset` in `[theme]` and override individual keys. Any
  color you set wins over the preset, so you can start from `ocean` and adjust a
  single hue.

The light palette uses a slate canvas with blue accents; dark uses deep slate
with soft blue accents. Selected rows use a consistent contrasting text color,
with underlined search matches.

Theme keys control the canvas and framing:

- `background` / `foreground` — the full canvas and default text colors

- `border` — the rounded panel frames
- `status_alt_bg` / `status_alt_fg` — the table-header bar and the key-hint chips

One passer-specific note: `warning` is the color revealed secrets are drawn in.
It is meant to be loud — it should be obvious at a glance that something
sensitive is on screen.

## Configuration

- Config: `$XDG_CONFIG_HOME/passer/config.toml` (default
  `~/.config/passer/config.toml`), written with mode `0600`.
- Override the path with `$PASSER_CONFIG`.
- Passer stores no data of its own. Your entries belong to `pass`, in the
  password store.

Every key is rebindable under `[keys]`; see
[`config.example.toml`](config.example.toml) for the full reference. The help
screen renders from your live bindings, so it stays correct after you change
them.

## Install (Linux)

```bash
./install.sh
```

Options:

```bash
./install.sh --prefix /usr/local --bin-name passer
```

The installer does a clean rebuild every time, writes a default config if you
don't have one, and warns if another `passer` earlier on your `PATH` would
shadow the freshly built binary — the usual cause of an "old version keeps
running" after an update. It also warns if `pass` or a clipboard tool is
missing.

To verify the checkout:

```bash
go test ./...
```

## Uninstall (Linux)

```bash
./uninstall.sh
```

Removes the installed binary and the local build artifact. If another
`passer` is still on your `PATH`, it asks before removing each one (run
non-interactively, it just reports them). Use the same `--prefix`/`--bin-name` you installed
with. To also delete the config:

```bash
./uninstall.sh --purge
```

**Your password store is never touched.** It belongs to `pass`; uninstalling
passer just removes the front end.

## Testing

```bash
go test ./...          # unit tests; nothing here decrypts
scripts/sandbox.sh     # run passer against a throwaway key and store
```

The sandbox creates a temporary GPG key (no passphrase), a password store
seeded with sample entries, and its own config, then starts passer from the
current sources. Your real store, keyring, and `~/.config/passer` are never
touched, and everything is deleted when passer exits. The clipboard is the real
one, so copy tests behave as they do in daily use.

- `scripts/sandbox.sh pass ls` runs any command inside the sandbox instead.
- `--keep` leaves the sandbox in place and prints the variables to reuse it.
