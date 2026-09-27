package tui

import (
	"fmt"
	"strings"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/profile"
	"github.com/wktorbert/sthin/internal/state"
)

// The category picker: choose which Categories stay enabled on the selected
// device. The choice is saved as the device's preference and used by every
// later slim boot, from the TUI or the CLI, until changed here.

type pickRow struct {
	cat  profile.Category
	keep bool // true = category stays enabled (excepted from slimming)
}

// openPicker builds the rows for d from its platform Profile and saved prefs.
func (m Model) openPicker(d device.Device) Model {
	prof, err := profile.Load(string(d.Platform))
	if err != nil {
		m.status = "profile: " + err.Error()
		return m
	}
	prefs, _, err := m.prefs.LoadPrefs(d.ID)
	if err != nil {
		m.status = "prefs: " + err.Error()
		return m
	}
	kept := map[string]bool{}
	for _, id := range prefs.Except {
		kept[id] = true
	}
	m.pick = nil
	for _, c := range prof.Categories {
		if !c.Default {
			continue // non-default categories (aggressive) are CLI-only
		}
		m.pick = append(m.pick, pickRow{cat: c, keep: kept[c.ID]})
	}
	m.pickCursor, m.target, m.mode = 0, d, modePicker
	return m
}

// applyPicker saves the selection and slim boots the device with it.
func (m Model) applyPicker() (Model, error) {
	except := []string{}
	for _, r := range m.pick {
		if r.keep {
			except = append(except, r.cat.ID)
		}
	}
	return m, m.prefs.SavePrefs(state.Prefs{ID: m.target.ID, Except: except})
}

// kept returns the saved Category IDs kept enabled for id, for the Details panel.
func (m Model) kept(id string) []string {
	p, ok, err := m.prefs.LoadPrefs(id)
	if err != nil || !ok {
		return nil
	}
	return p.Except
}

// pickerLines renders the picker rows for the dialog.
func (m Model) pickerLines() []string {
	var lines []string
	for i, r := range m.pick {
		box := "[ ]"
		if r.keep {
			box = okStyle.Render("[✓]")
		}
		line := fmt.Sprintf("%s %-13s %s %s", box, r.cat.ID, fit(r.cat.Name, 30), dimStyle.Render(fmt.Sprintf("(%d)", len(r.cat.Items))))
		if i == m.pickCursor {
			line = selStyle.Render(fmt.Sprintf("%s %-13s %s (%d)", box, r.cat.ID, fit(r.cat.Name, 30), len(r.cat.Items)))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", dimStyle.Render("✓ = keep enabled (not slimmed).  space toggle · enter apply and boot · esc cancel"))
	return lines
}

func joinOrNone(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, ", ")
}
