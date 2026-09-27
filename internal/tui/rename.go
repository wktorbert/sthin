package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wktorbert/sthin/internal/device"
)

// n: rename the selected simulator or AVD.

type renameForm struct {
	target device.Device
	value  string
}

type renameDoneMsg struct {
	name string
	err  error
}

func (m Model) openRename(d device.Device, p device.Provider) Model {
	if d.Kind == device.Physical {
		m.status = d.Name + " is a physical device; Sthin never modifies physical devices"
		return m
	}
	if _, ok := p.(device.Renamer); !ok {
		m.status = "provider cannot rename devices"
		return m
	}
	m.rename, m.mode = &renameForm{target: d, value: d.Name}, modeRename
	return m
}

func (m Model) renameKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.rename
	switch k.String() {
	case "esc":
		m.rename, m.mode = nil, modeList
	case "backspace":
		if r := []rune(f.value); len(r) > 0 {
			f.value = string(r[:len(r)-1])
		}
	case "ctrl+u":
		f.value = ""
	case "enter":
		name := strings.TrimSpace(f.value)
		if name == "" {
			m.status = "name must not be empty"
			return m, nil
		}
		d := f.target
		p, _ := m.reg.Provider(d.Platform)
		rn := p.(device.Renamer)
		ctx := m.ctx
		m.rename, m.mode, m.busy, m.status = nil, modeList, true, "renaming "+d.Name+"…"
		return m, func() tea.Msg { return renameDoneMsg{name: name, err: rn.Rename(ctx, d.ID, name)} }
	default:
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			f.value += string(k.Runes)
		}
	}
	return m, nil
}

func (m Model) renameLines() []string {
	return []string{
		labelStyle.Render("New name") + ": " + selStyle.Render(fit(m.rename.value+"▏", 40)),
		"",
		dimStyle.Render("enter save · ctrl+u clear · esc cancel"),
	}
}
