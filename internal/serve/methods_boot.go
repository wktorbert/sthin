package serve

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/ops"
	"github.com/wktorbert/sthin/internal/state"
)

type idIn struct {
	ID string `json:"id"`
}

func (in idIn) check() error {
	if strings.TrimSpace(in.ID) == "" {
		return invalidParams("id is required")
	}
	return nil
}

type bootIn struct {
	ID       string   `json:"id"`
	Stock    bool     `json:"stock,omitempty"`
	Except   []string `json:"except"` // absent/null: the device's saved preference
	Remember bool     `json:"remember,omitempty"`
	RAMMB    int      `json:"ram_mb,omitempty"`
	Headless bool     `json:"headless,omitempty"`
	// Android launch switches; absent fields use the AVD's saved preference.
	ColdBoot *bool  `json:"cold_boot,omitempty"`
	Audio    *bool  `json:"audio,omitempty"`
	LowRAM   *bool  `json:"lowram,omitempty"`
	Name     string `json:"name,omitempty"` // rename before booting
}

type renameIn struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// stageFailure is the error data attached to a failed boot or restore, so the
// client can show which stage failed and the command behind it.
type stageFailure struct {
	Stage   string         `json:"stage,omitempty"`
	Command string         `json:"command,omitempty"`
	Stages  []device.Stage `json:"stages"`
}

func failure(err error, stages []device.Stage) error {
	e := toError(err)
	d := stageFailure{Stages: stages}
	var se *device.StageError
	if errors.As(err, &se) {
		d.Stage, d.Command = se.Stage, se.Command
	}
	e.Data = d
	return e
}

// boot slim boots (or stock boots) a virtual device, streaming every stage as a
// `progress` notification before the final result.
func (s *Server) boot(r *request) (any, error) {
	var in bootIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	if in.RAMMB < 0 {
		return nil, invalidParams("ram_mb must be positive")
	}
	p, d, err := ops.ResolveVirtual(r.ctx, s.Reg, in.ID, "boot")
	if err != nil {
		return nil, err
	}
	if in.Remember {
		except := in.Except
		if except == nil {
			except = []string{}
		}
		if err := s.Prefs.SavePrefs(state.Prefs{ID: d.ID, Except: except}); err != nil {
			return nil, err
		}
	}
	opts := device.BootOptions{Stock: in.Stock, Except: in.Except, RAMMB: in.RAMMB, Headless: in.Headless, Launch: device.LaunchOptions{ColdBoot: in.ColdBoot, Audio: in.Audio, LowRAM: in.LowRAM}, Name: in.Name}
	res := ops.RunBoot(r.ctx, p, d, opts, r.progress)
	if res.Err != nil {
		if r.ctx.Err() != nil {
			return nil, r.ctx.Err()
		}
		return nil, failure(res.Err, res.Stages)
	}
	return res, nil
}

func (s *Server) restore(r *request) (any, error) {
	var in idIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	p, d, err := ops.ResolveVirtual(r.ctx, s.Reg, in.ID, "restore")
	if err != nil {
		return nil, err
	}
	stages := []device.Stage{}
	err = p.Restore(r.ctx, d.ID, func(st device.Stage) {
		r.progress(st)
		if st.Status != device.StageRunning {
			stages = append(stages, st)
		}
	})
	if err != nil {
		if r.ctx.Err() != nil {
			return nil, r.ctx.Err()
		}
		return nil, failure(err, stages)
	}
	return map[string]any{"device": d.ID, "stages": stages}, nil
}

func (s *Server) shutdown(r *request) (any, error) {
	var in idIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	p, d, err := ops.ResolveVirtual(r.ctx, s.Reg, in.ID, "shutdown")
	if err != nil {
		return nil, err
	}
	if err := p.Shutdown(r.ctx, d.ID); err != nil {
		return nil, err
	}
	return map[string]any{"device": d.ID}, nil
}

// rename sets a simulator's or AVD's display name.
func (s *Server) rename(r *request) (any, error) {
	var in renameIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if in.ID == "" || in.Name == "" {
		return nil, fmt.Errorf("%w: id and name are required", device.ErrUsage)
	}
	p, d, err := ops.ResolveVirtual(r.ctx, s.Reg, in.ID, "rename")
	if err != nil {
		return nil, err
	}
	rn, ok := p.(device.Renamer)
	if !ok {
		return nil, fmt.Errorf("%s provider cannot rename devices", d.Platform)
	}
	if err := rn.Rename(r.ctx, d.ID, in.Name); err != nil {
		return nil, err
	}
	return map[string]any{"device": d.ID, "name": in.Name}, nil
}

// deleteDevice removes a shut-down simulator or AVD for good, with Sthin's
// records and any lease on it.
func (s *Server) deleteDevice(r *request) (any, error) {
	var in idIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	p, d, err := ops.ResolveVirtual(r.ctx, s.Reg, in.ID, "delete")
	if err != nil {
		return nil, err
	}
	if err := ops.Delete(r.ctx, p, d, s.Pool); err != nil {
		return nil, err
	}
	return map[string]any{"device": d.ID}, nil
}

func (s *Server) measure(r *request) (any, error) {
	var in idIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	p, d, err := ops.ResolveVirtual(r.ctx, s.Reg, in.ID, "measure")
	if err != nil {
		return nil, err
	}
	return p.Measure(r.ctx, d.ID)
}
