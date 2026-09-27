package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/ops"
)

// initialize is the handshake: the client learns the schema version, the
// sidecar's version, and which platforms this host can serve (with the reason
// when one cannot, such as iOS on Windows).
func (s *Server) initialize(r *request) (any, error) {
	var in struct {
		ClientName    string `json:"client_name"`
		ClientVersion string `json:"client_version"`
	}
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	type platform struct {
		Platform  device.Platform `json:"platform"`
		Available bool            `json:"available"`
		Reason    string          `json:"reason,omitempty"`
	}
	platforms := []platform{}
	for _, p := range s.Reg.Providers {
		ok, reason := p.Available()
		platforms = append(platforms, platform{Platform: p.Platform(), Available: ok, Reason: reason})
	}
	return map[string]any{
		"schema_version": SchemaVersion,
		"sthin_version":  s.Version,
		"os":             runtime.GOOS,
		"platforms":      platforms,
	}, nil
}

type platformIn struct {
	Platform string `json:"platform,omitempty"`
}

func (s *Server) devicesList(r *request) (any, error) {
	var in platformIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	return ops.List(r.ctx, s.Reg, s.Pool, in.Platform), nil
}

// devicesSubscribe answers with the current listing, then re-lists every
// PollInterval while the subscription lives and sends a `devices` notification
// whenever the listing changed. `cancel` with this request's id stops it.
func (s *Server) devicesSubscribe(r *request) (any, error) {
	var in platformIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	first := ops.List(r.ctx, s.Reg, s.Pool, in.Platform)
	last, _ := json.Marshal(first)
	r.keep(func(ctx context.Context) {
		t := time.NewTicker(s.PollInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				r.end("cancelled")
				return
			case <-t.C:
			}
			cur := ops.List(ctx, s.Reg, s.Pool, in.Platform)
			b, _ := json.Marshal(cur)
			if bytes.Equal(b, last) {
				continue
			}
			last = b
			r.notify("devices", map[string]any{"platforms": cur.Platforms, "devices": cur.Devices})
		}
	})
	return first, nil
}
