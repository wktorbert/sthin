package android

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/profile"
	"github.com/wktorbert/lean-sim/internal/state"
)

func (p *Provider) profile() *profile.Profile {
	if p.Profile == nil {
		p.Profile = profile.MustLoad("android")
	}
	return p.Profile
}

// guestSetting is one `settings put` a slim boot applies.
type guestSetting struct{ scope, key, value string }

var slimSettings = []guestSetting{
	{"global", "window_animation_scale", "0"},
	{"global", "transition_animation_scale", "0"},
	{"global", "animator_duration_scale", "0"},
	{"global", "auto_sync", "0"},
	{"global", "activity_manager_constants", "max_cached_processes=4"},
}

// packages parses `pm list packages` output into a set.
func packages(out string) map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if pkg, ok := strings.CutPrefix(strings.TrimSpace(line), "package:"); ok && pkg != "" {
			m[pkg] = true
		}
	}
	return m
}

func (p *Provider) shell(ctx context.Context, serial string, args ...string) (string, error) {
	return p.adb(ctx, append([]string{"-s", serial, "shell"}, args...)...)
}

func (p *Provider) listPackages(ctx context.Context, serial string, disabledOnly bool) (map[string]bool, error) {
	args := []string{"pm", "list", "packages"}
	if disabledOnly {
		args = append(args, "-d")
	}
	out, err := p.shell(ctx, serial, append(args, "--user", "0")...)
	if err != nil {
		return nil, err
	}
	return packages(out), nil
}

// slimGuest disables the Profile's packages and applies settings on a booted
// guest, recording exactly what changed.
func (p *Provider) slimGuest(ctx context.Context, st device.Stages, id, serial string, except []string) error {
	rec, _, err := p.State.Load(id)
	if err != nil {
		return err
	}
	rec.ID, rec.Platform, rec.ProfileVersion = id, string(device.Android), p.profile().Version
	if rec.AppliedAt.IsZero() {
		rec.AppliedAt = time.Now().UTC()
	}
	if rec.Disabled == nil {
		rec.Disabled = []string{}
	}
	if err := st.Do(stageDisable, func() (string, error) {
		detail, err := p.disablePackages(ctx, serial, profile.DesiredSet(p.profile(), except), &rec)
		if saveErr := p.State.Save(rec); err == nil {
			err = saveErr
		}
		return detail, err
	}); err != nil {
		return err
	}
	return st.Do(stageSettings, func() (string, error) {
		detail := p.applySettings(ctx, serial, &rec)
		return detail, p.State.Save(rec)
	})
}

func (p *Provider) disablePackages(ctx context.Context, serial string, desired []string, rec *state.Record) (string, error) {
	installed, err := p.listPackages(ctx, serial, false)
	if err != nil {
		return "", err
	}
	already, err := p.listPackages(ctx, serial, true)
	if err != nil {
		return "", err
	}
	var disabled, skipped, refused int
	for _, pkg := range desired {
		switch {
		case !installed[pkg]:
			continue // not on this image
		case already[pkg]:
			skipped++
			continue
		}
		out, err := p.shell(ctx, serial, "pm", "disable-user", "--user", "0", pkg)
		if err != nil || !strings.Contains(out, "new state: disabled") {
			refused++
			continue
		}
		disabled++
		rec.Disabled = union(rec.Disabled, []string{pkg})
	}
	// Packages Lean disabled earlier that the current selection keeps (a
	// category newly excepted) are switched back on and dropped from the record.
	want := map[string]bool{}
	for _, pkg := range desired {
		want[pkg] = true
	}
	var reenabled int
	keep := []string{}
	for _, pkg := range rec.Disabled {
		if want[pkg] {
			keep = append(keep, pkg)
			continue
		}
		if _, err := p.shell(ctx, serial, "pm", "enable", "--user", "0", pkg); err != nil {
			keep = append(keep, pkg) // still disabled; restore will retry
			refused++
			continue
		}
		reenabled++
	}
	rec.Disabled = keep
	detail := fmt.Sprintf("disabled %d, %d already disabled", disabled, skipped)
	if reenabled > 0 {
		detail += fmt.Sprintf(", re-enabled %d kept", reenabled)
	}
	if refused > 0 {
		detail += fmt.Sprintf(", %d refused", refused)
	}
	return detail, nil
}

func (p *Provider) applySettings(ctx context.Context, serial string, rec *state.Record) string {
	var applied, skipped int
	for _, s := range slimSettings {
		idx := -1
		for i, r := range rec.Settings {
			if r.Scope == s.scope && r.Key == s.key {
				idx = i
			}
		}
		var prior *string
		if idx >= 0 {
			prior = rec.Settings[idx].Prior // keep the original prior across re-slims
		} else {
			out, err := p.shell(ctx, serial, "settings", "get", s.scope, s.key)
			if err != nil {
				skipped++
				rec.Settings = append(rec.Settings, state.Setting{Key: s.key, Scope: s.scope, New: s.value, Skipped: true})
				continue
			}
			if v := strings.TrimSpace(out); v != "null" && v != "" {
				prior = &v
			}
		}
		_, err := p.shell(ctx, serial, "settings", "put", s.scope, s.key, s.value)
		entry := state.Setting{Key: s.key, Scope: s.scope, Prior: prior, New: s.value, Skipped: err != nil}
		if idx >= 0 {
			rec.Settings[idx] = entry
		} else {
			rec.Settings = append(rec.Settings, entry)
		}
		if err != nil {
			skipped++
		} else {
			applied++
		}
	}
	d := fmt.Sprintf("applied %d settings", applied)
	if skipped > 0 {
		d += fmt.Sprintf(", %d rejected (recorded as skipped)", skipped)
	}
	return d
}

// slimState derives the slim state: from the guest when booted, else the Change record.
func (p *Provider) slimState(ctx context.Context, id, serial string) device.SlimState {
	// Slim means "matches this device's own selection": kept categories do
	// not count against it.
	except, _ := p.State.ExceptFor(id, nil)
	desired := profile.DesiredSet(p.profile(), except)
	if serial != "" {
		installed, err1 := p.listPackages(ctx, serial, false)
		dis, err2 := p.listPackages(ctx, serial, true)
		if err1 != nil || err2 != nil {
			return device.Unknown
		}
		var want []string
		for _, pkg := range desired {
			if installed[pkg] {
				want = append(want, pkg)
			}
		}
		return classify(want, dis)
	}
	rec, ok, err := p.State.Load(id)
	switch {
	case err != nil:
		return device.Unknown
	case !ok:
		return device.Stock
	case len(rec.Disabled) == 0:
		return device.Partial // tuned, packages not yet disabled
	default:
		return device.Slim
	}
}

func classify(desired []string, disabled map[string]bool) device.SlimState {
	n := 0
	for _, x := range desired {
		if disabled[x] {
			n++
		}
	}
	switch {
	case len(desired) == 0 || n == 0:
		return device.Stock
	case n == len(desired):
		return device.Slim
	default:
		return device.Partial
	}
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
	if out == nil {
		out = []string{}
	}
	return out
}
