package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// keyHint is a single key/label pair shown in the footer hint bar.
type keyHint struct {
	key   string
	label string
}

// panelInnerWidth is the content width available inside a framed panel.
//
// Leave two terminal cells unused on the right. Writing near the bottom/right
// edge can put some terminals into pending-wrap, which shows up as a duplicated
// status bar on the next repaint. The status bar uses the same spare-cell rule.
func (m Model) panelInnerWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	inner := w - 4 // two borders plus two spare anti-wrap cells
	if inner < 10 {
		inner = 10
	}
	return inner
}

// panel frames body in a rounded border with the title embedded in the top
// edge. Every body line is padded (ANSI-aware) to the inner width so the right
// border stays aligned regardless of the styling inside.
func (m Model) panel(title, body string) string {
	inner := m.panelInnerWidth()
	bs := m.styles.Border

	var b strings.Builder
	b.WriteString(m.panelTop(title, inner))
	b.WriteString("\n")

	left := bs.Render("│")
	right := bs.Render("│")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w < inner {
			line += strings.Repeat(" ", inner-w)
		} else if w > inner {
			line = truncateANSI(line, inner)
		}
		b.WriteString(left + line + right)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(bs.Render("╰" + strings.Repeat("─", inner) + "╯"))
	return b.String()
}

// panelTop renders the rounded top edge with an embedded, accent-styled title.
func (m Model) panelTop(title string, inner int) string {
	bs := m.styles.Border
	title = strings.TrimSpace(title)
	if title == "" {
		return bs.Render("╭" + strings.Repeat("─", inner) + "╮")
	}
	label := m.styles.PanelTitle.Render(title)
	midDashes := inner - 3 - lipgloss.Width(label)
	if midDashes < 0 {
		midDashes = 0
	}
	return bs.Render("╭─ ") + label + bs.Render(" "+strings.Repeat("─", midDashes)+"╮")
}

// hintBar renders a row of highlighted key chips with muted labels.
//
// When the row doesn't fit, whole hints are dropped from the end rather than
// the line being cut mid-chip: a hint bar ending in "q  qui" reads as a
// rendering bug, while a shorter bar just reads as a shorter bar. Hints are
// therefore ordered most- to least-important by their callers.
func (m Model) hintBar(hints []keyHint) string {
	sep := m.styles.Muted.Render("  ")

	render := func(n int) string {
		parts := make([]string, 0, n)
		for _, h := range hints[:n] {
			cap := m.styles.KeyCap.Render(" " + displayKey(h.key) + " ")
			parts = append(parts, cap+" "+m.styles.KeyLabel.Render(h.label))
		}
		return " " + strings.Join(parts, sep)
	}

	// Two cells short of the terminal width, so the row can't wrap onto a
	// second line or trigger pending-wrap at the right edge.
	maxWidth := m.width - 2
	if m.width <= 0 {
		return render(len(hints))
	}
	if maxWidth < 1 {
		maxWidth = 1
	}

	for n := len(hints); n > 0; n-- {
		out := render(n)
		if lipgloss.Width(out) <= maxWidth {
			return out
		}
	}
	// Even a single hint is too wide; fall back to clipping.
	return truncateANSI(render(1), maxWidth)
}

// displayKey turns a raw key binding into something readable in a hint or the
// help reference — a bound space would otherwise render as a blank chip.
func displayKey(key string) string {
	switch key {
	case " ":
		return "space"
	case "":
		return "—"
	}
	return key
}

// stripeLine re-applies style's background across an already-rendered line so a
// zebra tint survives the inner ANSI resets emitted by per-cell styling
// (lipgloss drops the background after each reset, leaving gaps otherwise).
// When the style carries no background it returns the line unchanged.
func stripeLine(style lipgloss.Style, line string) string {
	open := bgOpenSeq(style)
	if open == "" {
		return line
	}
	return open + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+open) + "\x1b[0m"
}

// bgOpenSeq extracts the leading SGR escape a style emits before its content,
// by probing with a sentinel rune that never appears inside an escape sequence.
func bgOpenSeq(style lipgloss.Style) string {
	probe := style.Render("M")
	if idx := strings.IndexByte(probe, 'M'); idx > 0 {
		return probe[:idx]
	}
	return ""
}

// truncateANSI cuts a possibly styled string to a visible width, preserving SGR
// escape sequences and appending a reset so colors don't bleed.
func truncateANSI(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	visible := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\x1b' {
			// copy the full escape sequence verbatim
			b.WriteRune(r)
			i++
			for i < len(runes) {
				b.WriteRune(runes[i])
				if (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') {
					break
				}
				i++
			}
			continue
		}
		rw := lipgloss.Width(string(r))
		if visible+rw > width {
			break
		}
		b.WriteRune(r)
		visible += rw
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// padTo right-pads an already-styled string to a visible width.
func padTo(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return truncateANSI(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

// centerBlock centers every line of a multi-line block within width.
func centerBlock(block string, width int) string {
	if width <= 0 {
		return block
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w >= width {
			continue
		}
		lines[i] = strings.Repeat(" ", (width-w)/2) + line
	}
	return strings.Join(lines, "\n")
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
