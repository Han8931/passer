package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"passer/internal/config"
	"passer/internal/pass"
)

func buildStyles(theme config.Theme) uiStyles {
	styles := uiStyles{
		Title:     lipgloss.NewStyle().Bold(true),
		Heading:   lipgloss.NewStyle().Bold(true),
		Accent:    lipgloss.NewStyle().Bold(true),
		Muted:     lipgloss.NewStyle(),
		Border:    lipgloss.NewStyle(),
		Selection: lipgloss.NewStyle().Bold(true),
		Danger:    lipgloss.NewStyle().Bold(true),
		Warning:   lipgloss.NewStyle(),
		Success:   lipgloss.NewStyle(),
		Status:    lipgloss.NewStyle(),
		StatusAlt: lipgloss.NewStyle(),
	}

	// Explicit canvas colors keep palettes readable on either terminal theme.
	background, foreground := theme.Background, theme.Foreground
	if background == "" {
		background = theme.StatusBg
	}
	if foreground == "" {
		foreground = theme.StatusFg
	}
	styles.Base = applyFg(applyBg(lipgloss.NewStyle(), background), foreground)
	styles.Title = applyFg(styles.Title, theme.Title)
	styles.Heading = applyFg(styles.Heading, theme.Heading)
	styles.Accent = applyFg(styles.Accent, theme.Accent)
	styles.Muted = applyFg(styles.Muted, theme.Muted)
	styles.Border = applyFg(styles.Border, theme.Border)
	styles.Danger = applyFg(styles.Danger, theme.Danger)
	styles.Warning = applyFg(styles.Warning, theme.Warning)
	styles.Success = applyFg(styles.Success, theme.Success)

	styles.Selection = applyBg(styles.Selection, theme.SelectionBg)
	selectionFg := theme.SelectionFg
	if strings.TrimSpace(selectionFg) == "" {
		selectionFg = theme.StatusFg
	}
	styles.Selection = applyFg(styles.Selection, selectionFg)

	styles.Status = applyBg(styles.Status, theme.StatusBg)
	styles.Status = applyFg(styles.Status, theme.StatusFg)
	styles.StatusAlt = applyBg(styles.StatusAlt, theme.StatusAltBg)
	styles.StatusAlt = applyFg(styles.StatusAlt, theme.StatusAltFg)

	// Structural styles derived from the theme so the framing stays
	// configurable and matches bada's look.
	styles.Panel = applyFg(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()), theme.Border)
	styles.PanelTitle = applyFg(lipgloss.NewStyle().Bold(true), theme.Accent)

	styles.TableHeader = lipgloss.NewStyle().Bold(true)
	styles.TableHeader = applyBg(styles.TableHeader, theme.StatusAltBg)
	styles.TableHeader = applyFg(styles.TableHeader, theme.StatusAltFg)

	styles.KeyCap = lipgloss.NewStyle().Bold(true)
	styles.KeyCap = applyBg(styles.KeyCap, theme.StatusAltBg)
	styles.KeyCap = applyFg(styles.KeyCap, theme.StatusAltFg)
	styles.KeyLabel = applyFg(lipgloss.NewStyle(), theme.Muted)

	// A revealed secret is deliberately loud: it should be obvious at a glance
	// that something sensitive is on screen right now.
	styles.Secret = applyFg(lipgloss.NewStyle().Bold(true), theme.Warning)
	styles.Mask = applyFg(lipgloss.NewStyle(), theme.Muted)
	styles.MatchHit = applyFg(lipgloss.NewStyle().Bold(true), theme.Accent)

	return styles
}

func applyFg(style lipgloss.Style, color string) lipgloss.Style {
	if strings.TrimSpace(color) == "" {
		return style
	}
	return style.Foreground(lipgloss.Color(color))
}

func applyBg(style lipgloss.Style, color string) lipgloss.Style {
	if strings.TrimSpace(color) == "" {
		return style
	}
	return style.Background(lipgloss.Color(color))
}

// ---------------------------------------------------------------------------
// Key dispatch
// ---------------------------------------------------------------------------

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	// Any keypress is proof the user is still here, so the idle lock restarts.
	m.lockLeft = m.cfg.Security.LockSeconds

	switch m.mode {
	case modeAdd:
		return m.keyAdd(msg)
	case modeRename:
		return m.keyRename(msg)
	case modeCommand:
		return m.keyCommand(msg)
	case modeSearch:
		return m.keySearch(msg)
	case modeConfirmDelete:
		return m.keyConfirmDelete(msg)
	case modeEditField:
		return m.keyEditField(msg)
	case modeHelp:
		return m.keyHelp(msg)
	case modeDetail:
		return m.keyDetail(msg)
	default:
		return m.keyList(msg)
	}
}

func (m Model) keyList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	k := m.cfg.Keys

	// gg is the only two-key motion, so a pending 'g' is resolved before
	// anything else gets a look at the keystroke.
	if m.navBuf == "g" {
		m.navBuf = ""
		if key == "g" {
			m.cursor = 0
			m.syncScroll()
			return m, nil
		}
	}

	switch key {
	case "g":
		m.navBuf = "g"
		return m, nil
	case "G":
		m.cursor = len(m.matches) - 1
		m.moveCursor(0)
		return m, nil
	case "ctrl+d":
		m.moveCursor(m.listRows() / 2)
		return m, nil
	case "ctrl+u":
		m.moveCursor(-m.listRows() / 2)
		return m, nil
	}

	switch key {
	case k.Up, "up", "ctrl+p":
		m.moveCursor(-1)
		return m, nil
	case k.Down, "down", "ctrl+n":
		m.moveCursor(1)
		return m, nil
	}

	switch key {
	case k.Quit:
		// q backs out of a filter first; only an unfiltered list quits, so an
		// active search can never be mistaken for the whole store.
		if m.query != "" {
			m.query = ""
			m.applyFilter()
			return m, nil
		}
		return m, tea.Quit

	case k.Cancel:
		if m.query != "" {
			m.query = ""
			m.applyFilter()
		}
		m.status, m.statusErr = "", false
		return m, nil

	case k.Search:
		m.mode = modeSearch
		m.input.SetValue(m.query)
		m.input.Placeholder = "fuzzy filter"
		m.input.EchoMode = textinput.EchoNormal
		m.input.CursorEnd()
		m.input.Focus()
		return m, nil

	case k.Command:
		m.mode = modeCommand
		m.prevMode = modeList
		m.input.SetValue("")
		m.input.Placeholder = "command"
		m.input.EchoMode = textinput.EchoNormal
		m.input.Focus()
		m.commandHistoryIdx = -1
		return m, nil

	case k.Help:
		m.prevMode = modeList
		m.mode = modeHelp
		m.helpScroll = 0
		return m, nil

	case k.Reload:
		m.status, m.statusErr = "reloading…", false
		return m, loadItemsCmd(m.store)

	case k.ThemeToggle:
		return m.cycleTheme()

	case k.Open:
		return m.requestEntry(intentOpen)
	case k.CopyPassword:
		return m.requestEntry(intentCopyPassword)
	case k.CopyUsername:
		return m.requestEntry(intentCopyUsername)
	case k.CopyURL:
		return m.requestEntry(intentCopyURL)

	case k.SetUsername:
		return m.requestEntry(intentSetUsername)

	case k.Add:
		return m.openAdd()

	case k.Edit, k.EditFile:
		// No row is selected in the list, so there is no single value to edit;
		// both keys open the whole file in $EDITOR.
		name := m.currentName()
		if name == "" {
			return m, nil
		}
		return m, editCmd(m.store, name)

	case k.Delete:
		name := m.currentName()
		if name == "" {
			return m, nil
		}
		m.confirmDelete = name
		m.mode = modeConfirmDelete
		return m, nil

	case k.Rename:
		name := m.currentName()
		if name == "" {
			return m, nil
		}
		m.renameFrom = name
		m.mode = modeRename
		m.input.SetValue(name)
		m.input.Placeholder = "new name"
		m.input.EchoMode = textinput.EchoNormal
		m.input.CursorEnd()
		m.input.Focus()
		return m, nil
	}

	return m, nil
}

// requestEntry kicks off a decrypt for the entry under the cursor.
func (m Model) requestEntry(intent loadIntent) (tea.Model, tea.Cmd) {
	name := m.currentName()
	if name == "" {
		return m, nil
	}
	m.loading = name
	m.status, m.statusErr = "", false
	return m, loadEntryCmd(m.store, name, intent)
}

func (m Model) keySearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Leave the filter applied: escaping the prompt is how you get back to
		// the list keys with a narrowed set still on screen.
		m.mode = modeList
		m.input.Blur()
		return m, nil

	case "enter":
		m.mode = modeList
		m.input.Blur()
		if len(m.matches) == 0 {
			return m, nil
		}
		return m.requestEntry(intentOpen)

	case "up", "ctrl+p":
		m.moveCursor(-1)
		return m, nil
	case "down", "ctrl+n":
		m.moveCursor(1)
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.query = m.input.Value()
	m.applyFilter()
	// While typing, the best match should always be under the cursor; keeping
	// the previous selection would fight the query the user is refining.
	m.cursor, m.scroll = 0, 0
	return m, cmd
}

func (m Model) keyDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	k := m.cfg.Keys
	rows := m.detailRows()

	switch key {
	case k.Quit, k.Cancel:
		// Closing the detail view is what drops the plaintext.
		m.entry = nil
		m.revealed = false
		m.revealLeft = 0
		m.mode = modeList
		return m, nil

	case k.Up, "up", "ctrl+p":
		if m.entryCursor > 0 {
			m.entryCursor--
		}
		return m, nil

	case k.Down, "down", "ctrl+n":
		if m.entryCursor < len(rows)-1 {
			m.entryCursor++
		}
		return m, nil

	case k.Reveal:
		if m.entry == nil {
			return m, nil
		}
		m.revealed = !m.revealed
		if m.revealed {
			m.revealLeft = m.cfg.Security.RevealSeconds
			return m, m.ensureTicking()
		}
		m.revealLeft = 0
		return m, nil

	case k.Help:
		m.prevMode = modeDetail
		m.mode = modeHelp
		m.helpScroll = 0
		return m, nil

	case k.CopyPassword:
		if m.entry == nil {
			return m, nil
		}
		return m.startCopy("password", m.entry.Password)

	case k.CopyUsername:
		if m.entry == nil {
			return m, nil
		}
		return m.copyUsername(*m.entry)

	case k.CopyURL:
		if m.entry == nil {
			return m, nil
		}
		value, ok := m.entry.URL()
		if !ok {
			m.status, m.statusErr = "no url field", true
			return m, nil
		}
		return m.startCopy("url", value)

	case k.Open, "c":
		// Copy whatever the cursor is on, which is how you get at fields that
		// have no dedicated key.
		if m.entryCursor < 0 || m.entryCursor >= len(rows) {
			return m, nil
		}
		row := rows[m.entryCursor]
		return m.startCopy(row.label, row.value)

	case k.SetUsername:
		if m.entry == nil {
			return m, nil
		}
		return m.openSetAccount(*m.entry, modeDetail)

	case k.Edit:
		// In an open entry the cursor is on a specific value, so `e` edits that
		// one value in place. The whole file is still one keystroke away on E.
		if m.entry == nil || m.entryCursor < 0 || m.entryCursor >= len(rows) {
			return m, nil
		}
		return m.openEditField(*m.entry, rows[m.entryCursor], modeDetail)

	case k.EditFile:
		if m.entry == nil {
			return m, nil
		}
		return m, editCmd(m.store, m.entry.Name)

	case k.ThemeToggle:
		return m.cycleTheme()

	case k.Command:
		m.mode = modeCommand
		m.prevMode = modeDetail
		m.input.SetValue("")
		m.input.Placeholder = "command"
		m.input.EchoMode = textinput.EchoNormal
		m.input.Focus()
		m.commandHistoryIdx = -1
		return m, nil
	}

	// Digits jump straight to a row and copy it, for the muscle memory of
	// "third field, copy".
	if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(rows) {
		m.entryCursor = n - 1
		row := rows[n-1]
		return m.startCopy(row.label, row.value)
	}

	return m, nil
}

func (m Model) keyHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "?":
		m.mode = m.prevMode
		if m.mode == modeDetail && m.entry == nil {
			m.mode = modeList
		}
		return m, nil
	case "j", "down":
		if m.helpScroll < m.helpMaxScroll() {
			m.helpScroll++
		}
		return m, nil
	case "k", "up":
		if m.helpScroll > 0 {
			m.helpScroll--
		}
		return m, nil
	case "ctrl+d":
		m.helpScroll += m.helpVisibleRows() / 2
		if m.helpScroll > m.helpMaxScroll() {
			m.helpScroll = m.helpMaxScroll()
		}
		return m, nil
	case "ctrl+u":
		m.helpScroll -= m.helpVisibleRows() / 2
		if m.helpScroll < 0 {
			m.helpScroll = 0
		}
		return m, nil
	case "g":
		m.helpScroll = 0
		return m, nil
	case "G":
		m.helpScroll = m.helpMaxScroll()
		return m, nil
	}
	return m, nil
}

func (m Model) keyConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		name := m.confirmDelete
		m.confirmDelete = ""
		m.mode = modeList
		m.status, m.statusErr = "deleting "+name+"…", false
		return m, removeCmd(m.store, name)
	default:
		m.confirmDelete = ""
		m.mode = modeList
		m.status, m.statusErr = "cancelled", false
		return m, nil
	}
}

func (m Model) keyRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.renameFrom = ""
		m.input.Blur()
		return m, nil

	case "enter":
		to := strings.TrimSpace(m.input.Value())
		from := m.renameFrom
		if to == "" || to == from {
			m.mode = modeList
			m.renameFrom = ""
			m.input.Blur()
			return m, nil
		}
		if err := pass.ValidateName(to); err != nil {
			m.status, m.statusErr = err.Error(), true
			return m, nil
		}
		m.mode = modeList
		m.renameFrom = ""
		m.input.Blur()
		return m, moveCmd(m.store, from, to)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// ---------------------------------------------------------------------------
// Add dialog
// ---------------------------------------------------------------------------

func (m Model) openAdd() (tea.Model, tea.Cmd) {
	add := &addState{inputs: make([]textinput.Model, 5)}
	placeholders := []string{
		"web/example.com",
		"blank to generate",
		"you@example.com",
		"https://example.com",
		"free-form notes",
	}
	for i := range add.inputs {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 512
		// Width must be set explicitly: at the zero value bubbles clips the
		// placeholder to a single character, so every field would show one
		// stray letter instead of its hint.
		ti.Width = 44
		ti.Placeholder = placeholders[i]
		add.inputs[i] = ti
	}
	add.inputs[fieldPassword].EchoMode = textinput.EchoPassword
	add.inputs[fieldPassword].EchoCharacter = '•'

	// Pre-fill the name with the folder the cursor is in, so adding a second
	// entry to web/ doesn't mean retyping the prefix.
	if name := m.currentName(); name != "" {
		if idx := strings.LastIndex(name, "/"); idx > 0 {
			add.inputs[fieldName].SetValue(name[:idx+1])
			add.inputs[fieldName].CursorEnd()
		}
	}

	add.index = fieldName
	add.inputs[fieldName].Focus()
	m.add = add
	m.mode = modeAdd
	m.status, m.statusErr = "", false
	return m, nil
}

func (m Model) keyAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.add == nil {
		m.mode = modeList
		return m, nil
	}
	a := m.add

	switch msg.String() {
	case "esc":
		m.add = nil
		m.mode = modeList
		return m, nil

	case "tab", "down":
		a.focusDelta(1)
		return m, nil

	case "shift+tab", "up":
		a.focusDelta(-1)
		return m, nil

	case "ctrl+r":
		// Reveal what was typed, for checking a pasted or generated password.
		a.reveal = !a.reveal
		if a.reveal {
			a.inputs[fieldPassword].EchoMode = textinput.EchoNormal
		} else {
			a.inputs[fieldPassword].EchoMode = textinput.EchoPassword
		}
		return m, nil

	case "ctrl+g":
		pw, err := pass.NewPassword(m.cfg.Generate.Length, m.cfg.Generate.Symbols)
		if err != nil {
			a.validate = err.Error()
			return m, nil
		}
		a.inputs[fieldPassword].SetValue(pw)
		a.inputs[fieldPassword].CursorEnd()
		a.validate = ""
		// Show it: a password you generated but never see is one you can't
		// check against a site's character rules.
		a.reveal = true
		a.inputs[fieldPassword].EchoMode = textinput.EchoNormal
		return m, nil

	case "ctrl+s":
		return m.submitAdd()

	case "enter":
		if a.index == fieldMore {
			a.expanded = !a.expanded
			return m, nil
		}
		return m.submitAdd()
	}

	if a.index == fieldMore {
		// The toggle row has no input to type into; space and → open it.
		switch msg.String() {
		case " ", "right", "l":
			a.expanded = !a.expanded
		}
		return m, nil
	}

	var cmd tea.Cmd
	a.inputs[a.index], cmd = a.inputs[a.index].Update(msg)
	return m, cmd
}

// focusDelta moves the modal's focus along the visible rows, wrapping.
func (a *addState) focusDelta(delta int) {
	order := a.order()
	pos := a.orderPos() + delta
	if pos < 0 {
		pos = len(order) - 1
	}
	if pos >= len(order) {
		pos = 0
	}
	for i := range a.inputs {
		a.inputs[i].Blur()
	}
	a.index = order[pos]
	if a.index != fieldMore {
		a.inputs[a.index].Focus()
	}
}

func (m Model) submitAdd() (tea.Model, tea.Cmd) {
	a := m.add
	name := strings.TrimSpace(a.inputs[fieldName].Value())
	if err := pass.ValidateName(name); err != nil {
		a.validate = err.Error()
		return m, nil
	}
	if m.store.Exists(name) {
		// Refuse rather than pass --force through: silently replacing an entry
		// would destroy a password with no way back.
		a.validate = name + " already exists"
		return m, nil
	}

	password := a.inputs[fieldPassword].Value()
	generated := false
	if strings.TrimSpace(password) == "" {
		pw, err := pass.NewPassword(m.cfg.Generate.Length, m.cfg.Generate.Symbols)
		if err != nil {
			a.validate = err.Error()
			return m, nil
		}
		password = pw
		generated = true
	}

	entry := pass.Entry{Name: name, Password: password}
	if v := strings.TrimSpace(a.inputs[fieldUsername].Value()); v != "" {
		entry.Fields = append(entry.Fields, pass.Field{Key: "login", Value: v})
	}
	if v := strings.TrimSpace(a.inputs[fieldURL].Value()); v != "" {
		entry.Fields = append(entry.Fields, pass.Field{Key: "url", Value: v})
	}
	if v := strings.TrimSpace(a.inputs[fieldNotes].Value()); v != "" {
		entry.Notes = append(entry.Notes, v)
	}

	m.add = nil
	m.mode = modeList
	if generated {
		m.status, m.statusErr = "generated "+name+"…", false
	} else {
		m.status, m.statusErr = "saving "+name+"…", false
	}
	return m, insertCmd(m.store, name, entry.Compose())
}

// ---------------------------------------------------------------------------
// Field editor
// ---------------------------------------------------------------------------

// openSetAccount opens the editor on an entry's account identifier, pre-filled
// with whatever passer currently resolves — so an inferred value can be
// confirmed into a real field with one keystroke, and a wrong guess corrected
// rather than only reported.
//
// from is the mode to return to. Opening from the list means this decrypt
// exists only to serve the rewrite, so closing drops the plaintext again.
func (m Model) openSetAccount(entry pass.Entry, from mode) (tea.Model, tea.Cmd) {
	current, _, _ := entry.UsernameFrom(m.cfg.UsernameFields)
	return m.openPrompt(entry, from, &fieldPrompt{
		name:    entry.Name,
		from:    from,
		label:   "account",
		line:    accountRow,
		account: true,
	}, current, "you@example.com")
}

// openEditField opens the editor on the detail row under the cursor.
//
// A row derived from the entry's name has no line to rewrite, so editing it
// means registering an account instead — the one case that adds a line rather
// than replacing one.
func (m Model) openEditField(entry pass.Entry, row detailRow, from mode) (tea.Model, tea.Cmd) {
	if row.line == accountRow {
		return m.openSetAccount(entry, from)
	}
	return m.openPrompt(entry, from, &fieldPrompt{
		name:   entry.Name,
		from:   from,
		label:  row.label,
		line:   row.line,
		secret: row.secret,
	}, row.value, "value")
}

// openPrompt is the shared setup for both entry points.
func (m Model) openPrompt(entry pass.Entry, from mode, prompt *fieldPrompt, value, placeholder string) (tea.Model, tea.Cmd) {
	e := entry
	m.entry = &e
	m.field = prompt
	m.mode = modeEditField

	m.input.SetValue(value)
	m.input.Placeholder = placeholder
	// A masked row stays masked while it is edited, so a shoulder-surfer gains
	// nothing from watching someone correct a typo in a TOTP seed. ctrl+r
	// reveals it, exactly as in the New Entry dialog.
	if prompt.secret {
		m.input.EchoMode = textinput.EchoPassword
		m.input.EchoCharacter = '•'
	} else {
		m.input.EchoMode = textinput.EchoNormal
	}
	m.input.CursorEnd()
	m.input.Focus()
	m.status, m.statusErr = "", false
	m.lockLeft = m.cfg.Security.LockSeconds
	return m, m.ensureTicking()
}

// closePrompt leaves the editor, releasing the plaintext when it was only
// decrypted for this.
func (m Model) closePrompt() Model {
	from := modeList
	if m.field != nil {
		from = m.field.from
	}
	m.field = nil
	m.input.Blur()
	m.input.SetValue("")
	m.input.Placeholder = ""
	m.input.EchoMode = textinput.EchoNormal
	m.mode = from
	if from != modeDetail {
		m.entry = nil
		m.revealed = false
		m.revealLeft = 0
	}
	return m
}

func (m Model) keyEditField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.field
	if f == nil {
		return m.closePrompt(), nil
	}

	switch msg.String() {
	case "esc":
		return m.closePrompt(), nil

	case "ctrl+r":
		if !f.secret {
			return m, nil
		}
		f.reveal = !f.reveal
		if f.reveal {
			m.input.EchoMode = textinput.EchoNormal
		} else {
			m.input.EchoMode = textinput.EchoPassword
		}
		return m, nil

	case "ctrl+g":
		// Only on the password line: generating a value for a url or a note
		// would be nonsense, and silently overwriting one would be worse.
		if f.line != 0 {
			return m, nil
		}
		pw, err := pass.NewPassword(m.cfg.Generate.Length, m.cfg.Generate.Symbols)
		if err != nil {
			m.status, m.statusErr = err.Error(), true
			return m, nil
		}
		m.input.SetValue(pw)
		m.input.CursorEnd()
		// Show it: a password you generated but never see is one you cannot
		// check against a site's character rules.
		f.reveal = true
		m.input.EchoMode = textinput.EchoNormal
		return m, nil

	case "enter":
		return m.submitEditField()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) submitEditField() (tea.Model, tea.Cmd) {
	entry, f := m.entry, m.field
	if entry == nil || f == nil {
		// The idle lock dropped the plaintext while the editor was open.
		mm := m.closePrompt()
		mm.status, mm.statusErr = "entry locked; open it again", true
		return mm, nil
	}

	value := strings.TrimSpace(m.input.Value())
	// An empty prompt cancels rather than clearing the value: losing a password
	// or an account should never be one keystroke away. Removing a line is what
	// E and $EDITOR are for.
	if value == "" {
		mm := m.closePrompt()
		mm.status, mm.statusErr = "cancelled", false
		return mm, nil
	}

	content, err := m.fieldContent(*entry, *f, value)
	if err != nil {
		mm := m.closePrompt()
		mm.status, mm.statusErr = err.Error(), true
		return mm, nil
	}
	name := entry.Name
	if content == "" {
		mm := m.closePrompt()
		mm.status, mm.statusErr = name+" unchanged", false
		return mm, nil
	}

	mm := m.closePrompt()
	mm.status, mm.statusErr = "saving "+name+"…", false
	return mm, updateCmd(mm.store, name, content)
}

// fieldContent builds the entry's new file content, or "" when the edit is a
// no-op.
func (m Model) fieldContent(entry pass.Entry, f fieldPrompt, value string) (string, error) {
	if f.account {
		// Submitting an unchanged value is only a no-op when it already lives in
		// a field. When it was *inferred* from a bare line or from the entry's
		// name, accepting the pre-filled value is the whole point of the prompt:
		// it is how an inferred account becomes a recorded one.
		current, source, _ := entry.UsernameFrom(m.cfg.UsernameFields)
		if value == current && source == pass.UsernameField {
			return "", nil
		}
		return entry.SetUsername(value, pass.UsernameKeys(m.cfg.UsernameFields)), nil
	}

	if f.line == 0 {
		if value == entry.Password {
			return "", nil
		}
	} else {
		for _, l := range entry.Lines {
			if l.Index == f.line && value == l.Value {
				return "", nil
			}
		}
	}
	return entry.SetValueAt(f.line, value)
}

// ---------------------------------------------------------------------------
// Command line
// ---------------------------------------------------------------------------

// commandNames are the : commands, in the order Tab cycles them.
var commandNames = []string{
	"account", "add", "config", "delete", "generate", "help", "quit", "reload", "rename", "theme",
}

func (m Model) keyCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = m.prevMode
		m.input.Blur()
		return m, nil

	case "enter":
		raw := strings.TrimSpace(m.input.Value())
		m.mode = m.prevMode
		m.input.Blur()
		m.input.SetValue("")
		if raw == "" {
			return m, nil
		}
		m.commandHistory = append(m.commandHistory, raw)
		m.commandHistoryIdx = -1
		return m.runCommand(raw)

	case "tab":
		m.input.SetValue(completeCommand(m.input.Value()))
		m.input.CursorEnd()
		return m, nil

	case "up":
		return m.commandHistoryStep(1)
	case "down":
		return m.commandHistoryStep(-1)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// commandHistoryStep walks backwards (delta 1) or forwards (delta -1) through
// previously run commands.
func (m Model) commandHistoryStep(delta int) (tea.Model, tea.Cmd) {
	if len(m.commandHistory) == 0 {
		return m, nil
	}
	idx := m.commandHistoryIdx + delta
	if idx < -1 {
		idx = -1
	}
	if idx >= len(m.commandHistory) {
		idx = len(m.commandHistory) - 1
	}
	m.commandHistoryIdx = idx
	if idx < 0 {
		m.input.SetValue("")
	} else {
		m.input.SetValue(m.commandHistory[len(m.commandHistory)-1-idx])
	}
	m.input.CursorEnd()
	return m, nil
}

// completeCommand fills in the unambiguous part of a command name.
func completeCommand(input string) string {
	prefix := strings.TrimSpace(input)
	if strings.Contains(prefix, " ") {
		return input
	}
	var hits []string
	for _, name := range commandNames {
		if strings.HasPrefix(name, prefix) {
			hits = append(hits, name)
		}
	}
	if len(hits) == 0 {
		return input
	}
	if len(hits) == 1 {
		return hits[0] + " "
	}
	sort.Strings(hits)
	// Extend to the longest shared prefix, the same way shell completion does.
	common := hits[0]
	for _, h := range hits[1:] {
		for !strings.HasPrefix(h, common) {
			common = common[:len(common)-1]
		}
	}
	return common
}

func (m Model) runCommand(raw string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(raw)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "q", "quit", "exit":
		return m, tea.Quit

	case "help", "h":
		m.prevMode = modeList
		m.mode = modeHelp
		m.helpScroll = 0
		return m, nil

	case "reload", "r":
		m.status, m.statusErr = "reloading…", false
		return m, loadItemsCmd(m.store)

	case "config":
		return m, editConfigCmd(m.configPath)

	case "theme":
		if len(args) == 0 {
			m.status, m.statusErr = "themes: "+strings.Join(config.ThemePresetNames(), ", "), false
			return m, nil
		}
		return m.applyTheme(args[0])

	case "add", "new", "insert":
		if len(args) == 0 {
			return m.openAdd()
		}
		model, cmd := m.openAdd()
		mm := model.(Model)
		mm.add.inputs[fieldName].SetValue(args[0])
		mm.add.inputs[fieldName].CursorEnd()
		return mm, cmd

	case "account", "user", "username":
		// The entry has to be decrypted before it can be rewritten, so this is
		// the same two-step the U key takes.
		if m.mode == modeDetail && m.entry != nil {
			return m.openSetAccount(*m.entry, modeDetail)
		}
		if m.currentName() == "" {
			m.status, m.statusErr = "no entry selected", true
			return m, nil
		}
		return m.requestEntry(intentSetUsername)

	case "generate", "gen":
		return m.commandGenerate(args)

	case "rename", "mv", "move":
		if len(args) != 2 {
			m.status, m.statusErr = "usage: :rename <old> <new>", true
			return m, nil
		}
		return m, moveCmd(m.store, args[0], args[1])

	case "delete", "rm":
		name := strings.Join(args, " ")
		if name == "" {
			name = m.currentName()
		}
		if name == "" {
			m.status, m.statusErr = "usage: :delete <name>", true
			return m, nil
		}
		if !m.store.Exists(name) {
			m.status, m.statusErr = name+" not found", true
			return m, nil
		}
		m.confirmDelete = name
		m.mode = modeConfirmDelete
		return m, nil
	}

	m.status, m.statusErr = "unknown command: "+cmd, true
	return m, nil
}

// commandGenerate handles ":generate <name> [length]", the shortcut for adding
// an entry that is nothing but a password.
func (m Model) commandGenerate(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.status, m.statusErr = "usage: :generate <name> [length]", true
		return m, nil
	}
	name := args[0]
	length := m.cfg.Generate.Length
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			m.status, m.statusErr = "length must be a positive number", true
			return m, nil
		}
		length = n
	}
	if err := pass.ValidateName(name); err != nil {
		m.status, m.statusErr = err.Error(), true
		return m, nil
	}
	if m.store.Exists(name) {
		m.status, m.statusErr = name+" already exists", true
		return m, nil
	}

	password, err := pass.NewPassword(length, m.cfg.Generate.Symbols)
	if err != nil {
		m.status, m.statusErr = err.Error(), true
		return m, nil
	}
	entry := pass.Entry{Name: name, Password: password}
	m.status, m.statusErr = "generating "+name+"…", false
	return m, insertCmd(m.store, name, entry.Compose())
}

// ---------------------------------------------------------------------------
// Theme
// ---------------------------------------------------------------------------

func (m Model) applyTheme(name string) (tea.Model, tea.Cmd) {
	theme, ok := config.PresetTheme(name)
	if !ok {
		m.status, m.statusErr = "unknown theme: "+name, true
		return m, nil
	}
	m.cfg.Theme = theme
	m.styles = buildStyles(theme)
	if err := config.SaveThemePreset(m.configPath, theme.Preset); err != nil {
		m.status, m.statusErr = "theme applied but not saved: "+err.Error(), true
		return m, nil
	}
	m.status, m.statusErr = "theme: "+theme.Preset, false
	return m, nil
}

// cycleTheme rotates to the next built-in palette and persists it, so `t` is a
// live preview you can stop on.
func (m Model) cycleTheme() (tea.Model, tea.Cmd) {
	names := config.ThemePresetNames()
	current := strings.ToLower(strings.TrimSpace(m.cfg.Theme.Preset))
	next := names[0]
	for i, name := range names {
		if name == current {
			next = names[(i+1)%len(names)]
			break
		}
	}
	return m.applyTheme(next)
}

// ---------------------------------------------------------------------------
// Detail rows
// ---------------------------------------------------------------------------

// detailRow is one copyable line in the detail view.
type detailRow struct {
	label  string
	value  string
	secret bool
	line   int // raw line the value lives on, or accountRow
}

// detailRows flattens the decrypted entry into the rows the detail view shows
// and the copy keys act on.
func (m Model) detailRows() []detailRow {
	if m.entry == nil {
		return nil
	}
	rows := []detailRow{{label: "password", value: m.entry.Password, secret: true, line: 0}}
	// An account identifier carried by the entry's name has no line of its own
	// to render. Give it a row so what `u` copies is visible on screen, and so
	// the positional copy keys can reach it like any real field. The other two
	// conventions already appear as a field or a note row.
	if value, source, ok := m.entry.UsernameFrom(m.cfg.UsernameFields); ok && source == pass.UsernameName {
		rows = append(rows, detailRow{label: "account", value: value, line: accountRow})
	}

	// Walked in file order rather than fields-then-notes, so a row's position
	// in the entry is the position it is shown in — which is what lets `e` edit
	// the line the cursor is actually on.
	notes := 0
	for _, l := range m.entry.Lines {
		if l.Key != "" {
			// Anything that looks like a second secret stays masked too; a TOTP
			// seed or recovery code is no less sensitive than the password.
			rows = append(rows, detailRow{label: l.Key, value: l.Value, secret: isSecretField(l.Key), line: l.Index})
			continue
		}
		if strings.TrimSpace(l.Value) == "" {
			continue
		}
		// pass-otp stores its seed as a bare otpauth:// URI, which the field
		// parser rightly treats as a note — but it carries the TOTP secret.
		if isOTPURI(l.Value) {
			rows = append(rows, detailRow{label: "otp", value: l.Value, secret: true, line: l.Index})
			continue
		}
		notes++
		rows = append(rows, detailRow{label: fmt.Sprintf("note %d", notes), value: l.Value, line: l.Index})
	}
	return rows
}

// secretFieldKeys are field names whose values are masked alongside the
// password.
var secretFieldKeys = []string{"otp", "totp", "otpauth", "secret", "token", "pin", "recovery", "seed", "key"}

func isOTPURI(line string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "otpauth://")
}

func isSecretField(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, s := range secretFieldKeys {
		if strings.Contains(key, s) {
			return true
		}
	}
	return false
}
