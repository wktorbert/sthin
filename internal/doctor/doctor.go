// Package doctor checks whether a slim boot can work on this host and says why not.
package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/wktorbert/lean-sim/internal/android"
	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/host"
	"github.com/wktorbert/lean-sim/internal/ios"
	"github.com/wktorbert/lean-sim/internal/profile"
)

// Check statuses.
const (
	OK   = "ok"
	Warn = "warn"
	Fail = "fail"
	NA   = "n/a"
)

// Check is one doctor finding.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Doctor holds the host view the checks read.
type Doctor struct {
	Env    host.Env
	Runner device.Runner
}

// Blocking reports whether any check failed.
func Blocking(checks []Check) bool {
	for _, c := range checks {
		if c.Status == Fail {
			return true
		}
	}
	return false
}

// Run executes every check in a fixed order.
func (d Doctor) Run(ctx context.Context) []Check {
	var cs []Check
	cs = append(cs, d.profiles()...)
	cs = append(cs, d.iosChecks(ctx)...)
	cs = append(cs, d.androidChecks()...)
	return cs
}

func (d Doctor) profiles() []Check {
	var cs []Check
	for _, pl := range []string{"ios", "android"} {
		c := Check{Name: pl + "-profile", Status: OK}
		if p, err := profile.Load(pl); err != nil {
			c.Status, c.Detail = Fail, err.Error()
		} else {
			c.Detail = fmt.Sprintf("v%d, %d categories, validated against %s", p.Version, len(p.Categories), stamp(p))
		}
		cs = append(cs, c)
	}
	return cs
}

func stamp(p *profile.Profile) string {
	var parts []string
	for _, k := range []string{"xcode", "ios", "api", "image"} {
		if v, ok := p.ValidatedAgainst[k]; ok {
			parts = append(parts, k+" "+v)
		}
	}
	return strings.Join(parts, ", ")
}

func (d Doctor) iosChecks(ctx context.Context) []Check {
	names := []string{"xcode-simctl", "ios-runtime-vs-profile", "ios-runtime-persistence", "ios-simulator-app"}
	if d.Env.GOOS != "darwin" {
		cs := make([]Check, len(names))
		for i, n := range names {
			cs[i] = Check{Name: n, Status: NA, Detail: "iOS simulators require macOS"}
		}
		return cs
	}
	simctl := Check{Name: names[0], Status: OK}
	versions, err := ios.RuntimeVersions(ctx, d.Runner)
	if err != nil {
		simctl.Status, simctl.Detail = Fail, err.Error()
		return []Check{simctl,
			{Name: names[1], Status: NA, Detail: "simctl unavailable"},
			{Name: names[2], Status: NA, Detail: "simctl unavailable"},
			d.simulatorApp(names[3])}
	}
	simctl.Detail = fmt.Sprintf("%d iOS runtime(s): %s", len(versions), strings.Join(versions, ", "))

	validated := profile.MustLoad("ios").ValidatedAgainst["ios"]
	var newer, old []string
	for _, v := range versions {
		if profile.Newer(validated, v) {
			newer = append(newer, v)
		}
		if !ios.PersistsOverrides(v) {
			old = append(old, v)
		}
	}
	vs := Check{Name: names[1], Status: OK, Detail: "all runtimes at or below the Profile's iOS " + validated}
	if len(newer) > 0 {
		vs.Status = Warn
		vs.Detail = fmt.Sprintf("iOS %s newer than the Profile (validated against iOS %s); labels may have been renamed", strings.Join(newer, ", "), validated)
	}
	ps := Check{Name: names[2], Status: OK, Detail: "every runtime persists overrides"}
	if len(old) > 0 {
		ps.Status = Warn
		ps.Detail = fmt.Sprintf("iOS %s cannot persist overrides (needs 18.5+); slim boot boots these stock", strings.Join(old, ", "))
	}
	return []Check{simctl, vs, ps, d.simulatorApp(names[3])}
}

// simulatorApp reports which app will show the simulator window. Xcode 27
// dropped Simulator.app in favour of DeviceHub.app; without either, a slim
// boot still works but the device stays headless.
func (d Doctor) simulatorApp(name string) Check {
	apps := d.Env.SimulatorApps()
	if len(apps) == 0 {
		return Check{Name: name, Status: Warn, Detail: "no Simulator.app (Xcode ≤ 26) or DeviceHub.app (Xcode 27) under " + d.Env.DeveloperDir() + "; boot will try LaunchServices by name, the window may not open"}
	}
	return Check{Name: name, Status: OK, Detail: apps[0]}
}

func (d Doctor) androidChecks() []Check {
	avds, _ := android.ListAVDs(d.Env.AVDHome())
	haveAVDs := len(avds) > 0

	emu := Check{Name: "android-emulator", Status: OK}
	if p, ok := d.Env.Emulator(); ok {
		emu.Detail = p
	} else if haveAVDs {
		emu.Status, emu.Detail = Fail, "no emulator binary on PATH or under ANDROID_HOME, ANDROID_SDK_ROOT, or the default SDK"
	} else {
		emu.Status, emu.Detail = Warn, "Android SDK not found; Android devices unavailable"
	}
	adb := Check{Name: "android-adb", Status: OK}
	if p, ok := d.Env.Adb(); ok {
		adb.Detail = p
	} else if haveAVDs {
		adb.Status, adb.Detail = Fail, "AVDs exist but adb was not found (install platform-tools)"
	} else {
		adb.Status, adb.Detail = Warn, "adb not found"
	}
	cs := []Check{emu, adb}

	names := []string{"android-api-vs-profile", "avd-page-size", "avd-gpu-mode", "avd-play-store"}
	if !haveAVDs {
		for _, n := range names {
			cs = append(cs, Check{Name: n, Status: NA, Detail: "no AVDs in " + d.Env.AVDHome()})
		}
		return cs
	}
	validated := profile.MustLoad("android").ValidatedAgainst["api"]
	var newer, ps16k, gpu, play []string
	for _, a := range avds {
		if api := android.APILevel(a.Config); api != "" && profile.Newer(validated, api) {
			newer = append(newer, a.Name+" (API "+api+")")
		}
		if bad, _ := android.Is16k(a.Config); bad {
			ps16k = append(ps16k, a.Name)
		}
		if m := a.Config["hw.gpu.mode"]; m == "auto" || strings.HasPrefix(m, "swiftshader") {
			gpu = append(gpu, a.Name+" ("+m+")")
		}
		if a.Config["PlayStore.enabled"] == "true" || strings.Contains(a.Config["tag.id"], "playstore") {
			play = append(play, a.Name)
		}
	}
	cs = append(cs,
		listCheck(names[0], newer, "all AVDs at or below the Profile's API "+validated, "newer than the Profile (validated against API "+validated+")"),
		listCheck(names[1], ps16k, "no 16 KB page-size images", "use 16 KB page-size images; Lean refuses to tune them"),
		listCheck(names[2], gpu, "no AVD uses gpu auto or swiftshader", "use a slow GPU mode; a slim boot switches them to host"),
		listCheck(names[3], play, "no Play Store images", "are Play Store images (no adb root; a few system packages may refuse to disable)"),
	)
	return cs
}

func listCheck(name string, hits []string, okDetail, warnSuffix string) Check {
	if len(hits) == 0 {
		return Check{Name: name, Status: OK, Detail: okDetail}
	}
	return Check{Name: name, Status: Warn, Detail: strings.Join(hits, ", ") + " " + warnSuffix}
}
