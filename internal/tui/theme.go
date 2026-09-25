package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/wktorbert/lean-sim/internal/device"
)

// Dracula palette. lipgloss degrades these to the nearest 256/16 colour on
// terminals without truecolor, so the theme still reads on a plain Terminal.app.
const (
	draculaPurple  = lipgloss.Color("#bd93f9")
	draculaGreen   = lipgloss.Color("#50fa7b")
	draculaYellow  = lipgloss.Color("#f1fa8c")
	draculaRed     = lipgloss.Color("#ff5555")
	draculaCyan    = lipgloss.Color("#8be9fd")
	draculaComment = lipgloss.Color("#6272a4")
	draculaFg      = lipgloss.Color("#f8f8f2")
)

// themeName is shown in the header, the way simutil shows "Theme: dracula".
const themeName = "dracula"

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(draculaPurple)
	labelStyle    = lipgloss.NewStyle().Foreground(draculaPurple) // panel titles, detail labels, OS column
	bodyStyle     = lipgloss.NewStyle().Foreground(draculaFg)
	dimStyle      = lipgloss.NewStyle().Foreground(draculaComment)
	selStyle      = lipgloss.NewStyle().Reverse(true)
	okStyle       = lipgloss.NewStyle().Foreground(draculaGreen)
	failStyle     = lipgloss.NewStyle().Foreground(draculaRed)
	warnStyle     = lipgloss.NewStyle().Foreground(draculaYellow)
	cyanStyle     = lipgloss.NewStyle().Foreground(draculaCyan)
	borderFocused = lipgloss.NewStyle().Foreground(draculaPurple)
	borderIdle    = lipgloss.NewStyle().Foreground(draculaComment)

	slimStyle = map[device.SlimState]lipgloss.Style{
		device.Slim:    okStyle,
		device.Partial: warnStyle,
		device.Stock:   dimStyle,
		device.Unknown: dimStyle,
	}
)

// Icons, matching simutil's glyphs.
const (
	iconOn  = "●"
	iconOff = "○"
)
