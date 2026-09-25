package serve

import (
	"context"
	"strings"
	"time"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/logs"
	"github.com/wktorbert/lean-sim/internal/ops"
)

type logsIn struct {
	ID    string `json:"id"`
	Lines int    `json:"lines,omitempty"`
}

// logs returns a snapshot of the last lines, parsed into the normalised form.
// Filtering is the client's job so one snapshot serves every filter setting.
func (s *Server) logs(r *request) (any, error) {
	var in logsIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	p, d, err := s.Reg.Resolve(r.ctx, in.ID)
	if err != nil {
		return nil, err
	}
	c, err := ops.Controller(p)
	if err != nil {
		return nil, err
	}
	text, err := c.Logs(r.ctx, d.ID, in.Lines)
	if err != nil {
		return nil, err
	}
	parse := logs.ParseLogcat
	if d.Platform == device.IOS {
		parse = logs.ParseIOS
	}
	out := []logs.Line{}
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line != "" {
			out = append(out, parse(line))
		}
	}
	return map[string]any{"device": d.ID, "lines": out}, nil
}

// logsFollow starts a live stream. It answers at once, then sends `log_lines`
// notifications carrying batches of lines (every LogFlush or LogBatch lines,
// whichever comes first) until cancelled or the device stops. A per-line
// message would cost one JSON-RPC message per line at thousands of lines a
// second; batching keeps the pipe and the webview responsive.
func (s *Server) logsFollow(r *request) (any, error) {
	var in idIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	p, d, err := s.Reg.Resolve(r.ctx, in.ID)
	if err != nil {
		return nil, err
	}
	c, err := ops.Controller(p)
	if err != nil {
		return nil, err
	}
	ch, err := c.LogStream(r.ctx, d.ID)
	if err != nil {
		return nil, err
	}
	r.keep(func(ctx context.Context) { s.pumpLogs(ctx, r, ch) })
	return map[string]any{"device": d.ID, "following": true}, nil
}

func (s *Server) pumpLogs(ctx context.Context, r *request, ch <-chan logs.Line) {
	batch := make([]logs.Line, 0, s.LogBatch)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		r.notify("log_lines", map[string]any{"lines": batch})
		batch = make([]logs.Line, 0, s.LogBatch)
	}
	t := time.NewTicker(s.LogFlush)
	defer t.Stop()
	for {
		select {
		case l, ok := <-ch:
			if !ok {
				flush()
				reason := "stream ended"
				if ctx.Err() != nil {
					reason = "cancelled"
				}
				r.end(reason)
				return
			}
			batch = append(batch, l)
			if len(batch) >= s.LogBatch {
				flush()
			}
		case <-t.C:
			flush()
		case <-ctx.Done():
			flush()
			r.end("cancelled")
			return
		}
	}
}
