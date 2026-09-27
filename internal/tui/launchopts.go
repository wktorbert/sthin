package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wktorbert/sthin/internal/device"
)

// o: Android launch options for the selected AVD, saved as its preference and
// used by every later boot (TUI Enter, sthin boot, MCP, serve).

type launchRow struct {
	label string
	on    bool
}

type launchForm struct {
	target device.Device
	rows   []launchRow // cold boot, audio, low-RAM, headless
	ram    string      // RAM MB text; empty = default
	cursor int         // 0..len(rows): the last index is the RAM field
}

func (m Model) openLaunchOptions(d device.Device) Model {
	if d.Platform != device.Android || d.Kind == device.Physical {
		m.status = "launch options apply to Android emulators only"
		return m
	}
	prefs, _, err := m.prefs.LoadPrefs(d.ID)
	if err != nil {
		m.status = "prefs: " + err.Error()
		return m
	}
	l := prefs.Launch
	f := launchForm{target: d, rows: []launchRow{
		{"Cold boot (ignore quick-boot snapshot)", device.Bool(l.ColdBoot, false)},
		{"Audio on", device.Bool(l.Audio, false)},
		{"Low-RAM mode (-lowram; some images fail)", device.Bool(l.LowRAM, false)},
		{"Headless (no window)", device.Bool(l.Headless, false)},
	}}
	if l.RAMMB != nil {
		f.ram = strconv.Itoa(*l.RAMMB)
	}
	m.launch, m.mode = &f, modeLaunch
	return m
}

func (m Model) launchKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.launch
	ramRow := len(f.rows)
	switch k.String() {
	case "esc", "q":
		m.launch, m.mode = nil, modeList
	case "up", "k":
		if f.cursor > 0 {
			f.cursor--
		}
	case "down", "j", "tab":
		if f.cursor < ramRow {
			f.cursor++
		}
	case " ", "space":
		if f.cursor < ramRow {
			f.rows[f.cursor].on = !f.rows[f.cursor].on
		}
	case "backspace":
		if f.cursor == ramRow && f.ram != "" {
			f.ram = f.ram[:len(f.ram)-1]
		}
	case "enter":
		l := device.LaunchOptions{
			ColdBoot: device.BoolPtr(f.rows[0].on), Audio: device.BoolPtr(f.rows[1].on),
			LowRAM: device.BoolPtr(f.rows[2].on), Headless: device.BoolPtr(f.rows[3].on),
		}
		if f.ram != "" {
			n, err := strconv.Atoi(f.ram)
			if err != nil || n <= 0 {
				m.status = "RAM must be a positive number of MB"
				return m, nil
			}
			l.RAMMB = device.IntPtr(n)
		} else {
			l.RAMMB = device.IntPtr(0) // explicit "default"
		}
		if err := m.prefs.MergeLaunch(f.target.ID, l); err != nil {
			m.status = "save launch options: " + err.Error()
		} else {
			m.status = "launch options saved for " + f.target.Name
		}
		m.launch, m.mode = nil, modeList
	default:
		if f.cursor == ramRow && k.Type == tea.KeyRunes {
			for _, r := range k.Runes {
				if r >= '0' && r <= '9' && len(f.ram) < 5 {
					f.ram += string(r)
				}
			}
		}
	}
	return m, nil
}

func (m Model) launchLines() []string {
	f := m.launch
	var lines []string
	for i, r := range f.rows {
		box := "[ ]"
		if r.on {
			box = okStyle.Render("[✓]")
		}
		line := box + " " + r.label
		if i == f.cursor {
			line = selStyle.Render(fmt.Sprintf("%s %s", map[bool]string{true: "[✓]", false: "[ ]"}[r.on], r.label))
		}
		lines = append(lines, line)
	}
	ram := f.ram
	if ram == "" {
		ram = dimStyle.Render("default")
	}
	ramLine := "RAM MB: " + ram
	if f.cursor == len(f.rows) {
		ramLine = "RAM MB: " + selStyle.Render(f.ram+"▏")
	}
	lines = append(lines, ramLine, "", dimStyle.Render("space toggle · digits set RAM · enter save · esc cancel"))
	return lines
}

// launchSummary describes saved switches for the Details panel.
func launchSummary(l device.LaunchOptions) string {
	var parts []string
	if device.Bool(l.ColdBoot, false) {
		parts = append(parts, "cold boot")
	}
	if device.Bool(l.Audio, false) {
		parts = append(parts, "audio")
	}
	if device.Bool(l.LowRAM, false) {
		parts = append(parts, "lowram")
	}
	if device.Bool(l.Headless, false) {
		parts = append(parts, "headless")
	}
	if n := device.Int(l.RAMMB, 0); n > 0 {
		parts = append(parts, fmt.Sprintf("%d MB", n))
	}
	return joinOrNone(parts)
}
