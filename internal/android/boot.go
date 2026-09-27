package android

import (
	"context"
	"fmt"
	"time"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/state"
)

// Boot implements device.Provider: tune, launch, wait for boot, then slim the guest.
func (p *Provider) Boot(ctx context.Context, id string, opts device.BootOptions, r device.Reporter) error {
	a, err := p.findAVD(id)
	if err != nil {
		return err
	}
	if opts.Name != "" {
		if err := (device.Stages{R: r}).Do(stageRename, func() (string, error) {
			return opts.Name, p.Rename(ctx, id, opts.Name)
		}); err != nil {
			return err
		}
	}
	prof := p.profile()
	except, err := p.State.ExceptFor(id, opts.Except)
	if err != nil {
		return err
	}
	if err := prof.CheckExcept(except); err != nil {
		return fmt.Errorf("%w: %v", device.ErrUsage, err)
	}
	launch, err := p.State.LaunchFor(id, opts.Launch)
	if err != nil {
		return err
	}
	ram := opts.RAMMB
	if ram == 0 {
		ram = device.Int(launch.RAMMB, defaultRAMMB)
	}
	sw := launchSwitches{
		headless: opts.Headless || device.Bool(launch.Headless, false),
		coldBoot: device.Bool(launch.ColdBoot, false),
		audio:    device.Bool(launch.Audio, false),
		lowRAM:   device.Bool(launch.LowRAM, false),
	}
	st := device.Stages{R: r}
	_, running := p.running(ctx)[id]

	if opts.Stock {
		if _, ok, _ := p.State.Load(id); ok {
			if err := p.Restore(ctx, id, r); err != nil {
				return err
			}
			running = false // Restore kills a running emulator
		}
		st.Skip(stageTune, "stock boot leaves config.ini untouched")
	} else {
		if bad, why := is16k(a.Config); bad {
			return st.Do(stageTune, func() (string, error) { return "", fmt.Errorf("%s", why) })
		}
		if running {
			st.Skip(stageTune, "emulator already running; config.ini applies at its next cold boot")
		} else if err := st.Do(stageTune, func() (string, error) { return p.tuneAndRecord(a, ram, sw.audio, prof.Version) }); err != nil {
			return err
		}
	}

	if running {
		st.Skip(stageLaunch, "already running")
	} else if err := st.Do(stageLaunch, func() (string, error) { return p.launch(ctx, id, ram, opts.Stock, sw) }); err != nil {
		return err
	}
	var serial string
	if err := st.Do(stageWaitBoot, func() (string, error) {
		s, err := p.waitBoot(ctx, id)
		serial = s
		return s, err
	}); err != nil {
		return err
	}
	if !opts.Stock {
		if err := p.slimGuest(ctx, st, id, serial, except); err != nil {
			return err
		}
	}
	p.measureStage(ctx, st, id)
	return nil
}

// tuneAndRecord tunes config.ini and records the backup in the Change record.
func (p *Provider) tuneAndRecord(a avd, ram int, audio bool, profileVersion int) (string, error) {
	backup, err := tune(a.Dir, ram, audio)
	if err != nil {
		return "", err
	}
	rec, _, err := p.State.Load(a.Name)
	if err != nil {
		return "", err
	}
	rec.ID, rec.Platform, rec.AppliedAt, rec.ProfileVersion = a.Name, string(device.Android), time.Now().UTC(), profileVersion
	rec.ConfigBackup = backup
	if rec.Disabled == nil {
		rec.Disabled = []string{}
	}
	if rec.Settings == nil {
		rec.Settings = []state.Setting{}
	}
	if err := p.State.Save(rec); err != nil {
		return "", err
	}
	audioTxt := "audio off"
	if audio {
		audioTxt = "audio on"
	}
	return fmt.Sprintf("hw.ramSize=%d, gpu host, %s, cameras off (backup %s)", ram, audioTxt, backup), nil
}
