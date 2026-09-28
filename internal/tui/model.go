// Package tui is Sthin's terminal UI: one list of every simulator and emulator,
// Enter to slim boot, r to restore, t to shut down, ? for help. It talks only
// to device.Providers through the Registry.
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/host"
	"github.com/wktorbert/sthin/internal/state"
)

type mode int

const (
	modeList mode = iota
	modeProgress
	modeConfirm
	modePicker
	modeWireless
	modeLogs
	modeLaunch
	modeRename
)

// Messages.
type (
	listMsg   struct{ lists []device.PlatformList }
	tickMsg   struct{}
	stageMsg  struct{ stage device.Stage }
	opDoneMsg struct {
		op  string
		err error
	}
)

// Model is the Bubble Tea model.
type Model struct {
	reg       *device.Registry
	ctx       context.Context
	tickEvery time.Duration // 0 disables the idle refresh (tests)
	version   string
	prefs     state.Store // per-device category preferences
	run       device.Runner
	env       host.Env
	width     int // terminal size from the last WindowSizeMsg; 0 = unknown
	height    int

	lists  []device.PlatformList
	loaded bool
	cursor int
	mode   mode
	help   bool
	busy   bool   // an operation is running
	status string // one-line result shown under the list

	// progress view
	op     string
	target device.Device
	// confirmOp is what a y in the confirm dialog runs: restore or delete.
	confirmOp string
	stages    []device.Stage
	done      bool
	opErr     error
	events    chan tea.Msg

	// category picker
	pick       []pickRow
	pickCursor int

	form   wirelessForm // wireless ADB dialog
	logs   *logView     // live log viewer (modeLogs)
	launch *launchForm  // Android launch options dialog (modeLaunch)
	rename *renameForm  // rename dialog (modeRename)
}

// New returns a Model that refreshes every 2 s while idle. version is shown
// in the header line.
func New(ctx context.Context, reg *device.Registry, version string, prefs state.Store, run device.Runner, env host.Env) Model {
	return Model{reg: reg, ctx: ctx, tickEvery: 2 * time.Second, version: version, prefs: prefs, run: run, env: env}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.tick())
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg { return listMsg{m.reg.ListAll(m.ctx)} }
}

func (m Model) tick() tea.Cmd {
	if m.tickEvery == 0 {
		return nil
	}
	return tea.Tick(m.tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// panelGroup is one bordered panel: a platform's virtual devices, or every
// physical device across platforms.
type panelGroup struct {
	title   string
	devices []device.Device
	note    string // shown instead of rows when there are none (styled)
}

// groups returns the panels in display order: iOS Simulators, Android
// Emulators, Physical Devices. The flat cursor follows the same order.
func (m Model) groups() []panelGroup {
	var out []panelGroup
	var physical []device.Device
	for _, pl := range m.lists {
		g := panelGroup{title: panelTitle(pl.Platform)}
		switch {
		case !pl.Available:
			g.note = warnStyle.Render("unavailable: " + pl.Reason)
		case pl.Err != nil:
			g.note = failStyle.Render(pl.Err.Error())
		}
		for _, d := range pl.Devices {
			if d.Kind == device.Physical {
				physical = append(physical, d)
			} else {
				g.devices = append(g.devices, d)
			}
		}
		if g.note == "" && len(g.devices) == 0 {
			g.note = dimStyle.Render("No devices found")
		}
		out = append(out, g)
	}
	pg := panelGroup{title: "Physical Devices", devices: physical}
	if len(physical) == 0 {
		pg.note = dimStyle.Render("No devices connected (USB, or w for wireless ADB)")
	}
	return append(out, pg)
}

// devices returns the selectable rows in display order.
func (m Model) devices() []device.Device {
	var out []device.Device
	for _, g := range m.groups() {
		out = append(out, g.devices...)
	}
	return out
}

// nextPanelStart returns the flat index of the first device in the panel after
// the one holding the cursor, wrapping around. Empty panels are skipped.
func (m Model) nextPanelStart() int {
	var starts []int
	first := 0
	for _, g := range m.groups() {
		if len(g.devices) > 0 {
			starts = append(starts, first)
		}
		first += len(g.devices)
	}
	for _, st := range starts {
		if st > m.cursor {
			return st
		}
	}
	if len(starts) > 0 {
		return starts[0]
	}
	return 0
}

func (m Model) selected() (device.Device, device.Provider, bool) {
	devs := m.devices()
	if m.cursor < 0 || m.cursor >= len(devs) {
		return device.Device{}, nil, false
	}
	d := devs[m.cursor]
	p, ok := m.reg.Provider(d.Platform)
	return d, p, ok
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case listMsg:
		m.lists, m.loaded = msg.lists, true
		if n := len(m.devices()); m.cursor >= n {
			m.cursor = max(n-1, 0)
		}
		return m, nil
	case tickMsg:
		if m.mode == modeList && !m.busy {
			return m, tea.Batch(m.load(), m.tick())
		}
		return m, m.tick()
	case stageMsg:
		m.stages = upsertStage(m.stages, msg.stage)
		return m, waitEvent(m.events)
	case logBatchMsg:
		if m.logs != nil {
			m.logs.ring.Add(msg.lines...)
			return m, waitEvent(m.logs.batches)
		}
		return m, nil
	case logEndMsg:
		if m.logs != nil {
			m.logs.ended = true
			if msg.err != nil {
				m.logs.status = msg.err.Error()
			}
		}
		return m, nil
	case renameDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "rename: " + msg.err.Error()
		} else {
			m.status = "renamed to " + msg.name
		}
		return m, m.load()
	case runDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "run: " + msg.err.Error()
		} else {
			m.status = "run: " + msg.detail
		}
		return m, m.load()
	case wirelessMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "wireless: " + msg.err.Error()
		} else {
			m.status = "wireless: " + msg.detail
		}
		return m, m.load()
	case opDoneMsg:
		m.busy = false
		if m.mode == modeProgress {
			m.done, m.opErr = true, msg.err
		} else if msg.err != nil {
			m.status = msg.op + " failed: " + msg.err.Error()
		} else {
			m.status = m.target.Name + ": " + msg.op + " done"
		}
		return m, m.load()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == "ctrl+c" {
		return m, tea.Quit
	}
	if m.help {
		m.help = false // any key closes the overlay
		return m, nil
	}
	switch m.mode {
	case modeConfirm:
		switch s {
		case "y", "Y":
			return m.startOp(m.confirmOp)
		default:
			m.mode, m.status = modeList, m.confirmOp+" cancelled"
		}
		return m, nil
	case modeRename:
		return m.renameKey(k)
	case modeLaunch:
		return m.launchKey(k)
	case modeLogs:
		return m.logsKey(k)
	case modeWireless:
		return m.wirelessKey(k)
	case modePicker:
		switch s {
		case "up", "k":
			if m.pickCursor > 0 {
				m.pickCursor--
			}
		case "down", "j":
			if m.pickCursor < len(m.pick)-1 {
				m.pickCursor++
			}
		case " ", "space":
			if len(m.pick) > 0 {
				m.pick[m.pickCursor].keep = !m.pick[m.pickCursor].keep
			}
		case "enter":
			var err error
			if m, err = m.applyPicker(); err != nil {
				m.mode, m.status = modeList, "save preference: "+err.Error()
				return m, nil
			}
			m.mode = modeList
			return m.startOp("boot")
		case "esc", "q", "c":
			m.mode = modeList
		}
		return m, nil
	case modeProgress:
		if m.done && (s == "enter" || s == "esc" || s == "q") {
			m.mode, m.stages, m.done, m.opErr = modeList, nil, false, nil
			return m, m.load()
		}
		return m, nil
	}
	switch s {
	case "q":
		return m, tea.Quit
	case "?":
		m.help = true
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.devices())-1 {
			m.cursor++
		}
	case "tab":
		m.cursor = m.nextPanelStart()
	case "enter", "r", "t", "c", "d":
		if m.busy {
			return m, nil
		}
		d, _, ok := m.selected()
		if !ok {
			return m, nil
		}
		if d.Kind == device.Physical {
			m.status = d.Name + " is a physical device; Sthin never modifies physical devices"
			return m, nil
		}
		switch s {
		case "enter":
			return m.startOp("boot")
		case "r":
			m.target, m.mode, m.confirmOp = d, modeConfirm, "restore"
		case "d":
			m.target, m.mode, m.confirmOp = d, modeConfirm, "delete"
		case "t":
			return m.startOp("shutdown")
		case "c":
			m = m.openPicker(d)
		}
	case "w":
		if !m.busy {
			m = m.openWireless()
		}
	case "x":
		if !m.busy {
			return m.runApp()
		}
	case "l":
		if d, p, ok := m.selected(); ok && !m.busy {
			return m.openLogs(d, p)
		}
	case "o":
		if d, _, ok := m.selected(); ok && !m.busy {
			m = m.openLaunchOptions(d)
		}
	case "n":
		if d, p, ok := m.selected(); ok && !m.busy {
			m = m.openRename(d, p)
		}
	}
	return m, nil
}

// startOp runs boot, restore, or shutdown on the selected device in the background,
// streaming stage reports back as messages.
func (m Model) startOp(op string) (tea.Model, tea.Cmd) {
	d, p, ok := m.selected()
	if !ok {
		m.mode = modeList
		return m, nil
	}
	m.target, m.op, m.busy = d, op, true
	m.stages, m.done, m.opErr = nil, false, nil
	events := make(chan tea.Msg, 64)
	m.events = events
	report := func(s device.Stage) { events <- stageMsg{s} }
	ctx := m.ctx
	var run func() error
	switch op {
	case "boot":
		m.mode = modeProgress
		run = func() error { return p.Boot(ctx, d.ID, device.BootOptions{}, report) }
	case "restore":
		m.mode = modeProgress
		run = func() error { return p.Restore(ctx, d.ID, report) }
	case "delete":
		del, ok := p.(device.Deleter)
		if !ok {
			m.busy, m.events = false, nil
			m.mode, m.status = modeList, string(d.Platform)+" provider cannot delete devices"
			return m, nil
		}
		m.mode, m.status = modeList, "deleting "+d.Name+"…"
		run = func() error { return del.Delete(ctx, d.ID) }
	default:
		m.mode, m.status = modeList, "shutting down "+d.Name+"…"
		run = func() error { return p.Shutdown(ctx, d.ID) }
	}
	go func() {
		err := run()
		events <- opDoneMsg{op: op, err: err}
		close(events)
	}()
	return m, waitEvent(events)
}

// waitEvent delivers the next message from a running operation.
func waitEvent(ch chan tea.Msg) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// upsertStage replaces the running entry of a stage with its result.
func upsertStage(ss []device.Stage, s device.Stage) []device.Stage {
	if n := len(ss); n > 0 && ss[n-1].Name == s.Name && ss[n-1].Status == device.StageRunning {
		ss[n-1] = s
		return ss
	}
	return append(ss, s)
}
