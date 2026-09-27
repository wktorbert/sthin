package android

import (
	"context"
	"fmt"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/state"
)

// Restore implements device.Provider: replay the Change record in reverse.
// Package and settings replay needs a running guest, so a stopped AVD with
// recorded guest changes is booted stock for the replay and shut down after.
func (p *Provider) Restore(ctx context.Context, id string, r device.Reporter) error {
	a, err := p.findAVD(id)
	if err != nil {
		return err
	}
	st := device.Stages{R: r}
	rec, ok, err := p.State.Load(id)
	if err != nil {
		return err
	}
	if !ok {
		st.Skip(stageUnchanged, "no change record; nothing to restore")
		return nil
	}
	if err := st.Do(stageUntune, func() (string, error) {
		restored, err := untune(a.Dir)
		if !restored {
			return "no config.ini backup", err
		}
		return "config.ini restored from backup", err
	}); err != nil {
		return err
	}

	serial, running := p.running(ctx)[id]
	needsGuest := len(rec.Disabled) > 0 || len(applied(rec.Settings)) > 0
	if needsGuest && !running {
		if err := st.Do(stageLaunch, func() (string, error) { return p.launch(ctx, id, 0, true, launchSwitches{}) }); err != nil {
			return err
		}
		if err := st.Do(stageWaitBoot, func() (string, error) {
			s, err := p.waitBoot(ctx, id)
			serial = s
			return s, err
		}); err != nil {
			return err
		}
		running = true
	}
	if needsGuest {
		if err := st.Do(stageEnable, func() (string, error) { return p.enablePackages(ctx, serial, rec.Disabled) }); err != nil {
			return err
		}
		if err := st.Do(stageSettings, func() (string, error) { return p.restoreSettings(ctx, serial, rec.Settings) }); err != nil {
			return err
		}
	}
	if err := p.State.Delete(id); err != nil {
		return err
	}
	if running {
		return st.Do(stageKill, func() (string, error) {
			if needsGuest {
				if err := p.waitPersist(ctx); err != nil {
					return "", err
				}
			}
			how, err := p.powerOff(ctx, serial)
			return how + "; next boot is a cold stock boot", err
		})
	}
	return nil
}

func applied(ss []state.Setting) []state.Setting {
	var out []state.Setting
	for _, s := range ss {
		if !s.Skipped {
			out = append(out, s)
		}
	}
	return out
}

func (p *Provider) enablePackages(ctx context.Context, serial string, pkgs []string) (string, error) {
	var failed []string
	for _, pkg := range pkgs {
		if _, err := p.shell(ctx, serial, "pm", "enable", "--user", "0", pkg); err != nil {
			failed = append(failed, pkg)
		}
	}
	if len(failed) > 0 {
		return "", &device.StageError{Command: p.Adb + " -s " + serial + " shell pm enable --user 0 " + failed[0],
			Err: fmt.Errorf("%d of %d packages failed to re-enable", len(failed), len(pkgs))}
	}
	return fmt.Sprintf("re-enabled %d packages", len(pkgs)), nil
}

func (p *Provider) restoreSettings(ctx context.Context, serial string, ss []state.Setting) (string, error) {
	n := 0
	for _, s := range applied(ss) {
		var err error
		if s.Prior == nil {
			_, err = p.shell(ctx, serial, "settings", "delete", s.Scope, s.Key)
		} else {
			_, err = p.shell(ctx, serial, "settings", "put", s.Scope, s.Key, *s.Prior)
		}
		if err != nil {
			return "", &device.StageError{Command: p.Adb + " -s " + serial + " shell settings … " + s.Key, Err: err}
		}
		n++
	}
	return fmt.Sprintf("restored %d settings", n), nil
}
