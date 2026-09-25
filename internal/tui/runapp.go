package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/project"
)

// x: run the project in Lean's working directory on the selected device.
// Framework projects take over the terminal (tea.ExecProcess) until the user
// quits them; native projects install and launch in the background.

type runDoneMsg struct {
	detail string
	err    error
}

func (m Model) runApp() (tea.Model, tea.Cmd) {
	d, p, ok := m.selected()
	if !ok {
		return m, nil
	}
	if d.Kind != device.Physical && d.State != device.Booted {
		m.status = "boot " + d.Name + " first (enter), then x to run"
		return m, nil
	}
	cwd, _ := os.Getwd()
	prj := project.Detect(cwd)
	if prj.Kind == project.Unknown {
		m.status = "no Flutter, React Native, Xcode or Gradle project in " + cwd
		return m, nil
	}
	ios := d.Platform == device.IOS
	if (ios && !prj.SupportsIOS()) || (!ios && !prj.SupportsAndroid()) {
		m.status = string(prj.Kind) + " project cannot run on " + string(d.Platform)
		return m, nil
	}
	ctl, ok := p.(device.Controller)
	if !ok {
		m.status = "provider cannot drive devices"
		return m, nil
	}
	serial, err := ctl.Serial(m.ctx, d.ID)
	if err != nil {
		m.status = "run: " + err.Error()
		return m, nil
	}
	if c := prj.HandoffCmd(ios, serial); c != nil {
		m.status = "running " + string(prj.Kind) + " on " + d.Name + "…"
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return runDoneMsg{detail: string(prj.Kind) + " run on " + d.Name + " finished", err: err}
		})
	}
	m.busy, m.status = true, "installing "+prj.Name+" on "+d.Name+"…"
	ctx, reg, run, env := m.ctx, m.reg, m.run, m.env
	return m, func() tea.Msg {
		res, err := project.Run(ctx, project.Deps{Reg: reg, Run: run, Env: env}, project.Opts{Dir: prj.Dir, Device: d.ID, NoHandoff: true}, func(device.Stage) {})
		if err != nil {
			return runDoneMsg{err: err}
		}
		return runDoneMsg{detail: res.AppID + " launched on " + d.Name}
	}
}
