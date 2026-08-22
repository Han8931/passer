# Passer

<p align="center">
  A calm, Vim-first terminal front end for <a href="https://www.passwordstore.org/">pass</a>,
  the standard unix password manager.
</p>

Passer gives your password store a keyboard-driven interface — fuzzy search,
one-key copy, and full editing — without changing where anything lives. Your
store stays a directory of GPG files that `pass` and `git` still understand.

## Why a TUI for pass?

`pass` is excellent and does exactly one thing well. What it doesn't give you is
a way to *look around*: finding an entry means remembering its path or piping
`pass ls` through a pager, and reading one field means printing every secret in
the file to your scrollback.

**Passer adds the browsing, and takes away the leaking.** Nothing is decrypted
until you ask for a specific entry, secrets are masked by default, and anything
that does end up exposed — on screen or on the clipboard — is on a visible
timer.

## Highlights

- **Find by typing** — fuzzy search across the whole store, ranked so the entry
  you meant is under the cursor by the third keystroke.
- **Copy without looking** — one key for the password, the username, or the URL,
  straight from the list. Nothing is ever displayed.
- **Auto-clearing clipboard** — copied secrets are wiped after 45 seconds, with
  a live countdown, and only if the clipboard still holds what passer put there.
- **Masked by default** — passwords, OTP seeds, and recovery codes render as
  dots until you reveal them, and re-mask on a timer.
- **Idle lock** — decrypted entries are dropped from memory after two minutes
  without a keypress.
- **Full editing** — add, generate, rename, and delete entries; `e` hands the
  file to `$EDITOR` through `pass edit`.
- **Personal by design** — every key is rebindable and seven built-in themes
  ship with full color customization.

## Nothing new to trust

Passer is a front end, not a re-implementation:

- Every read and write goes through the `pass` binary, so GPG key resolution,
  per-directory `.gpg-id` files, pinentry, and the store's git integration all
  behave exactly as they do on the command line.
- Decrypted entries are held in memory only, for as long as the detail view is
  open. Passer never writes plaintext to disk.
- Secrets reach the clipboard through the tool's stdin, never as a command-line
  argument, so they don't appear in the process list.

The one thing passer does itself is generate passwords, so the add dialog can
show you the password before it is stored. It draws from `crypto/rand` — the
same kernel CSPRNG `pass generate` reads — over the same character set.

## Quick Start

Passer supports Linux and macOS, and requires Go 1.25 or newer plus a working
`pass` installation.

```bash
git clone https://github.com/Han8931/passer.git
cd passer
./install.sh
passer
```

Or run directly from a source checkout:

```bash
go build -o bin/passer ./cmd/passer
./bin/passer
```

If you have never used `pass`, initialise the store first:

```bash
pass init <your-gpg-id>
```

For copying you also need a clipboard tool: `wl-clipboard` on Wayland, `xclip`
or `xsel` on X11, `pbcopy` on macOS. Passer runs without one and will tell you
when a copy can't go through.

## Your First Minute

- `j` / `k` — move through entries
- `/` — fuzzy search; `Enter` opens the best match
- `y` — copy the password (auto-clears after 45s)
- `u` — copy the username
- `Enter` — open the entry and see its fields
- `space` — reveal the password while the entry is open
- `a` — add an entry (leave the password blank to generate one)
- `?` — the complete in-app keybinding reference
- `q` — back, or quit from the unfiltered list

## Documentation

Read the **[User Guide](GUIDE.md)** for the full walkthrough, including:

- [the list and detail views](GUIDE.md#views)
- [searching and copying](GUIDE.md#daily-use)
- [adding and generating entries](GUIDE.md#adding-entries)
- [the security timers](GUIDE.md#security-model)
- [themes and configuration](GUIDE.md#theme)
- [installation and removal](GUIDE.md#install-linux)

The full configuration reference is
[`config.example.toml`](config.example.toml).

## Development

```bash
go test ./...
```

## Not included

- **OTP codes.** Passer displays an `otp` field (masked) but does not compute
  TOTP codes; that is [pass-otp](https://github.com/tadfisher/pass-otp)'s job.
- **Store git operations.** `pass` commits store changes itself if the store is
  a git repository, but passer has no UI for `pass git`.
- **Multi-select.** Every destructive action applies to one entry at a time, on
  purpose.
