package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wktorbert/sthin/internal/device"
)

// The wireless ADB dialog: connect to a phone over Wi-Fi, pairing first when
// the phone shows a pairing code. A minimal text form; no extra dependency.

var wirelessFields = []string{"Connect address (ip:port)", "Pair address (ip:port, optional)", "Pairing code (optional)"}

type wirelessForm struct {
	values [3]string
	active int
}

type wirelessMsg struct {
	detail string
	err    error
}

// wireless finds the Provider that implements wireless ADB (Android).
func (m Model) wireless() device.WirelessADB {
	for _, p := range m.reg.Providers {
		if w, ok := p.(device.WirelessADB); ok {
			if ok, _ := p.Available(); ok {
				return w
			}
		}
	}
	return nil
}

func (m Model) openWireless() Model {
	if m.wireless() == nil {
		m.status = "wireless ADB needs the Android provider (adb not found)"
		return m
	}
	m.form, m.mode = wirelessForm{}, modeWireless
	return m
}

// wirelessKey edits the form; enter runs pair (if given) then connect.
func (m Model) wirelessKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.form
	switch k.String() {
	case "esc":
		m.mode = modeList
	case "tab", "down":
		f.active = (f.active + 1) % len(f.values)
	case "shift+tab", "up":
		f.active = (f.active + len(f.values) - 1) % len(f.values)
	case "backspace":
		if v := f.values[f.active]; v != "" {
			r := []rune(v)
			f.values[f.active] = string(r[:len(r)-1])
		}
	case "enter":
		connect := strings.TrimSpace(f.values[0])
		if connect == "" {
			m.status = "connect address is required"
			return m, nil
		}
		pair, code := strings.TrimSpace(f.values[1]), strings.TrimSpace(f.values[2])
		w, ctx := m.wireless(), m.ctx
		m.mode, m.status, m.busy = modeList, "connecting to "+connect+"…", true
		return m, func() tea.Msg { return runWireless(ctx, w, connect, pair, code) }
	default:
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			f.values[f.active] += string(k.Runes)
		}
	}
	return m, nil
}

func runWireless(ctx context.Context, w device.WirelessADB, connect, pair, code string) tea.Msg {
	if pair != "" {
		if out, err := w.Pair(ctx, pair, code); err != nil {
			return wirelessMsg{detail: out, err: err}
		}
	}
	out, err := w.Connect(ctx, connect)
	return wirelessMsg{detail: out, err: err}
}

// wirelessLines renders the form for the dialog.
func (m Model) wirelessLines() []string {
	var lines []string
	for i, label := range wirelessFields {
		box := fit(m.form.values[i], 26)
		if i == m.form.active {
			box = selStyle.Render(box)
		}
		lines = append(lines, labelStyle.Render(fit(label, 34))+" ["+box+"]")
	}
	lines = append(lines, "", dimStyle.Render("Phone: Settings › Developer options › Wireless debugging. Pair once with the"),
		dimStyle.Render("code shown under \"Pair device with pairing code\", then connect to the main address."),
		"", dimStyle.Render("tab next field · enter connect · esc cancel"))
	return lines
}
