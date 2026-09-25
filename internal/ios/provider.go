// Package ios is the Provider for iOS simulators. Every simctl, launchctl,
// top, and ps invocation, and every Override store path, lives here.
package ios

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/host"
	"github.com/wktorbert/lean-sim/internal/profile"
	"github.com/wktorbert/lean-sim/internal/state"
)

// Provider implements device.Provider for simulators.
type Provider struct {
	Run          device.Runner
	Env          host.Env
	OverrideRoot string // directory holding com.apple.CoreSimulator.SimDevice.<UDID>/
	Profile      *profile.Profile
	State        state.Store
	PollInterval time.Duration // between launchd readiness polls; default 1 s
	WaitCap      time.Duration // launchd readiness cap; default 120 s
	MaxPolls     int           // tests only: stop polling after this many attempts
}

// New returns a Provider wired to the real host locations.
func New(run device.Runner, env host.Env) *Provider {
	return &Provider{
		Run:          run,
		Env:          env,
		OverrideRoot: "/private/var/tmp",
		Profile:      profile.MustLoad("ios"),
		State:        state.Store{Home: env.LeanHome()},
	}
}

// Platform implements device.Provider.
func (p *Provider) Platform() device.Platform { return device.IOS }

// Available implements device.Provider.
func (p *Provider) Available() (bool, string) { return p.Env.IOSSupported() }

// sim is one simulator as simctl reports it, plus its runtime version.
type sim struct {
	UDID     string
	Name     string
	State    string
	Version  string
	DataPath string // where the device's data lives; a wiped device still lists but cannot boot
}

func (p *Provider) simctl(ctx context.Context, args ...string) ([]byte, error) {
	out, stderr, err := p.Run.Run(ctx, "xcrun", append([]string{"simctl"}, args...)...)
	if err != nil {
		return out, fmt.Errorf("xcrun simctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(stderr)))
	}
	return out, nil
}

// sims returns every available iOS simulator, sorted by runtime version then name.
func (p *Provider) sims(ctx context.Context) ([]sim, error) {
	devOut, err := p.simctl(ctx, "list", "-j", "devices", "available")
	if err != nil {
		return nil, err
	}
	rtOut, err := p.simctl(ctx, "list", "-j", "runtimes")
	if err != nil {
		return nil, err
	}
	var devs struct {
		Devices map[string][]struct {
			UDID        string `json:"udid"`
			Name        string `json:"name"`
			State       string `json:"state"`
			IsAvailable bool   `json:"isAvailable"`
			DataPath    string `json:"dataPath"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(devOut, &devs); err != nil {
		return nil, fmt.Errorf("parse simctl devices: %w", err)
	}
	var rts struct {
		Runtimes []struct {
			Identifier string `json:"identifier"`
			Version    string `json:"version"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal(rtOut, &rts); err != nil {
		return nil, fmt.Errorf("parse simctl runtimes: %w", err)
	}
	versions := map[string]string{}
	for _, r := range rts.Runtimes {
		versions[r.Identifier] = r.Version
	}
	var out []sim
	for rt, list := range devs.Devices {
		if !strings.Contains(rt, "SimRuntime.iOS-") {
			continue
		}
		v := versions[rt]
		if v == "" {
			v = strings.ReplaceAll(rt[strings.LastIndex(rt, "iOS-")+4:], "-", ".")
		}
		for _, d := range list {
			if !d.IsAvailable {
				continue
			}
			out = append(out, sim{UDID: d.UDID, Name: d.Name, State: d.State, Version: v, DataPath: d.DataPath})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if c := profile.CompareVersions(out[i].Version, out[j].Version); c != 0 {
			return c < 0
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func mapState(s string) device.State {
	switch s {
	case "Booted":
		return device.Booted
	case "Booting", "Shutting Down", "Creating":
		return device.Booting
	default:
		return device.Shutdown
	}
}

// find returns the simulator with the given UDID.
func (p *Provider) find(ctx context.Context, udid string) (sim, error) {
	all, err := p.sims(ctx)
	if err != nil {
		return sim{}, err
	}
	for _, s := range all {
		if s.UDID == udid {
			return s, nil
		}
	}
	return sim{}, fmt.Errorf("%w: unknown simulator %q", device.ErrUsage, udid)
}

// List implements device.Provider.
func (p *Provider) List(ctx context.Context) ([]device.Device, error) {
	all, err := p.sims(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]device.Device, 0, len(all))
	for _, s := range all {
		warnings := p.warnings(s.Version)
		if p.Env.MissingDir(s.DataPath) {
			// simctl keeps listing a device whose data directory was deleted from
			// disk; it boots into a black screen or refuses to boot at all.
			warnings = append(warnings, "device data missing on disk; run: xcrun simctl erase "+s.UDID)
		}
		out = append(out, device.Device{
			ID:        s.UDID,
			Name:      s.Name,
			Platform:  device.IOS,
			Kind:      device.Simulator,
			OSVersion: s.Version,
			State:     mapState(s.State),
			Slim:      p.slimState(s.UDID),
			Warnings:  warnings,
		})
	}
	p.fillFootprints(ctx, out)
	if phys, err := p.physicalDevices(ctx); err == nil {
		out = append(out, phys...)
	}
	return out, nil
}
