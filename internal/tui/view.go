package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/wktorbert/lean-sim/internal/device"
)

// Layout constants. The screen is: header line, body, status line. The body is
// a left column of one panel per platform and a fixed-width Details panel on
// the right, the way simutil lays it out.
const (
	defaultWidth  = 100
	defaultHeight = 30
	detailsWidth  = 46
	minListWidth  = 40
	detailLabelW  = 10
)

const keyHelp = `  ↑/↓  j/k   move
  tab        next panel
  enter      slim boot the selected device
  r          restore to stock (asks y/n)
  t          shut down
  c          choose categories to keep enabled (saved per device)
  w          wireless ADB: pair and connect a phone over Wi-Fi
  x          run the app in the current directory on the device
  l          follow the device log live (filter, level, save)
  ?          toggle this help
  q          quit`

// panelTitle is the section header for a platform, matching simutil's wording.
func panelTitle(p device.Platform) string {
	if p == device.IOS {
		return "iOS Simulators"
	}
	return "Android Emulators"
}

// platformLabel is the human name used in rows and the Details panel.
func platformLabel(p device.Platform) string {
	if p == device.IOS {
		return "iOS"
	}
	return "Android"
}

// osLabel renders the version column: "iOS 26.5" or "API 33".
func osLabel(d device.Device) string {
	if d.Platform == device.IOS {
		return "iOS " + d.OSVersion
	}
	return d.OSVersion
}

func (m Model) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	return w, h
}

// View implements tea.Model.
func (m Model) View() string {
	w, h := m.size()
	bodyH := h - 2 // header + status bar
	var body string
	switch {
	case m.help:
		body = m.dialog("Help", keyHelp+"\n\n"+dimStyle.Render("press any key to close"), w, bodyH)
	case m.mode == modeLogs:
		body = panel("Logs: "+m.logs.target.Name, m.logsLines(w, bodyH-2), w, bodyH-2, true)
	case m.mode == modeWireless:
		body = m.dialogLines("Wireless ADB", m.wirelessLines(), w, bodyH)
	case m.mode == modePicker:
		body = m.dialogLines("Keep enabled on "+m.target.Name, m.pickerLines(), w, bodyH)
	case m.mode == modeConfirm:
		txt := fmt.Sprintf("Restore %s to stock?\nThis undoes everything Lean changed.\n\n%s", m.target.Name, warnStyle.Render("[y/N]"))
		body = m.dialog("Restore", txt, w, bodyH)
	default:
		body = m.columns(w, bodyH)
	}
	return m.header(w) + "\n" + body + "\n" + m.statusBar(w)
}

func (m Model) header(w int) string {
	left := headerStyle.Render(" " + iconOn + " Lean v" + m.version + " ")
	return fit(left+dimStyle.Render("Theme: "+themeName), w)
}

func (m Model) statusBar(w int) string {
	var s string
	switch {
	case m.mode == modeLogs:
		s = "Filter: / | Level: L | Follow: f | Save: s | Clear: c | Close: esc"
	case m.mode == modeProgress && m.done:
		s = "Back: <enter> | Quit: q"
	case m.status != "":
		s = m.status + "   " + dimStyle.Render("Help: ?")
	default:
		s = "Launch: <enter> | Logs: l | Run app: x | Categories: c | Restore: r | Shutdown: t | Wireless: w | Switch: <tab> | Help: ? | Quit: q"
	}
	return fit(" "+s, w)
}

// columns lays out the left panels and the Details panel side by side, or
// stacks the Details panel underneath when the terminal is narrow.
func (m Model) columns(w, bodyH int) string {
	if w < minListWidth+detailsWidth {
		listH := bodyH - 9
		if listH < 3 {
			listH = 3
		}
		return m.left(w, listH) + "\n" + m.details(w, bodyH-listH-1)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.left(w-detailsWidth, bodyH), m.details(detailsWidth, bodyH))
}

// left renders one panel per platform (or the progress panel during an
// operation), sharing bodyH rows between them.
func (m Model) left(w, bodyH int) string {
	if m.mode == modeProgress {
		return m.progressPanel(w, bodyH)
	}
	if !m.loaded {
		return panel("Devices", []string{dimStyle.Render("Loading devices...")}, w, bodyH-2, true)
	}
	groups := m.groups()
	// Natural height per panel: its rows plus the frame.
	natural := make([]int, len(groups))
	for i, g := range groups {
		natural[i] = max(len(g.devices), 1) + 2
	}
	heights := fitHeights(natural, bodyH)

	var out []string
	first := 0 // flat index of the panel's first device
	for i, g := range groups {
		inner := heights[i] - 2
		focused := m.cursor >= first && m.cursor < first+len(g.devices)
		var lines []string
		if g.note != "" {
			lines = []string{g.note}
		} else {
			from, to := window(len(g.devices), inner, m.cursor-first)
			for j := from; j < to; j++ {
				lines = append(lines, m.row(g.devices[j], w-2, first+j == m.cursor))
			}
		}
		out = append(out, panel(g.title, lines, w, inner, focused))
		first += len(g.devices)
	}
	return strings.Join(out, "\n")
}

// row renders one device line: state dot, name, OS, state, slim badge, memory.
func (m Model) row(d device.Device, w int, selected bool) string {
	dot := dimStyle.Render(" " + iconOff + " ")
	state := dimStyle.Render(string(d.State))
	if d.Kind == device.Physical {
		state = dimStyle.Render("Offline")
		if d.State == device.Booted {
			dot, state = okStyle.Render(" "+iconOn+" "), okStyle.Render("Connected")
		}
		right := fmt.Sprintf("%s %s %s ", labelStyle.Render(fmt.Sprintf("%-9s", osLabel(d))), fit(state, 9), dimStyle.Render(fit(platformLabel(d.Platform)+" physical", 16)))
		nameW := max(w-lipgloss.Width(dot)-lipgloss.Width(right), 4)
		name := fit(d.Name, nameW)
		if selected {
			name = selStyle.Render(name)
		} else {
			name = bodyStyle.Render(name)
		}
		return dot + name + right
	}
	if d.State == device.Booted {
		dot = okStyle.Render(" " + iconOn + " ")
		state = okStyle.Render("Booted")
	} else if d.State == device.Booting {
		dot = warnStyle.Render(" " + iconOn + " ")
		state = warnStyle.Render("Booting")
	} else {
		state = dimStyle.Render("Shutdown")
	}
	mem := ""
	if d.FootprintMB != nil {
		mem = fmt.Sprintf("%d MB", *d.FootprintMB)
	}
	right := fmt.Sprintf("%s %s %s %7s ", labelStyle.Render(fmt.Sprintf("%-9s", osLabel(d))), fit(state, 8), fit(slimStyle[d.Slim].Render(string(d.Slim)), 7), mem)
	nameW := w - lipgloss.Width(dot) - lipgloss.Width(right)
	if nameW < 4 {
		nameW = 4
	}
	name := fit(d.Name, nameW)
	if selected {
		name = selStyle.Render(name)
	} else {
		name = bodyStyle.Render(name)
	}
	return dot + name + right
}

// details renders the Details panel for the selected device.
func (m Model) details(w, bodyH int) string {
	d, _, ok := m.selected()
	if !ok || m.mode == modeProgress {
		d = m.target
		ok = m.mode == modeProgress
	}
	if !ok {
		return panel("Details", []string{dimStyle.Render("Select a device to view details")}, w, bodyH-2, false)
	}
	mem := "-"
	if d.FootprintMB != nil {
		mem = fmt.Sprintf("%d MB", *d.FootprintMB)
	}
	rows := [][2]string{
		{"Name", d.Name},
		{"ID", d.ID},
		{"Platform", platformLabel(d.Platform)},
		{"Kind", string(d.Kind)},
		{"OS", osLabel(d)},
	}
	if d.Kind == device.Physical {
		st := "offline"
		if d.State == device.Booted {
			st = "connected"
		}
		rows = append(rows, [2]string{"State", st}, [2]string{"Model", d.Model})
	} else {
		rows = append(rows, [2]string{"State", string(d.State)}, [2]string{"Slim", string(d.Slim)}, [2]string{"Memory", mem})
	}
	if kept := m.kept(d.ID); len(kept) > 0 {
		rows = append(rows, [2]string{"Kept", joinOrNone(kept)})
	}
	if len(d.Warnings) > 0 {
		rows = append(rows, [2]string{"Warnings", strings.Join(d.Warnings, "; ")})
	}
	valueW := w - 2 - detailLabelW - 3
	var lines []string
	for _, r := range rows {
		value := lipgloss.NewStyle().Width(valueW).Render(r[1])
		for i, vl := range strings.Split(value, "\n") {
			if i == 0 {
				lines = append(lines, " "+labelStyle.Render(fmt.Sprintf("%-*s", detailLabelW, r[0]))+": "+vl)
			} else {
				lines = append(lines, strings.Repeat(" ", detailLabelW+3)+vl)
			}
		}
	}
	return panel("Details", lines, w, bodyH-2, false)
}

// progressPanel shows each stage of the running operation.
func (m Model) progressPanel(w, bodyH int) string {
	verb := map[string]string{"boot": "Slim booting", "restore": "Restoring"}[m.op]
	var lines []string
	for _, s := range m.stages {
		var mark string
		switch s.Status {
		case device.StageOK:
			mark = okStyle.Render("✓")
		case device.StageFail:
			mark = failStyle.Render("✗")
		case device.StageWarn:
			mark = warnStyle.Render("!")
		case device.StageSkip:
			mark = dimStyle.Render("-")
		default:
			mark = cyanStyle.Render("…")
		}
		lines = append(lines, strings.TrimRight(fmt.Sprintf(" %s %-17s %s", mark, s.Name, dimStyle.Render(s.Detail)), " "))
		if s.Status == device.StageFail && s.Command != "" {
			lines = append(lines, "     failed command: "+s.Command)
		}
	}
	if m.done {
		lines = append(lines, "")
		if m.opErr != nil {
			lines = append(lines, failStyle.Render(m.op+" failed: "+m.opErr.Error()))
			lines = append(lines, dimStyle.Render("The device is left as it is; restore undoes anything Lean changed."))
		} else {
			lines = append(lines, okStyle.Render(m.op+" complete"))
		}
		lines = append(lines, dimStyle.Render("enter to return to the list"))
	}
	return panel(verb+" "+m.target.Name, lines, w, bodyH-2, true)
}

// dialog renders a centred bordered box, used for help and confirmations.
func (m Model) dialog(title, text string, w, bodyH int) string {
	return m.dialogLines(title, strings.Split(text, "\n"), w, bodyH)
}

// dialogLines is dialog for pre-split (possibly styled) lines.
func (m Model) dialogLines(title string, lines []string, w, bodyH int) string {
	boxW := 0
	for _, l := range lines {
		boxW = max(boxW, lipgloss.Width(l)+4)
	}
	boxW = max(min(boxW, w), 20)
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	box := panel(title, lines, boxW, 0, true)
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}
