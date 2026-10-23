package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	switch m.mode {
	case modeHelp:
		return m.fillView(m.renderHelpView())
	case modeAdd:
		return m.fillView(m.renderAddModal())
	default:
		return m.fillView(m.renderMainView())
	}
}

// fillView pads body to the terminal height and pins the status bar to the
// last row, so the bar never floats mid-screen on a short list.
func (m Model) fillView(body string) string {
	status := m.renderStatusBar()
	if m.height <= 0 {
		return m.paintCanvas(body + "\n" + status)
	}

	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	target := m.height - 1 // rows above the status bar
	if len(lines) > target {
		lines = lines[:target]
	}
	for len(lines) < target {
		lines = append(lines, "")
	}
	return m.paintCanvas(strings.Join(lines, "\n") + "\n" + status)
}

// paintCanvas also paints empty rows, preserving the terminal's anti-wrap gutter.
func (m Model) paintCanvas(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if m.width > 2 {
			line = padTo(line, m.width-2)
		}
		lines[i] = m.styles.Base.Render(line)
	}
	return strings.Join(lines, "\n")
}

// renderMainView stacks the list, the open entry (if any), the prompt line, and
// the hint bar. The list stays on screen while an entry is open, the way bada
// keeps its task table above the detail pane, so you never lose your place.
func (m Model) renderMainView() string {
	l := m.layout()
	out := m.renderListPanel(l.listRows)
	if l.detailRows > 0 {
		out += "\n" + m.renderDetailPanel(l.detailRows)
	}
	if line := m.promptLine(); line != "" {
		out += "\n" + line
	}
	if m.detailOpen() {
		out += "\n" + m.hintBar(m.detailHints())
	} else {
		out += "\n" + m.hintBar(m.listHints())
	}
	return out
}

// promptLine is the single line under the panels: whichever prompt is active,
// else the committed search filter. Must agree with promptActive.
func (m Model) promptLine() string {
	switch m.mode {
	case modeConfirmDelete:
		return m.renderConfirmLine()
	case modeRename:
		return m.renderRenameLine()
	case modeCommand:
		return m.renderCommandLine()
	case modeEditField:
		return m.renderFieldPromptLine()
	}
	if m.mode == modeSearch || m.query != "" {
		return m.renderSearchLine()
	}
	return ""
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// listColumns are the list's column widths. They are sized from the whole
// store, not the visible slice, so columns hold still while you scroll or
// filter.
type listColumns struct {
	folder, entry, updated int
}

const (
	listLead     = 2 // cursor marker
	updatedWidth = 10
)

func (m Model) listColumns() listColumns {
	inner := m.panelInnerWidth()
	cols := listColumns{folder: len("Folder"), entry: 12, updated: updatedWidth}
	for _, it := range m.items {
		folder, entry := splitName(it.Name)
		if w := lipgloss.Width(folder); w > cols.folder {
			cols.folder = w
		}
		if w := lipgloss.Width(entry); w > cols.entry {
			cols.entry = w
		}
	}
	// Caps keep the Updated column next to the names instead of stranded at
	// the far edge, where the eye has to travel a whole row to reach it.
	if cols.folder > 24 {
		cols.folder = 24
	}
	if cols.entry > 48 {
		cols.entry = 48
	}
	// Then fit the terminal, giving up entry width first and folder second.
	over := listLead + cols.folder + 1 + cols.entry + 1 + cols.updated - inner
	for _, col := range []*int{&cols.entry, &cols.folder} {
		if over <= 0 {
			break
		}
		room := *col - 6
		if room > over {
			room = over
		}
		if room > 0 {
			*col -= room
			over -= room
		}
	}
	return cols
}

// splitName separates an entry name into its folder and leaf. Root entries
// have no folder.
func splitName(name string) (folder, entry string) {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		return name[:idx], name[idx+1:]
	}
	return "", name
}

func (m Model) renderListPanel(rows int) string {
	inner := m.panelInnerWidth()
	cols := m.listColumns()

	header := strings.Repeat(" ", listLead) +
		padTo("Folder", cols.folder) + " " +
		padTo("Entry", cols.entry) + " " +
		padTo("Updated", cols.updated)
	lines := []string{m.styles.TableHeader.Render(padTo(header, inner))}

	if len(m.matches) == 0 {
		lines = append(lines, m.emptyListMessage())
	} else {
		start := m.visibleStart(rows)
		end := start + rows
		if end > len(m.matches) {
			end = len(m.matches)
		}
		prevFolder := ""
		for i := start; i < end; i++ {
			line, folder := m.renderListRow(i, cols, inner, i > start, prevFolder)
			lines = append(lines, line)
			prevFolder = folder
		}
	}
	for len(lines) < rows+1 {
		lines = append(lines, "")
	}

	return m.panel("passer • "+shortenPath(m.storeDir()), strings.Join(lines, "\n"))
}

// visibleStart is the first match drawn. It starts from the stored scroll
// offset but always brings the cursor into view, so a layout change — an
// entry opening underneath the list — can't strand the cursor off screen.
func (m Model) visibleStart(rows int) int {
	start := m.scroll
	if m.cursor < start {
		start = m.cursor
	}
	if m.cursor >= start+rows {
		start = m.cursor - rows + 1
	}
	if start > len(m.matches)-rows {
		start = len(m.matches) - rows
	}
	if start < 0 {
		start = 0
	}
	return start
}

// renderListRow draws one entry and returns its folder, so the next row can
// tell whether it continues the same group.
func (m Model) renderListRow(i int, cols listColumns, inner int, continues bool, prevFolder string) (string, string) {
	mt := m.matches[i]
	if mt.index < 0 || mt.index >= len(m.items) {
		// Matches built against a longer item slice; see currentItem.
		return strings.Repeat(" ", inner), ""
	}
	item := m.items[mt.index]
	selected := i == m.cursor
	folder, entry := splitName(item.Name)

	hit := make(map[int]bool, len(mt.positions))
	for _, p := range mt.positions {
		hit[p] = true
	}
	folderRunes := len([]rune(folder))
	entryOffset := 0
	if folder != "" {
		entryOffset = folderRunes + 1
	}

	// Name each folder once per run of entries, so groups read like a tree.
	// Search results are ranked rather than grouped, and their folder text may
	// carry match highlights, so there every row names its folder.
	var folderCell string
	switch {
	case folder == "":
		folderCell = "–"
		if !selected {
			folderCell = m.styles.Muted.Render(folderCell)
		}
	case continues && folder == prevFolder && m.query == "" && !selected:
		folderCell = ""
	default:
		base := m.styles.Heading.UnsetBold()
		if selected {
			base = lipgloss.NewStyle()
		}
		folderCell = m.highlight(folder, 0, hit, cols.folder, base, selected)
	}

	entryCell := m.highlight(entry, entryOffset, hit, cols.entry, lipgloss.NewStyle(), selected)
	updated := humanTime(item.Modified)
	if !selected {
		updated = m.styles.Muted.Render(updated)
	}

	lead := "  "
	if selected {
		lead = "▸ "
		if m.loading == item.Name {
			lead = "◌ "
		}
	}

	line := lead + padTo(folderCell, cols.folder) + " " + padTo(entryCell, cols.entry) + " " + padTo(updated, cols.updated)
	line = padTo(line, inner)
	if selected {
		line = stripeLine(m.styles.Selection, line)
	}
	return line, folder
}

// highlight renders text in base, with fuzzy-matched runes picked out.
// positions index the full entry name; offset is where text starts within it.
// Over-long text keeps its head and ends in an ellipsis.
func (m Model) highlight(text string, offset int, hit map[int]bool, width int, base lipgloss.Style, selected bool) string {
	runes := []rune(text)
	if len(runes) > width {
		runes = append(runes[:width-1], '…')
	}
	var b strings.Builder
	for i, r := range runes {
		if hit[offset+i] && r != '…' {
			hitStyle := m.styles.MatchHit
			if selected {
				hitStyle = lipgloss.NewStyle().Bold(true).Underline(true)
			}
			b.WriteString(hitStyle.Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}

func (m Model) emptyListMessage() string {
	if len(m.items) == 0 {
		return "  " + m.styles.Muted.Render("store is empty — press a to add your first entry")
	}
	return "  " + m.styles.Muted.Render("no entry matches "+strconv.Quote(m.query))
}

func (m Model) renderSearchLine() string {
	label := m.styles.Accent.Render(" / ")
	if m.mode == modeSearch {
		return label + m.input.View()
	}
	return label + m.styles.Muted.Render(m.query+"  (esc to clear)")
}

func (m Model) renderCommandLine() string {
	line := m.styles.Accent.Render(" : ") + m.input.View()
	return line + m.completionMenu(lipgloss.Width(line))
}

// completionMenu lists the candidates for what is being typed, with the one
// currently in the input picked out.
//
// It shares the single prompt line with the input, so when the names don't fit
// in the room left over, whole ones are dropped from the end — the same rule
// the hint bar follows, and the active candidate is readable in the input
// regardless.
func (m Model) completionMenu(used int) string {
	hits, active := m.commandCandidates()
	if len(hits) == 0 {
		return ""
	}
	const gap = 2
	room := m.panelInnerWidth() - used - gap
	if room < 4 {
		return ""
	}

	var parts []string
	width := 0
	for i, name := range hits {
		w := len(name) + 1 // command and palette names are ASCII; plus a space
		if width+w > room {
			parts = append(parts, m.styles.Muted.Render("…"))
			break
		}
		width += w
		style := m.styles.Muted
		if i == active {
			style = m.styles.MatchHit
		}
		parts = append(parts, style.Render(name))
	}
	return strings.Repeat(" ", gap) + strings.Join(parts, " ")
}

// commandCandidates are the names to list beside the prompt, and which of them
// is in the input (-1 for none).
//
// A Tab cycle supplies its own. Without one, a command whose argument comes
// from a fixed set still lists it, so typing ":theme" shows the palettes
// without having to press anything or run the command to find out.
func (m Model) commandCandidates() ([]string, int) {
	if c := m.completion; c != nil {
		return c.hits, c.index
	}
	hits, keep := commandArgCandidates(m.input.Value())
	// Once the argument is typed out in full, repeating it beside the input
	// says nothing the input doesn't.
	if len(hits) == 1 && strings.TrimPrefix(m.input.Value(), keep) == hits[0] {
		return nil, -1
	}
	return hits, -1
}

func (m Model) renderRenameLine() string {
	return m.styles.Accent.Render(" rename → ") + m.input.View()
}

// renderFieldPromptLine draws the one-line field editor, naming the value being
// edited and only the extra keys that actually apply to it.
func (m Model) renderFieldPromptLine() string {
	label := "value"
	hint := "enter saves, esc cancels"
	if f := m.field; f != nil {
		label = f.label
		switch {
		case f.line == 0:
			hint = "enter saves, ctrl+g generates, ctrl+r reveals, esc cancels"
		case f.secret:
			hint = "enter saves, ctrl+r reveals, esc cancels"
		}
	}
	return m.styles.Accent.Render(" "+label+" → ") + m.input.View() +
		m.styles.Muted.Render("  ("+hint+")")
}

func (m Model) renderConfirmLine() string {
	warn := m.styles.Danger.Render(" delete " + m.confirmDelete + "? ")
	return warn + m.styles.Muted.Render("y to confirm, any other key to cancel")
}

func (m Model) listHints() []keyHint {
	k := m.cfg.Keys
	return []keyHint{
		{k.Open, "open"},
		{k.CopyPassword, "copy pw"},
		{k.CopyUsername, "user"},
		{k.SetUsername, "set user"},
		{k.Search, "search"},
		{k.Add, "add"},
		{k.Edit, "edit"},
		{k.Delete, "delete"},
		{k.Help, "help"},
		{k.Quit, "quit"},
	}
}

// ---------------------------------------------------------------------------
// Detail
// ---------------------------------------------------------------------------

// renderDetailPanel draws the open entry in at most maxRows field rows,
// scrolling to keep the field cursor visible.
func (m Model) renderDetailPanel(maxRows int) string {
	inner := m.panelInnerWidth()
	rows := m.detailRows()

	labelWidth := 10
	for _, r := range rows {
		if len(r.label) > labelWidth {
			labelWidth = len(r.label)
		}
	}
	if labelWidth > 20 {
		labelWidth = 20
	}
	valueWidth := inner - labelWidth - 8
	if valueWidth < 10 {
		valueWidth = 10
	}

	start := 0
	if m.entryCursor >= maxRows {
		start = m.entryCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(rows) {
		end = len(rows)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		r := rows[i]
		num := m.styles.Muted.Render(fmt.Sprintf(" %2d ", i+1))
		label := m.styles.Heading.Render(padTo(r.label, labelWidth))

		value := r.value
		style := lipgloss.NewStyle()
		if r.secret && !m.revealed {
			value = maskOf(value)
			style = m.styles.Mask
		} else if r.secret {
			style = m.styles.Secret
		}
		if i == m.entryCursor {
			num = fmt.Sprintf(" %2d ", i+1)
			label = padTo(r.label, labelWidth)
			style = lipgloss.NewStyle()
			if r.secret && m.revealed {
				style = style.Underline(true)
			}
		}
		value = style.Render(truncateANSI(value, valueWidth))

		line := padTo(num+label+"  "+value, inner)
		if i == m.entryCursor {
			line = stripeLine(m.styles.Selection, line)
		}
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	title := m.entry.Name
	if m.revealed {
		title += " • REVEALED"
	}
	return m.panel(title, b.String())
}

// maskOf renders a secret as dots without leaking its exact length, which is
// itself a small hint about the password.
func maskOf(value string) string {
	if value == "" {
		return ""
	}
	n := len([]rune(value))
	if n > 12 {
		n = 12
	}
	return strings.Repeat("•", n)
}

func (m Model) detailHints() []keyHint {
	k := m.cfg.Keys
	return []keyHint{
		{k.Reveal, "reveal"},
		{k.CopyPassword, "copy pw"},
		{k.CopyUsername, "user"},
		{k.CopyURL, "url"},
		{"1-9", "copy field"},
		{k.Edit, "edit field"},
		{k.SetUsername, "account"},
		{k.EditFile, "$EDITOR"},
		{k.Quit, "back"},
	}
}

// ---------------------------------------------------------------------------
// Add modal
// ---------------------------------------------------------------------------

func (m Model) renderAddModal() string {
	a := m.add
	if a == nil {
		return m.renderMainView()
	}

	width := m.panelInnerWidth()
	if width > 64 {
		width = 64
	}
	labelWidth := 10

	var b strings.Builder
	for _, idx := range a.order() {
		if idx == fieldMore {
			arrow := "▸"
			if a.expanded {
				arrow = "▾"
			}
			row := "  " + m.styles.Muted.Render(arrow+" More details")
			if a.index == fieldMore {
				row = stripeLine(m.styles.Selection, padTo("  "+arrow+" More details", width))
			}
			b.WriteString(padTo(row, width))
			b.WriteString("\n")
			continue
		}

		marker := "  "
		if a.index == idx {
			marker = m.styles.Accent.Render("▸ ")
		}
		label := m.styles.Heading.Render(padTo(addFieldLabel(idx), labelWidth))
		b.WriteString(padTo(marker+label+" "+a.inputs[idx].View(), width))
		b.WriteString("\n")
	}

	b.WriteString(padTo("", width))
	b.WriteString("\n")
	if a.validate != "" {
		b.WriteString(padTo("  "+m.styles.Danger.Render(a.validate), width))
	} else {
		b.WriteString(padTo("  "+m.styles.Muted.Render(fmt.Sprintf(
			"blank password generates %d chars", m.cfg.Generate.Length)), width))
	}

	modal := m.panel("New Entry", b.String())
	hints := m.hintBar([]keyHint{
		{"ctrl+g", "generate"},
		{"ctrl+r", "reveal"},
		{"ctrl+s", "save"},
		{"tab", "next field"},
		{"esc", "cancel"},
	})

	block := modal + "\n" + hints
	// Centre the dialog the way bada centres its metadata modal.
	if m.height > 0 {
		pad := (m.height - 1 - countLines(block)) / 2
		if pad > 0 {
			block = strings.Repeat("\n", pad) + block
		}
	}
	return block
}

// ---------------------------------------------------------------------------
// Help
// ---------------------------------------------------------------------------

// helpLines builds the full keybinding reference as rendered lines. Both the
// view and the scroll handler use it, so the clamp and the display can't
// disagree about how far the reference actually goes.
func (m Model) helpLines() []string {
	k := m.cfg.Keys
	type section struct {
		title string
		rows  [][2]string
	}
	sections := []section{
		{"Navigate", [][2]string{
			{k.Down + " / " + k.Up, "move down / up"},
			{"gg / G", "jump to top / bottom"},
			{"ctrl+d / ctrl+u", "half page down / up"},
			{k.Open, "open the entry (decrypts)"},
			{k.Quit, "back, or quit from the unfiltered list"},
		}},
		{"Find", [][2]string{
			{k.Search, "fuzzy filter the store"},
			{"enter", "open the best match"},
			{"esc", "leave the prompt, keep the filter"},
			{k.Cancel + " (again)", "clear the filter"},
		}},
		{"Copy", [][2]string{
			{k.CopyPassword, "copy the password"},
			{k.CopyUsername, "copy the username or email"},
			{k.CopyURL, "copy the url field"},
			{"1-9", "copy the nth field (detail view)"},
			{"c", "copy the field under the cursor"},
		}},
		{"Entry", [][2]string{
			{k.Reveal, "reveal / hide the password (detail view)"},
			{k.SetUsername, "register the username / email"},
			{k.Add, "add an entry"},
			{k.Edit, "edit the focused field (open entry)"},
			{k.EditFile, "edit the whole file in $EDITOR"},
			{k.Delete, "delete, with confirmation"},
			{k.Rename, "rename / move"},
			{k.Reload, "reload the store"},
		}},
		{"Commands", [][2]string{
			{":add [name]", "open the new-entry dialog"},
			{":generate <name> [len]", "add a password-only entry"},
			{":account", "register the account on an entry"},
			{":rename <old> <new>", "move an entry"},
			{":delete [name]", "delete an entry"},
			{":theme [name]", "list or set a palette"},
			{":config", "edit the config in $EDITOR"},
			{":reload", "re-read the store"},
			{":quit", "exit"},
		}},
	}

	var lines []string
	for i, s := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, " "+m.styles.Title.Render(s.title))
		for _, r := range s.rows {
			key := m.styles.KeyCap.Render(" " + displayKey(r[0]) + " ")
			lines = append(lines, "   "+padTo(key, 26)+m.styles.KeyLabel.Render(r[1]))
		}
	}
	return lines
}

// helpVisibleRows is how many reference lines fit inside the panel.
func (m Model) helpVisibleRows() int {
	visible := m.height - 4 // panel borders, hint bar, status bar
	if visible < 1 {
		visible = 1
	}
	return visible
}

// helpMaxScroll is the furthest scroll offset that still fills the panel.
func (m Model) helpMaxScroll() int {
	max := len(m.helpLines()) - m.helpVisibleRows()
	if max < 0 {
		return 0
	}
	return max
}

func (m Model) renderHelpView() string {
	// Scroll rather than truncate: the reference is taller than most terminals.
	lines := m.helpLines()
	visible := m.helpVisibleRows()

	start := m.helpScroll
	if start > m.helpMaxScroll() {
		start = m.helpMaxScroll()
	}
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(lines) {
		end = len(lines)
	}

	body := strings.Join(lines[start:end], "\n")
	return m.panel("Keys", body) + "\n" + m.hintBar([]keyHint{
		{"j/k", "scroll"},
		{"q", "back"},
	})
}

// ---------------------------------------------------------------------------
// Status bar
// ---------------------------------------------------------------------------

// renderStatusBar mirrors bada's: an app badge and a mode badge, then the
// position and any filter, then the latest message. The live countdowns are
// pinned to the right; they are the only way to know how long a secret stays
// exposed, so they never move or get pushed off by a long message.
func (m Model) renderStatusBar() string {
	width := m.width - 2 // the same anti-wrap gutter the panels leave
	if width < 10 {
		width = 10
	}
	th := m.cfg.Theme
	base := m.styles.Status

	chip := func(s, bg string) string {
		st := base.Bold(true)
		if strings.TrimSpace(bg) != "" {
			st = st.Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(textOn(bg)))
		}
		return st.Render(" " + s + " ")
	}
	seg := func(s, fg string) string {
		st := base
		if strings.TrimSpace(fg) != "" {
			st = st.Foreground(lipgloss.Color(fg))
		}
		return st.Render(s)
	}
	sp := base.Render("  ")

	left := chip("passer", th.Accent) + base.Render(" ") + chip(m.modeLabel(), th.StatusAltBg)
	if m.detailOpen() {
		left += sp + seg(m.entry.Name, th.Heading)
	} else {
		cur := 0
		if len(m.matches) > 0 {
			cur = m.cursor + 1
		}
		left += sp + seg(fmt.Sprintf("%d/%d", cur, len(m.matches)), th.Success)
		if m.query != "" {
			left += sp + seg("filter:", th.Muted) + seg(strconv.Quote(m.query), th.Warning)
		}
	}
	switch {
	case m.loading != "":
		left += sp + seg("decrypting "+m.loading+"…", th.Warning)
	case m.status != "" && m.statusErr:
		left += sp + seg(m.status, th.Danger)
	case m.status != "":
		left += sp + seg(m.status, "")
	}

	right := m.statusRight(seg, sp)
	room := width - lipgloss.Width(right)
	if lipgloss.Width(left) > room {
		left = truncateANSI(left, room)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return stripeLine(base, left+base.Render(strings.Repeat(" ", gap))+right)
}

func (m Model) modeLabel() string {
	switch m.mode {
	case modeSearch:
		return "SEARCH"
	case modeCommand:
		return "COMMAND"
	case modeRename:
		return "RENAME"
	case modeConfirmDelete:
		return "DELETE"
	case modeAdd:
		return "ADD"
	case modeHelp:
		return "HELP"
	case modeDetail:
		return "ENTRY"
	case modeEditField:
		return "EDIT"
	}
	return "LIST"
}

// statusRight shows the live countdowns.
func (m Model) statusRight(seg func(s, fg string) string, sp string) string {
	th := m.cfg.Theme
	var parts []string
	if m.clipLeft > 0 {
		parts = append(parts, seg(fmt.Sprintf("clipboard: %s %ds", m.clipLabel, m.clipLeft), th.Warning))
	}
	if m.revealed {
		if m.revealLeft > 0 {
			parts = append(parts, seg(fmt.Sprintf("revealed %ds", m.revealLeft), th.Danger))
		} else {
			parts = append(parts, seg("revealed", th.Danger))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, sp) + seg(" ", "")
}

// textOn picks near-black or white text for a hex background, whichever reads
// better. Presets put both pale and saturated colors behind badges (dark's
// accent is a light yellow), so a fixed text color fails on one or the other.
func textOn(bg string) string {
	hex := strings.TrimPrefix(strings.TrimSpace(bg), "#")
	if len(hex) != 6 {
		return "#FFFFFF"
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return "#FFFFFF"
	}
	r, g, b := float64(v>>16&0xFF), float64(v>>8&0xFF), float64(v&0xFF)
	// Perceived brightness (ITU-R BT.601 weights) is enough to pick a side.
	if 0.299*r+0.587*g+0.114*b > 150 {
		return "#0F172A"
	}
	return "#FFFFFF"
}

func (m Model) storeDir() string {
	if m.store == nil {
		return ""
	}
	return m.store.Dir()
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// humanTime renders an mtime compactly: a clock for today, a date otherwise.
func humanTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	if t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("Jan 02")
	}
	return t.Format("2006-01-02")
}

// shortenPath abbreviates the user's home directory to ~.
func shortenPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
