package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"passer/internal/clip"
	"passer/internal/config"
	"passer/internal/pass"
)

type mode int

const (
	modeList mode = iota
	modeSearch
	modeDetail
	modeCommand
	modeHelp
	modeAdd
	modeRename
	modeConfirmDelete
	modeEditField
)

// loadIntent is what to do once an entry finishes decrypting.
//
// Decryption is asynchronous — it can block on a pinentry prompt — so the
// action the user asked for has to travel with the request and be applied when
// the plaintext comes back.
type loadIntent int

const (
	intentOpen loadIntent = iota
	intentCopyPassword
	intentCopyUsername
	intentCopyURL
	intentSetUsername // decrypt so the account prompt can rewrite the entry
	intentEditField   // decrypt, then open the field editor on the entry
	intentRefresh     // re-decrypt an entry already on screen, e.g. after `pass edit`
)

// fieldPrompt backs the one-line editor used for both `e` (edit the field under
// the cursor) and `U` (register the account).
//
// The decrypted entry it rewrites is the one in Model.entry, so that plaintext
// has exactly one owner and the idle lock already covers it. The rest is what
// the prompt needs to write the value back and to put the user where they were.
type fieldPrompt struct {
	name  string // entry being rewritten
	from  mode   // where to return on cancel or save
	label string // what is being edited, shown in the prompt
	// line is the raw line the value lives on. accountRow means there is no
	// such line: the account was derived from the entry's name, so saving has
	// to add a field rather than replace a line.
	line    int
	secret  bool // mask while typing
	reveal  bool
	account bool // write through SetUsername, not SetValueAt
}

// accountRow is the line index of a detail row that has no line in the file.
const accountRow = -1

type uiStyles struct {
	Base        lipgloss.Style
	Title       lipgloss.Style
	Heading     lipgloss.Style
	Accent      lipgloss.Style
	Muted       lipgloss.Style
	Border      lipgloss.Style
	Selection   lipgloss.Style
	Danger      lipgloss.Style
	Warning     lipgloss.Style
	Success     lipgloss.Style
	Status      lipgloss.Style
	StatusAlt   lipgloss.Style
	Panel       lipgloss.Style
	PanelTitle  lipgloss.Style
	TableHeader lipgloss.Style
	KeyCap      lipgloss.Style
	KeyLabel    lipgloss.Style
	Secret      lipgloss.Style // revealed secret text
	Mask        lipgloss.Style // the ••• stand-in for a hidden secret
	MatchHit    lipgloss.Style // fuzzy-matched characters in the list
}

// addState backs the Create Entry modal.
type addState struct {
	inputs   []textinput.Model
	index    int
	expanded bool
	reveal   bool // show the typed password in clear
	validate string
}

// Field indices in the add modal.
const (
	fieldName = iota
	fieldPassword
	fieldUsername
	fieldURL
	fieldNotes
)

// fieldMore is the sentinel index of the expand/collapse toggle row.
const fieldMore = -1

func addCoreFields() []int   { return []int{fieldName, fieldPassword, fieldUsername, fieldURL} }
func addDetailFields() []int { return []int{fieldNotes} }

// order returns the navigable rows in display order, with the expand toggle
// always sitting between the core fields and the optional detail fields.
func (a *addState) order() []int {
	order := append([]int{}, addCoreFields()...)
	order = append(order, fieldMore)
	if a.expanded {
		order = append(order, addDetailFields()...)
	}
	return order
}

func (a *addState) orderPos() int {
	for i, f := range a.order() {
		if f == a.index {
			return i
		}
	}
	return 0
}

func addFieldLabel(idx int) string {
	switch idx {
	case fieldName:
		return "Name"
	case fieldPassword:
		return "Password"
	case fieldUsername:
		return "Username"
	case fieldURL:
		return "URL"
	case fieldNotes:
		return "Notes"
	}
	return ""
}

type Model struct {
	store      *pass.Store
	cfg        config.Config
	configPath string
	clipboard  clip.Backend
	clipErr    error // why the clipboard is unavailable, if it is

	items   []pass.Item
	matches []match
	cursor  int
	scroll  int
	navBuf  string // pending prefix key, for the gg motion
	// pendingSelect names an entry the cursor should land on after the next
	// reload, so adding or renaming leaves you on the entry you just touched.
	pendingSelect string

	mode     mode
	prevMode mode

	input textinput.Model // search, command line, and rename all share it
	query string          // the committed fuzzy filter

	// entry is the currently decrypted entry. It holds plaintext, so it is set
	// only while the detail view is open and cleared on close, lock, or quit.
	entry       *pass.Entry
	entryCursor int
	revealed    bool
	revealLeft  int

	clipLabel string // what was copied, for the status bar
	clipValue string // the exact string we wrote, so auto-clear can verify it
	clipLeft  int

	lockLeft int

	add           *addState
	field         *fieldPrompt
	renameFrom    string
	confirmDelete string

	commandHistory    []string
	commandHistoryIdx int

	status    string
	statusErr bool
	loading   string // entry name currently being decrypted, if any

	styles     uiStyles
	width      int
	height     int
	helpScroll int
	ticking    bool
}

func Run(store *pass.Store, board clip.Backend, clipErr error, cfg config.Config, configPath string) error {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 512
	ti.Width = 40

	m := Model{
		store:             store,
		cfg:               cfg,
		configPath:        configPath,
		clipboard:         board,
		clipErr:           clipErr,
		mode:              modeList,
		input:             ti,
		commandHistoryIdx: -1,
		styles:            buildStyles(cfg.Theme),
		lockLeft:          cfg.Security.LockSeconds,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if clearErr := clearOnExit(final, board.ClearIfUnchanged); clearErr != nil && err == nil {
		err = fmt.Errorf("clearing clipboard on exit: %w", clearErr)
	}
	return err
}

// clearOnExit wipes a copied secret whose auto-clear timer was still running
// when passer quit. The timer lives in this process, so without this, copying a
// password and quitting — the most common way to use a copy — would leave it on
// the clipboard indefinitely.
func clearOnExit(final tea.Model, clear func(expect string) error) error {
	m, ok := final.(Model)
	if !ok || m.clipValue == "" {
		return nil
	}
	return clear(m.clipValue)
}

func (m Model) Init() tea.Cmd { return loadItemsCmd(m.store) }

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type itemsLoadedMsg struct {
	items []pass.Item
	err   error
}

type entryLoadedMsg struct {
	name   string
	entry  pass.Entry
	intent loadIntent
	err    error
}

type copiedMsg struct {
	label string
	value string
	err   error
}

type clipClearedMsg struct{ err error }

type writeDoneMsg struct {
	name   string
	action string
	err    error
}

type editDoneMsg struct {
	name string
	err  error
}

type configEditedMsg struct{ err error }

type tickMsg time.Time

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func loadItemsCmd(store *pass.Store) tea.Cmd {
	return func() tea.Msg {
		items, err := store.List()
		return itemsLoadedMsg{items: items, err: err}
	}
}

func loadEntryCmd(store *pass.Store, name string, intent loadIntent) tea.Cmd {
	return func() tea.Msg {
		entry, err := store.Show(name)
		return entryLoadedMsg{name: name, entry: entry, intent: intent, err: err}
	}
}

func copyCmd(board clip.Backend, label, value string) tea.Cmd {
	return func() tea.Msg {
		return copiedMsg{label: label, value: value, err: board.Copy(value)}
	}
}

func clearClipCmd(board clip.Backend, expect string) tea.Cmd {
	return func() tea.Msg {
		return clipClearedMsg{err: board.ClearIfUnchanged(expect)}
	}
}

func insertCmd(store *pass.Store, name, content string) tea.Cmd {
	return func() tea.Msg {
		return writeDoneMsg{name: name, action: "added", err: store.Insert(name, content, false)}
	}
}

// actionUpdated marks an in-place field write, so the result can refresh an
// entry that is still on screen.
const actionUpdated = "updated"

// updateCmd rewrites an entry whose content has had one value changed.
//
// This is the one place passer overwrites an existing entry. That is safe only
// because content is the entry's own decrypted text with a single line changed
// — see Entry.SetValueAt and Entry.SetUsername — rather than something composed
// from scratch.
func updateCmd(store *pass.Store, name, content string) tea.Cmd {
	return func() tea.Msg {
		return writeDoneMsg{name: name, action: actionUpdated, err: store.Insert(name, content, true)}
	}
}

func removeCmd(store *pass.Store, name string) tea.Cmd {
	return func() tea.Msg {
		return writeDoneMsg{name: name, action: "deleted", err: store.Remove(name)}
	}
}

func moveCmd(store *pass.Store, from, to string) tea.Cmd {
	return func() tea.Msg {
		return writeDoneMsg{name: to, action: "renamed", err: store.Move(from, to)}
	}
}

// editCmd suspends the TUI so `pass edit` can hand the terminal to $EDITOR.
func editCmd(store *pass.Store, name string) tea.Cmd {
	cmd, finish, err := store.EditCmd(name)
	if err != nil {
		return func() tea.Msg { return editDoneMsg{name: name, err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editDoneMsg{name: name, err: finish(err)}
	})
}

// editConfigCmd opens the config in $EDITOR, mirroring bada's :config.
func editConfigCmd(path string) tea.Cmd {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if editor == "" {
		editor = "vi"
	}
	// Split so EDITOR="code -w" and friends keep working.
	parts := strings.Fields(editor)
	parts = append(parts, path)
	cmd := exec.Command(parts[0], parts[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return configEditedMsg{err: err}
	})
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncScroll()
		return m, nil

	case itemsLoadedMsg:
		if msg.err != nil {
			m.status, m.statusErr = msg.err.Error(), true
			return m, nil
		}
		m.items = msg.items
		m.applyFilter()
		return m, nil

	case entryLoadedMsg:
		return m.onEntryLoaded(msg)

	case copiedMsg:
		return m.onCopied(msg)

	case clipClearedMsg:
		m.clipLabel, m.clipValue, m.clipLeft = "", "", 0
		if msg.err != nil {
			m.status, m.statusErr = "clipboard clear failed: "+msg.err.Error(), true
		} else {
			m.status, m.statusErr = "clipboard cleared", false
		}
		return m, nil

	case writeDoneMsg:
		return m.onWriteDone(msg)

	case editDoneMsg:
		if errors.Is(msg.err, pass.ErrUnchanged) {
			m.status, m.statusErr = msg.name+" unchanged", false
			return m, nil
		}
		if msg.err != nil {
			m.status, m.statusErr = "edit failed: "+msg.err.Error(), true
			return m, nil
		}
		m.status, m.statusErr = msg.name+" saved", false
		// The file changed underneath us: refresh the listing for the new
		// mtime, and re-decrypt if the entry is still on screen.
		cmds := []tea.Cmd{loadItemsCmd(m.store)}
		if m.mode == modeDetail && m.entry != nil && m.entry.Name == msg.name {
			cmds = append(cmds, loadEntryCmd(m.store, msg.name, intentRefresh))
		}
		return m, tea.Batch(cmds...)

	case configEditedMsg:
		return m.onConfigEdited(msg)

	case tickMsg:
		return m.onTick()

	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onEntryLoaded(msg entryLoadedMsg) (tea.Model, tea.Cmd) {
	m.loading = ""
	if msg.err != nil {
		m.status, m.statusErr = msg.err.Error(), true
		return m, nil
	}

	// Decryption can sit on a pinentry prompt for a long time. If the user has
	// moved on meanwhile — opened a dialog, or closed the entry being refreshed
	// — the result must not yank them back into the detail view.
	switch msg.intent {
	case intentOpen, intentSetUsername, intentEditField:
		// A decrypt that sat on pinentry must not open a view or a prompt over
		// whatever the user moved on to.
		if m.mode != modeList {
			return m, nil
		}
	case intentRefresh:
		if m.mode != modeDetail || m.entry == nil || m.entry.Name != msg.name {
			return m, nil
		}
	}

	switch msg.intent {
	case intentOpen, intentRefresh:
		entry := msg.entry
		m.entry = &entry
		m.entryCursor = 0
		m.revealed = false
		m.revealLeft = 0
		m.mode = modeDetail
		m.lockLeft = m.cfg.Security.LockSeconds
		if msg.intent == intentOpen {
			m.status, m.statusErr = "", false
		}
		return m, m.ensureTicking()

	case intentCopyPassword:
		return m.startCopy("password", msg.entry.Password)

	case intentCopyUsername:
		return m.copyUsername(msg.entry)

	case intentSetUsername:
		return m.openSetAccount(msg.entry, modeList)

	case intentCopyURL:
		value, ok := msg.entry.URL()
		if !ok {
			m.status, m.statusErr = msg.name+" has no url field", true
			return m, nil
		}
		return m.startCopy("url", value)
	}
	return m, nil
}

// copyUsername copies an entry's account identifier.
//
// The label names the convention that matched, so a value passer inferred from
// a bare line or from the entry's own name is never mistaken for one the user
// labelled themselves. A miss lists the field names that would work rather than
// just reporting the absence, since adding one is the fix.
func (m Model) copyUsername(entry pass.Entry) (tea.Model, tea.Cmd) {
	value, source, ok := entry.UsernameFrom(m.cfg.UsernameFields)
	if !ok {
		keys := strings.Join(pass.UsernameKeys(m.cfg.UsernameFields), ", ")
		m.status, m.statusErr = "no account in "+entry.Name+"; add a field: "+keys, true
		return m, nil
	}
	return m.startCopy(usernameLabel(source), value)
}

// usernameLabel is what the status bar calls the copied value.
func usernameLabel(source pass.UsernameSource) string {
	switch source {
	case pass.UsernameNote:
		return "username (from note)"
	case pass.UsernameName:
		return "username (from name)"
	}
	return "username"
}

// startCopy puts a value on the clipboard and arms the auto-clear countdown.
func (m Model) startCopy(label, value string) (tea.Model, tea.Cmd) {
	if !m.clipboard.Available() {
		msg := "no clipboard tool available"
		if m.clipErr != nil {
			msg = m.clipErr.Error()
		}
		m.status, m.statusErr = msg, true
		return m, nil
	}
	if value == "" {
		m.status, m.statusErr = "nothing to copy", true
		return m, nil
	}
	return m, copyCmd(m.clipboard, label, value)
}

func (m Model) onCopied(msg copiedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status, m.statusErr = "copy failed: "+msg.err.Error(), true
		return m, nil
	}
	m.clipLabel = msg.label
	m.clipValue = msg.value
	m.clipLeft = m.cfg.Security.ClipClearSeconds
	if m.clipLeft <= 0 {
		// Auto-clear disabled: say so plainly rather than implying a timer that
		// will never fire.
		m.clipLabel, m.clipValue = "", ""
		m.status, m.statusErr = msg.label+" copied (auto-clear disabled)", false
		return m, nil
	}
	m.status, m.statusErr = "", false
	return m, m.ensureTicking()
}

func (m Model) onWriteDone(msg writeDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status, m.statusErr = msg.err.Error(), true
		return m, nil
	}
	m.status, m.statusErr = fmt.Sprintf("%s %s", msg.name, msg.action), false
	m.pendingSelect = msg.name
	if msg.action == actionUpdated && m.entry != nil && m.entry.Name == msg.name {
		// The entry is still open, and its file just changed underneath it:
		// re-decrypt so the detail view shows the field that was registered
		// rather than the stale rows it was built from.
		return m, tea.Batch(loadItemsCmd(m.store), loadEntryCmd(m.store, msg.name, intentRefresh))
	}
	if msg.action == "deleted" {
		m.pendingSelect = ""
		// The deleted entry may be the one open in the detail view.
		if m.entry != nil && m.entry.Name == msg.name {
			m.entry = nil
			m.mode = modeList
		}
	}
	return m, loadItemsCmd(m.store)
}

func (m Model) onConfigEdited(msg configEditedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status, m.statusErr = "config edit failed: "+msg.err.Error(), true
		return m, nil
	}
	cfg, err := config.LoadOrCreate(m.configPath)
	if err != nil {
		m.status, m.statusErr = "config reload failed: "+err.Error(), true
		return m, nil
	}
	m.cfg = cfg
	m.styles = buildStyles(cfg.Theme)
	m.status, m.statusErr = "config reloaded", false
	return m, nil
}

// onTick advances every active countdown. One ticker drives all of them; it
// stops as soon as nothing is counting down, so an idle passer is idle.
func (m Model) onTick() (tea.Model, tea.Cmd) {
	m.ticking = false
	var cmds []tea.Cmd

	if m.clipLeft > 0 {
		m.clipLeft--
		if m.clipLeft == 0 {
			cmds = append(cmds, clearClipCmd(m.clipboard, m.clipValue))
		}
	}

	if m.revealed && m.revealLeft > 0 {
		m.revealLeft--
		if m.revealLeft == 0 {
			m.revealed = false
			m.status, m.statusErr = "hidden", false
		}
	}

	// The idle lock only makes sense while plaintext is held.
	if m.entry != nil && m.cfg.Security.LockSeconds > 0 {
		if m.lockLeft > 0 {
			m.lockLeft--
		}
		if m.lockLeft == 0 {
			m.entry = nil
			m.revealed = false
			m.revealLeft = 0
			switch m.mode {
			case modeDetail:
				m.mode = modeList
			case modeEditField:
				// The prompt rewrites m.entry, which has just been dropped, so
				// it cannot be submitted any more; leaving it on screen would
				// only invite typing into a dead field.
				m.field = nil
				m.input.Blur()
				m.input.SetValue("")
				m.mode = modeList
			}
			m.status, m.statusErr = "locked after inactivity", false
		}
	}

	cmds = append(cmds, m.ensureTicking())
	return m, tea.Batch(cmds...)
}

// ensureTicking starts the shared one-second ticker if any countdown is live,
// and returns nil otherwise so the ticker dies out on its own.
func (m *Model) ensureTicking() tea.Cmd {
	if m.ticking {
		return nil
	}
	countdown := m.clipLeft > 0 ||
		(m.revealed && m.revealLeft > 0) ||
		(m.entry != nil && m.cfg.Security.LockSeconds > 0 && m.lockLeft > 0)
	if !countdown {
		return nil
	}
	m.ticking = true
	return tickCmd()
}

// ---------------------------------------------------------------------------
// List helpers
// ---------------------------------------------------------------------------

// applyFilter recomputes the visible list, keeping the cursor on the same entry
// where it can.
func (m *Model) applyFilter() {
	names := make([]string, len(m.items))
	for i, it := range m.items {
		names[i] = it.Name
	}
	previous := m.currentName()
	m.matches = filterItems(names, m.query)

	target := previous
	if m.pendingSelect != "" {
		target = m.pendingSelect
		m.pendingSelect = ""
	}
	m.cursor = 0
	if target != "" {
		for i, mt := range m.matches {
			if m.items[mt.index].Name == target {
				m.cursor = i
				break
			}
		}
	}
	m.syncScroll()
}

// currentItem resolves the cursor to an entry.
//
// It bounds-checks against items as well as matches: when the store shrinks —
// a `pass rm` in another terminal, then a reload — the retained matches still
// hold indices into the longer slice they were built from, and following one
// blindly panics.
func (m Model) currentItem() (pass.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return pass.Item{}, false
	}
	idx := m.matches[m.cursor].index
	if idx < 0 || idx >= len(m.items) {
		return pass.Item{}, false
	}
	return m.items[idx], true
}

func (m Model) currentName() string {
	item, ok := m.currentItem()
	if !ok {
		return ""
	}
	return item.Name
}

// screenLayout is how the rows between the top of the screen and the status
// bar are shared out. The view and the scroll logic both read it, so what is
// drawn and what the cursor thinks is visible can't disagree.
type screenLayout struct {
	listRows   int // entry rows inside the list panel
	detailRows int // field rows inside the detail panel; 0 when it is closed
}

// minListRows keeps a sliver of the list on screen while an entry is open, so
// you never lose your place in the store.
const minListRows = 3

func (m Model) layout() screenLayout {
	height := m.height
	if height <= 0 {
		height = 30
	}
	// Status bar, then the list panel's top edge, column header, and bottom edge.
	avail := height - 1 - m.footerLineCount() - 3

	var l screenLayout
	if m.detailOpen() {
		n := len(m.detailRows())
		if most := avail - 2 - minListRows; n > most {
			n = most
		}
		if n < 1 {
			n = 1
		}
		l.detailRows = n
		avail -= n + 2 // the detail panel's own top and bottom edges
	}
	if avail < 1 {
		avail = 1
	}
	l.listRows = avail
	return l
}

// listRows is how many entry rows fit inside the list panel.
func (m Model) listRows() int { return m.layout().listRows }

// detailOpen reports whether the decrypted entry is on screen, including while
// a : command typed from the detail view is being entered.
func (m Model) detailOpen() bool {
	if m.entry == nil {
		return false
	}
	switch m.mode {
	case modeDetail, modeEditField:
		return true
	case modeCommand:
		return m.prevMode == modeDetail
	}
	return false
}

// promptActive reports whether a one-line prompt sits under the list.
func (m Model) promptActive() bool {
	switch m.mode {
	case modeSearch, modeCommand, modeRename, modeConfirmDelete, modeEditField:
		return true
	}
	return m.query != ""
}

// footerLineCount is the number of lines between the panels and the status
// bar: the hint bar, plus a prompt line when one is showing.
func (m Model) footerLineCount() int {
	if m.promptActive() {
		return 2
	}
	return 1
}

func (m *Model) syncScroll() {
	rows := m.listRows()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+rows {
		m.scroll = m.cursor - rows + 1
	}
	if m.scroll > len(m.matches)-rows {
		m.scroll = len(m.matches) - rows
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m *Model) moveCursor(delta int) {
	if len(m.matches) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.matches) {
		m.cursor = len(m.matches) - 1
	}
	m.syncScroll()
}
