package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/logs"
)

// The log viewer: `l` on a device follows its log live. Lines arrive from the
// Provider's LogStream, are batched by a collector goroutine so a chatty
// device never re-renders per line, and land in a bounded ring buffer.

const (
	logRingMax   = 50000
	logBatchTick = 50 * time.Millisecond
	logBatchMax  = 2000
)

type logView struct {
	target  device.Device
	ring    *logs.Ring
	filter  logs.Filter
	batches chan tea.Msg
	cancel  context.CancelFunc
	follow  bool
	scroll  int  // lines from the bottom when not following
	typing  bool // editing the text filter
	ended   bool
	status  string
}

type logBatchMsg struct{ lines []logs.Line }
type logEndMsg struct{ err error }

// openLogs starts the stream for the selected device and switches to modeLogs.
func (m Model) openLogs(d device.Device, p device.Provider) (tea.Model, tea.Cmd) {
	ctl, ok := p.(device.Controller)
	if !ok {
		m.status = "provider cannot read logs"
		return m, nil
	}
	if d.Kind != device.Physical && d.State != device.Booted {
		m.status = "boot " + d.Name + " first (enter), then l for logs"
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	ch, err := ctl.LogStream(ctx, d.ID)
	if err != nil {
		cancel()
		m.status = "logs: " + err.Error()
		return m, nil
	}
	lv := &logView{target: d, ring: &logs.Ring{Max: logRingMax}, batches: make(chan tea.Msg, 64), cancel: cancel, follow: true}
	go collectLogs(ch, lv.batches)
	m.logs, m.mode = lv, modeLogs
	return m, waitEvent(lv.batches)
}

// collectLogs batches stream lines by time or size, then reports the end.
func collectLogs(ch <-chan logs.Line, out chan<- tea.Msg) {
	var buf []logs.Line
	flush := func() {
		if len(buf) > 0 {
			out <- logBatchMsg{lines: buf}
			buf = nil
		}
	}
	t := time.NewTicker(logBatchTick)
	defer t.Stop()
	for {
		select {
		case l, ok := <-ch:
			if !ok {
				flush()
				out <- logEndMsg{}
				close(out)
				return
			}
			buf = append(buf, l)
			if len(buf) >= logBatchMax {
				flush()
			}
		case <-t.C:
			flush()
		}
	}
}

func (m Model) closeLogs() Model {
	if m.logs != nil {
		m.logs.cancel()
	}
	m.logs, m.mode = nil, modeList
	return m
}

// logsKey handles keys in modeLogs.
func (m Model) logsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	lv := m.logs
	s := k.String()
	if lv.typing {
		switch s {
		case "enter", "esc":
			lv.typing = false
		case "backspace":
			if r := []rune(lv.filter.Text); len(r) > 0 {
				lv.filter.Text = string(r[:len(r)-1])
			}
		default:
			if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
				lv.filter.Text += string(k.Runes)
			}
		}
		return m, nil
	}
	switch s {
	case "esc", "q":
		return m.closeLogs(), nil
	case "/":
		lv.typing = true
	case "L":
		lv.filter.MinLevel = (lv.filter.MinLevel + 1) % (logs.Fatal + 1)
	case "f":
		lv.follow = !lv.follow
		if lv.follow {
			lv.scroll = 0
		}
	case "c":
		lv.ring.Clear()
		lv.scroll = 0
	case "s":
		path, err := m.saveLogs()
		if err != nil {
			lv.status = "save: " + err.Error()
		} else {
			lv.status = "saved " + path
		}
	case "up", "k":
		lv.follow = false
		lv.scroll++
	case "down", "j":
		if lv.scroll > 0 {
			lv.scroll--
		}
	case "pgup":
		lv.follow = false
		lv.scroll += 20
	case "pgdown":
		lv.scroll = max(lv.scroll-20, 0)
	case "end", "G":
		lv.follow, lv.scroll = true, 0
	}
	return m, nil
}

// saveLogs writes the filtered buffer to $STHIN_HOME/logs/<id>-<time>.log.
func (m Model) saveLogs() (string, error) {
	lv := m.logs
	lines, _ := lv.ring.Snapshot(lv.filter)
	dir := filepath.Join(m.prefs.Home, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, sanitizeID(lv.target.ID)+"-"+time.Now().Format("20060102-150405")+".log")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(logs.Format(l))
		b.WriteByte('\n')
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func sanitizeID(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == ':' || r == ' ' {
			return '_'
		}
		return r
	}, s)
}

// logsLines renders the visible window plus a status footer for the panel.
func (m Model) logsLines(w, innerH int) []string {
	lv := m.logs
	lines, total := lv.ring.Snapshot(lv.filter)
	viewH := max(innerH-1, 1) // last row is the footer
	end := len(lines) - lv.scroll
	if end < 0 {
		end = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	start := max(end-viewH, 0)
	out := make([]string, 0, viewH+1)
	for _, l := range lines[start:end] {
		txt := fit(logs.Format(l), w-2)
		switch {
		case l.Level >= logs.Error:
			txt = failStyle.Render(txt)
		case l.Level == logs.Warn:
			txt = warnStyle.Render(txt)
		case l.Level <= logs.Debug:
			txt = dimStyle.Render(txt)
		}
		out = append(out, txt)
	}
	for len(out) < viewH {
		out = append(out, "")
	}
	filterTxt := lv.filter.Text
	if lv.typing {
		filterTxt = selStyle.Render(filterTxt + " ")
	} else if filterTxt == "" {
		filterTxt = dimStyle.Render("(none)")
	}
	mode := "follow"
	if !lv.follow {
		mode = fmt.Sprintf("paused ↑%d", lv.scroll)
	}
	if lv.ended {
		mode = "ended"
	}
	foot := fmt.Sprintf(" %d/%d · level≥%s · filter %s · %s", len(lines), total, lv.filter.MinLevel, filterTxt, mode)
	if lv.status != "" {
		foot += " · " + lv.status
	}
	foot += dimStyle.Render("   / filter  L level  f follow  s save  c clear  esc close")
	return append(out, fit(foot, w-2))
}
