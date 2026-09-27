package ios

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/profile"
	"github.com/wktorbert/sthin/internal/state"
)

const (
	defaultPoll       = time.Second
	launchdWaitCap    = 120 * time.Second
	missingTolerance  = 0.10 // readback may miss up to 10 % of Sthin's labels before the fallback runs
	stageRename       = "rename"
	stageCheckRuntime = "check-runtime"
	stageWrite        = "write-overrides"
	stageBoot         = "boot"
	stageOpen         = "open-simulator"
	stageWaitLaunchd  = "wait-launchd"
	stageVerify       = "verify"
	stageMeasure      = "measure"
	stageShutdown     = "shutdown"
)

// cmd runs xcrun simctl args; a failure becomes a StageError naming the command.
func (p *Provider) cmd(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"simctl"}, args...)
	out, stderr, err := p.Run.Run(ctx, "xcrun", full...)
	if err != nil {
		return string(out), &device.StageError{
			Command: "xcrun " + strings.Join(full, " "),
			Err:     fmt.Errorf("%v: %s", err, strings.TrimSpace(string(stderr))),
		}
	}
	return string(out), nil
}

func (p *Provider) poll() time.Duration {
	if p.PollInterval > 0 {
		return p.PollInterval
	}
	return defaultPoll
}

// waitLaunchd polls `launchctl print system` inside the simulator until it answers.
func (p *Provider) waitLaunchd(ctx context.Context, udid string) (string, error) {
	start := time.Now()
	limit := p.WaitCap
	if limit == 0 {
		limit = launchdWaitCap
	}
	var last error
	for attempt := 1; ; attempt++ {
		if _, err := p.cmd(ctx, "spawn", udid, "launchctl", "print", "system"); err == nil {
			return fmt.Sprintf("launchd up after %d attempt(s)", attempt), nil
		} else {
			last = err
		}
		if time.Since(start) >= limit || (p.MaxPolls > 0 && attempt >= p.MaxPolls) {
			return "", fmt.Errorf("launchd did not answer within %s: %w", limit, last)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.poll()):
		}
	}
}

// readbackDisabled returns the labels launchd reports as disabled.
func (p *Provider) readbackDisabled(ctx context.Context, udid string) (map[string]bool, error) {
	out, err := p.cmd(ctx, "spawn", udid, "launchctl", "print-disabled", "system")
	if err != nil {
		return nil, err
	}
	return parsePrintDisabled(out), nil
}

// parsePrintDisabled parses lines like `"com.apple.x" => disabled` (or `=> true`).
func parsePrintDisabled(out string) map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		label, val, ok := strings.Cut(strings.TrimSpace(line), "=>")
		if !ok {
			continue
		}
		label = strings.Trim(strings.TrimSpace(label), `"`)
		val = strings.TrimSpace(val)
		if val == "disabled" || val == "true" {
			m[label] = true
		}
	}
	return m
}

func missingFrom(want []string, have map[string]bool) []string {
	var miss []string
	for _, l := range want {
		if !have[l] {
			miss = append(miss, l)
		}
	}
	return miss
}

func tooMany(missing, total int) bool {
	return total > 0 && float64(missing) > missingTolerance*float64(total)
}

// Boot implements device.Provider: a slim boot (or stock with opts.Stock).
func (p *Provider) Boot(ctx context.Context, udid string, opts device.BootOptions, r device.Reporter) error {
	s, err := p.find(ctx, udid)
	if err != nil {
		return err
	}
	if opts.Name != "" {
		if err := (device.Stages{R: r}).Do(stageRename, func() (string, error) {
			return opts.Name, p.Rename(ctx, udid, opts.Name)
		}); err != nil {
			return err
		}
		s.Name = opts.Name
	}
	prof := p.profile()
	except, err := p.State.ExceptFor(udid, opts.Except)
	if err != nil {
		return err
	}
	if err := prof.CheckExcept(except); err != nil {
		return fmt.Errorf("%w: %v", device.ErrUsage, err)
	}
	sr := device.Stages{R: r}
	booted := mapState(s.State) == device.Booted

	stock := opts.Stock
	if !stock && !persistsOverrides(s.Version) {
		sr.Warn(stageCheckRuntime, fmt.Sprintf("iOS %s does not persist overrides (needs %s+); booting stock", s.Version, minPersistVersion))
		stock = true
	} else if err := sr.Do(stageCheckRuntime, func() (string, error) { return "iOS " + s.Version, nil }); err != nil {
		return err
	}

	if stock {
		if _, ok, _ := p.State.Load(udid); ok {
			// Restore reboots a booted device and leaves a shut-down one shut down.
			if err := p.Restore(ctx, udid, r); err != nil {
				return err
			}
		}
		if !booted {
			if err := p.bootAndWait(ctx, sr, udid); err != nil {
				return err
			}
		}
		p.openSimulator(ctx, sr, udid, opts.Headless)
		p.measureStage(ctx, sr, udid)
		return nil
	}

	desired := profile.DesiredSet(prof, except)
	if err := sr.Do(stageWrite, func() (string, error) {
		return p.writeOverrides(ctx, udid, booted, desired, prof.Version)
	}); err != nil {
		return err
	}
	if err := p.bootAndWait(ctx, sr, udid); err != nil {
		return err
	}
	if err := sr.Do(stageVerify, func() (string, error) { return p.verifyOverrides(ctx, udid, desired) }); err != nil {
		return err
	}
	p.openSimulator(ctx, sr, udid, opts.Headless)
	p.measureStage(ctx, sr, udid)
	return nil
}

// openSimulator shows the device in Simulator.app: `simctl boot` alone runs
// the device headless, so without this the user sees nothing. Failure to open
// the window only warns; the device is booted either way.
func (p *Provider) openSimulator(ctx context.Context, sr device.Stages, udid string, headless bool) {
	if headless {
		sr.Skip(stageOpen, "headless")
		return
	}
	apps := p.Env.SimulatorApps()
	if len(apps) == 0 {
		// No known bundle on disk: let LaunchServices resolve by name.
		apps = []string{"Simulator", "DeviceHub"}
	}
	var last error
	for _, app := range apps {
		_, stderr, err := p.Run.Run(ctx, "open", "-a", app, "--args", "-CurrentDeviceUDID", udid)
		if err == nil {
			sr.R.Report(device.Stage{Name: stageOpen, Status: device.StageOK, Detail: filepath.Base(app)})
			return
		}
		last = fmt.Errorf("%s: %v %s", filepath.Base(app), err, strings.TrimSpace(string(stderr)))
	}
	sr.Warn(stageOpen, "no simulator window app found ("+last.Error()+"); Xcode 26 ships Simulator.app, Xcode 27 ships DeviceHub.app")
}

// writeOverrides shuts a booted simulator down, writes the store, and records the change.
func (p *Provider) writeOverrides(ctx context.Context, udid string, booted bool, desired []string, profileVersion int) (string, error) {
	if booted {
		if _, err := p.cmd(ctx, "shutdown", udid); err != nil {
			return "", err
		}
	}
	st := p.store(udid)
	cur, err := st.Read()
	if err != nil {
		return "", err
	}
	rec, _, err := p.State.Load(udid)
	if err != nil {
		return "", err
	}
	want := map[string]bool{}
	var toDisable []string
	for _, l := range desired {
		want[l] = true
		if !cur[l] {
			toDisable = append(toDisable, l)
		}
	}
	// Labels Sthin disabled earlier that the current selection keeps (a category
	// newly excepted) are switched back on; labels Sthin never touched are not.
	var toEnable []string
	for _, l := range rec.Disabled {
		if !want[l] && cur[l] {
			toEnable = append(toEnable, l)
		}
	}
	if len(toDisable) == 0 && len(toEnable) == 0 {
		return fmt.Sprintf("already slim (%d labels disabled)", len(desired)), nil
	}
	rec.ID, rec.Platform, rec.AppliedAt, rec.ProfileVersion = udid, string(device.IOS), time.Now().UTC(), profileVersion
	rec.Disabled = union(without(rec.Disabled, toEnable), toDisable)
	if rec.Settings == nil {
		rec.Settings = []state.Setting{}
	}
	// Record first: if the store write fails, restore still knows what may have changed.
	if err := p.State.Save(rec); err != nil {
		return "", err
	}
	if err := st.Write(toDisable, toEnable); err != nil {
		return "", err
	}
	detail := fmt.Sprintf("disabled %d labels (%d already disabled)", len(toDisable), len(desired)-len(toDisable))
	if len(toEnable) > 0 {
		detail += fmt.Sprintf(", re-enabled %d kept", len(toEnable))
	}
	return detail, nil
}

// without returns a minus every element of b, preserving order.
func without(a, b []string) []string {
	drop := map[string]bool{}
	for _, x := range b {
		drop[x] = true
	}
	out := []string{}
	for _, x := range a {
		if !drop[x] {
			out = append(out, x)
		}
	}
	return out
}

func (p *Provider) bootAndWait(ctx context.Context, sr device.Stages, udid string) error {
	if err := sr.Do(stageBoot, func() (string, error) {
		_, err := p.cmd(ctx, "boot", udid)
		return "", err
	}); err != nil {
		return err
	}
	return sr.Do(stageWaitLaunchd, func() (string, error) { return p.waitLaunchd(ctx, udid) })
}

// verifyOverrides reads the disabled set back; if launchd ignored the store it
// applies labels through launchctl and reboots once.
func (p *Provider) verifyOverrides(ctx context.Context, udid string, desired []string) (string, error) {
	got, err := p.readbackDisabled(ctx, udid)
	if err != nil {
		return "", err
	}
	miss := missingFrom(desired, got)
	if !tooMany(len(miss), len(desired)) {
		return fmt.Sprintf("%d/%d labels disabled", len(desired)-len(miss), len(desired)), nil
	}
	// Fallback: the supported path, one extra boot.
	for _, l := range miss {
		if _, err := p.cmd(ctx, "spawn", udid, "launchctl", "disable", "system/"+l); err != nil {
			return "", err
		}
	}
	if _, err := p.cmd(ctx, "shutdown", udid); err != nil {
		return "", err
	}
	if _, err := p.cmd(ctx, "boot", udid); err != nil {
		return "", err
	}
	if _, err := p.waitLaunchd(ctx, udid); err != nil {
		return "", err
	}
	got, err = p.readbackDisabled(ctx, udid)
	if err != nil {
		return "", err
	}
	miss2 := missingFrom(desired, got)
	if tooMany(len(miss2), len(desired)) {
		return "", &device.StageError{
			Command: "xcrun simctl spawn " + udid + " launchctl print-disabled system",
			Err:     fmt.Errorf("%d of %d labels still enabled after fallback; device left booted", len(miss2), len(desired)),
		}
	}
	return fmt.Sprintf("fallback applied: %d/%d labels disabled", len(desired)-len(miss2), len(desired)), nil
}

// measureStage reports the footprint; a measurement failure only warns.
func (p *Provider) measureStage(ctx context.Context, sr device.Stages, udid string) {
	sr.R.Report(device.Stage{Name: stageMeasure, Status: device.StageRunning})
	snap, err := p.takeSnapshot(ctx)
	var m device.Measurement
	if err == nil {
		m, err = p.measureWith(ctx, snap, udid)
	}
	if err != nil {
		sr.Warn(stageMeasure, "could not measure: "+err.Error())
		return
	}
	sr.R.Report(device.Stage{
		Name: stageMeasure, Status: device.StageOK,
		Detail:      fmt.Sprintf("%d MB across %d processes", m.FootprintMB, m.ProcessCount),
		Measurement: &m,
	})
}

// Restore implements device.Provider: re-enable exactly the labels in the Change record.
func (p *Provider) Restore(ctx context.Context, udid string, r device.Reporter) error {
	s, err := p.find(ctx, udid)
	if err != nil {
		return err
	}
	sr := device.Stages{R: r}
	rec, ok, err := p.State.Load(udid)
	if err != nil {
		return err
	}
	if !ok {
		sr.Skip(stageWrite, "no change record; nothing to restore")
		return nil
	}
	if err := sr.Do(stageWrite, func() (string, error) {
		if err := p.store(udid).Write(nil, rec.Disabled); err != nil {
			return "", err
		}
		return fmt.Sprintf("re-enabled %d labels", len(rec.Disabled)), nil
	}); err != nil {
		return err
	}
	if mapState(s.State) == device.Booted {
		if err := sr.Do(stageShutdown, func() (string, error) {
			_, err := p.cmd(ctx, "shutdown", udid)
			return "", err
		}); err != nil {
			return err
		}
		if err := p.bootAndWait(ctx, sr, udid); err != nil {
			return err
		}
	}
	return p.State.Delete(udid)
}

// Shutdown implements device.Provider.
func (p *Provider) Shutdown(ctx context.Context, udid string) error {
	s, err := p.find(ctx, udid)
	if err != nil {
		return err
	}
	if mapState(s.State) == device.Shutdown {
		return nil
	}
	_, err = p.cmd(ctx, "shutdown", udid)
	return err
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range append(append([]string{}, a...), b...) {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
