package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Box-drawing helpers for the simutil-style bordered panels. lipgloss borders
// cannot put a title on the top edge, so the frame is drawn by hand.

// fit pads or truncates a (possibly styled) line to exactly w visible cells.
func fit(line string, w int) string {
	if w <= 0 {
		return ""
	}
	line = ansi.Truncate(line, w, "")
	if pad := w - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

// panel frames lines in a rounded box w cells wide with the title on the top
// edge. innerH fixes the number of content rows (padding with blanks); 0 means
// "as many rows as there are lines". Extra lines are dropped.
func panel(title string, lines []string, w, innerH int, focused bool) string {
	border := borderIdle
	if focused {
		border = borderFocused
	}
	if w < 6 {
		w = 6
	}
	inner := w - 2
	if innerH <= 0 {
		innerH = len(lines)
	}
	if innerH < 1 {
		innerH = 1
	}

	// Top edge: ╭─ Title ─────╮
	t := " " + title + " "
	t = ansi.Truncate(t, inner-2, "")
	dashes := inner - 1 - lipgloss.Width(t)
	if dashes < 0 {
		dashes = 0
	}
	var b strings.Builder
	b.WriteString(border.Render("╭─") + labelStyle.Bold(true).Render(t) + border.Render(strings.Repeat("─", dashes)+"╮") + "\n")

	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		b.WriteString(border.Render("│") + fit(line, inner) + border.Render("│") + "\n")
	}
	b.WriteString(border.Render("╰" + strings.Repeat("─", inner) + "╯"))
	return b.String()
}

// fitHeights shrinks a list of natural panel heights (borders included) until
// their sum fits in avail, taking rows from the tallest panel first. A panel
// never drops below 3 rows (one content row plus the frame).
func fitHeights(natural []int, avail int) []int {
	out := append([]int(nil), natural...)
	sum := 0
	for _, h := range out {
		sum += h
	}
	for sum > avail {
		tallest := -1
		for i, h := range out {
			if h > 3 && (tallest < 0 || h > out[tallest]) {
				tallest = i
			}
		}
		if tallest < 0 {
			break
		}
		out[tallest]--
		sum--
	}
	return out
}

// window returns the slice bounds [from, to) of n rows that keep row idx
// visible in a viewport of h rows.
func window(n, h, idx int) (int, int) {
	if h <= 0 || n <= h {
		return 0, n
	}
	from := idx - h + 1
	if from < 0 {
		from = 0
	}
	if from+h > n {
		from = n - h
	}
	return from, from + h
}
