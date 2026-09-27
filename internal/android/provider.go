// Package android is the Provider for Android virtual devices. Every emulator
// and adb invocation, and every AVD file path, lives here.
package android

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/host"
	"github.com/wktorbert/sthin/internal/profile"
	"github.com/wktorbert/sthin/internal/state"
)

// Provider implements device.Provider for AVDs.
type Provider struct {
	Run       device.Runner
	Env       host.Env
	AVDHome   string
	Adb       string // path to adb, "" when not found
	Emulator  string // path to emulator, "" when not found
	SthinHome string
	Profile   *profile.Profile
	State     state.Store

	PollInterval time.Duration // between boot polls; default 2 s
	WaitCap      time.Duration // boot wait cap; default 180 s
	MaxPolls     int           // tests only: stop polling after this many attempts
	PowerOffCap  time.Duration // clean power-off cap; default 60 s
	PersistWait  time.Duration // wait for PackageManager to persist before power-off; default 12 s, negative disables (tests)
}

// New returns a Provider wired to the host's SDK and AVD locations.
func New(run device.Runner, env host.Env) *Provider {
	adb, _ := env.Adb()
	emu, _ := env.Emulator()
	return &Provider{
		Run: run, Env: env, AVDHome: env.AVDHome(), Adb: adb, Emulator: emu, SthinHome: env.SthinHome(),
		Profile: profile.MustLoad("android"), State: state.Store{Home: env.SthinHome()},
	}
}

// Platform implements device.Provider.
func (p *Provider) Platform() device.Platform { return device.Android }

// Available implements device.Provider.
func (p *Provider) Available() (bool, string) {
	if p.Emulator == "" {
		if _, err := os.Stat(p.AVDHome); err != nil {
			return false, "Android SDK not found (no emulator binary, no " + p.AVDHome + ")"
		}
	}
	return true, ""
}

// avd is one AVD as read from disk.
type avd struct {
	Name   string            // AVD id, e.g. Pixel_7_Pro
	Dir    string            // the .avd directory
	Config map[string]string // config.ini
}

// readINI parses key=value lines.
func readINI(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m, sc.Err()
}

// avds reads every <name>.ini under AVDHome, sorted by name.
func (p *Provider) avds() ([]avd, error) {
	matches, err := filepath.Glob(filepath.Join(p.AVDHome, "*.ini"))
	if err != nil {
		return nil, err
	}
	var out []avd
	for _, m := range matches {
		ini, err := readINI(m)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(m), ".ini")
		dir := ini["path"]
		if dir == "" || !isDir(dir) {
			dir = filepath.Join(p.AVDHome, name+".avd")
		}
		cfg, err := readINI(filepath.Join(dir, "config.ini"))
		if err != nil {
			cfg = map[string]string{}
		}
		out = append(out, avd{Name: name, Dir: dir, Config: cfg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func (p *Provider) findAVD(name string) (avd, error) {
	all, err := p.avds()
	if err != nil {
		return avd{}, err
	}
	for _, a := range all {
		if a.Name == name {
			return a, nil
		}
	}
	return avd{}, fmt.Errorf("%w: unknown AVD %q", device.ErrUsage, name)
}

// apiLevel extracts the API level from config.ini's target (android-33) or image.sysdir.1.
func apiLevel(cfg map[string]string) string {
	for _, k := range []string{"target", "image.sysdir.1"} {
		v := cfg[k]
		if i := strings.Index(v, "android-"); i >= 0 {
			rest := v[i+len("android-"):]
			if j := strings.IndexAny(rest, "/ "); j >= 0 {
				rest = rest[:j]
			}
			return rest
		}
	}
	return ""
}

func (a avd) displayName() string {
	if n := a.Config["avd.ini.displayname"]; n != "" {
		return n
	}
	return a.Name
}

func (a avd) osVersion() string {
	if api := apiLevel(a.Config); api != "" {
		return "API " + api
	}
	return ""
}

func (p *Provider) adb(ctx context.Context, args ...string) (string, error) {
	if p.Adb == "" {
		return "", fmt.Errorf("adb not found")
	}
	out, stderr, err := p.Run.Run(ctx, p.Adb, args...)
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", p.Adb, strings.Join(args, " "), err, strings.TrimSpace(string(stderr)))
	}
	return string(out), nil
}

// running maps AVD name → emulator serial for every emulator adb can see.
func (p *Provider) running(ctx context.Context) map[string]string {
	out := map[string]string{}
	if p.Adb == "" {
		return out
	}
	list, err := p.adb(ctx, "devices")
	if err != nil {
		return out
	}
	for _, serial := range emulatorSerials(list) {
		name, err := p.adb(ctx, "-s", serial, "emu", "avd", "name")
		if err != nil {
			continue
		}
		if n := firstLine(name); n != "" {
			out[n] = serial
		}
	}
	return out
}

// emulatorSerials returns the emulator-NNNN serials in `adb devices` output (any state).
func emulatorSerials(out string) []string {
	var s []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.HasPrefix(f[0], "emulator-") {
			s = append(s, f[0])
		}
	}
	return s
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && l != "OK" {
			return l
		}
	}
	return ""
}

// List implements device.Provider.
func (p *Provider) List(ctx context.Context) ([]device.Device, error) {
	all, err := p.avds()
	if err != nil {
		return nil, err
	}
	run := p.running(ctx)
	var snap *hostSnapshot // one ps + top sample shared by every running AVD
	out := make([]device.Device, 0, len(all))
	for _, a := range all {
		d := device.Device{
			ID:        a.Name,
			Name:      a.displayName(),
			Platform:  device.Android,
			Kind:      device.Emulator,
			OSVersion: a.osVersion(),
			State:     device.Shutdown,
			Warnings:  []string{},
		}
		serial, ok := run[a.Name]
		if ok {
			d.State = device.Booted
		}
		d.Slim = p.slimState(ctx, a.Name, serial)
		if ok {
			if snap == nil {
				snap, _ = p.takeSnapshot(ctx)
			}
			if snap != nil {
				if pid, found := qemuPID(snap.args, a.Name); found {
					mb := int(math.Round(snap.mem[pid] / (1 << 20)))
					d.FootprintMB = &mb
				}
			}
		}
		out = append(out, d)
	}
	out = append(out, p.physicalDevices(ctx)...)
	return out, nil
}
