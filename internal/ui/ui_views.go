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
	case modeDetail:
		return m.fillView(m.renderDetailView())
	default:
		return m.fillView(m.renderListView())
	}
}

// fillView pads body to the terminal height and pins the status bar to the
// last row, so the bar never floats mid-screen on a short list.
func (m Model) fillView(body string) string {
	status := m.renderStatusBar()
	if m.height <= 0 {
		return body + "\n" + status
	}

	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	target := m.height - 1 // rows above the status bar
	if len(lines) > target {
		lines = lines[:target]
	}
	for len(lines) < target {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n" + status
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func (m Model) renderListView() string {
	inner := m.panelInnerWidth()
	rows := m.listRows()

	// Column layout: the Updated column is fixed and the name takes the rest,
	// because a truncated date tells you nothing while a truncated name is
	// still recognisable.
	const updatedWidth = 12
	nameWidth := inner - updatedWidth - 3
	if nameWidth < 10 {
		nameWidth = 10
	}

	var b strings.Builder
	header := " " + padTo("NAME", nameWidth) + " " + padTo("UPDATED", updatedWidth) + " "
	b.WriteString(m.styles.TableHeader.Render(padTo(header, inner)))
	b.WriteString("\n")

	if len(m.matches) == 0 {
		b.WriteString(m.emptyListMessage())
		for i := 1; i < rows; i++ {
			b.WriteString("\n")
		}
	} else {
		end := m.scroll + rows
		if end > len(m.matches) {
			end = len(m.matches)
		}
		for i := m.scroll; i < end; i++ {
			b.WriteString(m.renderListRow(i, nameWidth, updatedWidth, inner))
			if i < end-1 {
				b.WriteString("\n")
			}
		}
		for i := end - m.scroll; i < rows; i++ {
			b.WriteString("\n")
		}
	}

	title := fmt.Sprintf("Passer · %d", len(m.matches))
	if m.query != "" {
		title = fmt.Sprintf("Passer · %d/%d", len(m.matches), len(m.items))
	}

	out := m.panel(title, b.String())
	if m.mode == modeSearch || m.query != "" {
		out += "\n" + m.renderSearchLine()
	}
	out += "\n" + m.hintBar(m.listHints())

	if m.mode == modeConfirmDelete {
		out += "\n" + m.renderConfirmLine()
	}
	if m.mode == modeRename {
		out += "\n" + m.renderRenameLine()
	}
	if m.mode == modeCommand {
		out += "\n" + m.renderCommandLine()
	}
	return out
}

func (m Model) renderListRow(i, nameWidth, updatedWidth, inner int) string {
	mt := m.matches[i]
	if mt.index < 0 || mt.index >= len(m.items) {
		// Matches built against a longer item slice; see currentItem.
		return strings.Repeat(" ", inner)
	}
	item := m.items[mt.index]
	selected := i == m.cursor

	name := m.renderName(item.Name, mt.positions, nameWidth, selected)
	updated := m.styles.Muted.Render(padTo(humanTime(item.Modified), updatedWidth))
	if selected {
		updated = padTo(humanTime(item.Modified), updatedWidth)
	}

	cursor := "  "
	if selected {
		cursor = m.styles.Accent.Render("▸ ")
		if m.loading == item.Name {
			cursor = m.styles.Warning.Render("◌ ")
		}
	}

	line := cursor + padTo(name, nameWidth) + " " + updated + " "
	line = padTo(line, inner)

	if selected {
		return stripeLine(m.styles.Selection, line)
	}
	if i%2 == 1 {
		return stripeLine(m.styles.RowStripe, line)
	}
	return line
}

// renderName styles one entry name: the folder prefix is muted so the leaf
// stands out, and fuzzy-matched characters are highlighted in place.
func (m Model) renderName(name string, positions []int, width int, selected bool) string {
	runes := []rune(name)
	if len(runes) > width {
		// Keep the tail: the leaf name is what identifies the entry, so drop
		// characters from the folder prefix instead.
		runes = append([]rune("…"), runes[len(runes)-width+1:]...)
		positions = nil
	}

	hit := make(map[int]bool, len(positions))
	for _, p := range positions {
		hit[p] = true
	}

	slash := -1
	for i, r := range runes {
		if r == '/' {
			slash = i
		}
	}

	var b strings.Builder
	for i, r := range runes {
		s := string(r)
		switch {
		case hit[i]:
			b.WriteString(m.styles.MatchHit.Render(s))
		case selected:
			b.WriteString(s)
		case i <= slash:
			b.WriteString(m.styles.Muted.Render(s))
		default:
			b.WriteString(s)
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
	return m.styles.Accent.Render(" : ") + m.input.View()
}

func (m Model) renderRenameLine() string {
	return m.styles.Accent.Render(" rename → ") + m.input.View()
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

func (m Model) renderDetailView() string {
	if m.entry == nil {
		return m.renderListView()
	}
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

	var b strings.Builder
	for i, r := range rows {
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
		value = style.Render(truncateANSI(value, valueWidth))

		line := padTo(num+label+"  "+value, inner)
		if i == m.entryCursor {
			line = stripeLine(m.styles.Selection, line)
		}
		b.WriteString(line)
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}

	title := m.entry.Name
	if m.revealed {
		title += "  ·  REVEALED"
	}

	out := m.panel(title, b.String())
	out += "\n" + m.hintBar(m.detailHints())
	if m.mode == modeCommand {
		out += "\n" + m.renderCommandLine()
	}
	return out
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
		{k.Edit, "edit"},
		{k.Quit, "back"},
	}
}

// ---------------------------------------------------------------------------
// Add modal
// ---------------------------------------------------------------------------

func (m Model) renderAddModal() string {
	a := m.add
	if a == nil {
		return m.renderListView()
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
			{k.CopyUsername, "copy the username field"},
			{k.CopyURL, "copy the url field"},
			{"1-9", "copy the nth field (detail view)"},
			{"c", "copy the field under the cursor"},
		}},
		{"Entry", [][2]string{
			{k.Reveal, "reveal / hide the password (detail view)"},
			{k.Add, "add an entry"},
			{k.Edit, "edit in $EDITOR via pass edit"},
			{k.Delete, "delete, with confirmation"},
			{k.Rename, "rename / move"},
			{k.Reload, "reload the store"},
		}},
		{"Commands", [][2]string{
			{":add [name]", "open the new-entry dialog"},
			{":generate <name> [len]", "add a password-only entry"},
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

func (m Model) renderStatusBar() string {
	width := m.width - 2 // the same anti-wrap gutter the panels leave
	if width < 10 {
		width = 10
	}

	left := m.statusLeft()
	right := m.statusRight()

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		left = truncateANSI(left, width-lipgloss.Width(right)-1)
		gap = width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 0 {
			gap = 0
		}
	}

	line := left + strings.Repeat(" ", gap) + right
	return stripeLine(m.styles.Status, padTo(line, width))
}

func (m Model) statusLeft() string {
	if m.loading != "" {
		return " decrypting " + m.loading + "…"
	}
	if m.status != "" {
		if m.statusErr {
			return " " + m.styles.Danger.Render(m.status)
		}
		return " " + m.status
	}
	if m.mode == modeDetail && m.entry != nil {
		return " " + m.entry.Name
	}
	return " " + shortenPath(m.store.Dir())
}

// statusRight shows the live countdowns. They are the only way to know how long
// a secret stays exposed, so they sit in a fixed spot rather than in the
// transient status message.
func (m Model) statusRight() string {
	var parts []string
	if m.clipLeft > 0 {
		parts = append(parts, fmt.Sprintf("📋 %s %ds", m.clipLabel, m.clipLeft))
	}
	if m.revealed {
		if m.revealLeft > 0 {
			parts = append(parts, fmt.Sprintf("👁 %ds", m.revealLeft))
		} else {
			parts = append(parts, "👁 shown")
		}
	}
	if len(parts) == 0 {
		return " "
	}
	return strings.Join(parts, "  ") + " "
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
