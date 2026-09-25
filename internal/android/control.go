package android

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/logs"
)

// device.Controller for emulators and physical Android devices, through adb.

// serialOf maps a Device ID to an adb serial: physical devices are addressed
// by serial already, AVDs by the serial of their running emulator.
func (p *Provider) serialOf(ctx context.Context, id string) (string, error) {
	if strings.HasPrefix(id, "emulator-") || strings.Contains(id, ":") || !isAVDName(id) {
		return id, nil
	}
	if serial, ok := p.running(ctx)[id]; ok {
		return serial, nil
	}
	return "", fmt.Errorf("%w: %s", device.ErrNotBooted, id)
}

// isAVDName reports whether id looks like an AVD name rather than a USB serial.
// AVD names never contain a colon; USB serials are typically shorter uppercase
// alphanumerics, so the check is "does a config.ini exist for it" when possible.
func isAVDName(id string) bool {
	return !strings.ContainsAny(id, ":/") && strings.ContainsAny(id, "_-abcdefghijklmnopqrstuvwxyz")
}

// Screenshot implements device.Controller.
func (p *Provider) Screenshot(ctx context.Context, id, path string) error {
	serial, err := p.serialOf(ctx, id)
	if err != nil {
		return err
	}
	png, stderr, err := p.Run.Run(ctx, p.Adb, "-s", serial, "exec-out", "screencap", "-p")
	if err != nil {
		return fmt.Errorf("screencap: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return os.WriteFile(path, png, 0o644)
}

// Tap implements device.Controller.
func (p *Provider) Tap(ctx context.Context, id string, x, y int) error {
	serial, err := p.serialOf(ctx, id)
	if err != nil {
		return err
	}
	_, err = p.shell(ctx, serial, "input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	return err
}

// Install implements device.Controller: an .apk path, reinstalling if present.
func (p *Provider) Install(ctx context.Context, id, path string) error {
	serial, err := p.serialOf(ctx, id)
	if err != nil {
		return err
	}
	out, err := p.adb(ctx, "-s", serial, "install", "-r", path)
	if err != nil {
		return err
	}
	if strings.Contains(out, "Failure") {
		return fmt.Errorf("install: %s", strings.TrimSpace(out))
	}
	return nil
}

// Launch implements device.Controller: a package name, via its launcher activity.
func (p *Provider) Launch(ctx context.Context, id, pkg string) error {
	serial, err := p.serialOf(ctx, id)
	if err != nil {
		return err
	}
	out, err := p.shell(ctx, serial, "monkey", "-p", pkg, "-c", "android.intent.category.LAUNCHER", "1")
	if err != nil {
		return err
	}
	if strings.Contains(out, "No activities found") {
		return fmt.Errorf("launch: %s has no launcher activity or is not installed", pkg)
	}
	return nil
}

// Serial implements device.Controller.
func (p *Provider) Serial(ctx context.Context, id string) (string, error) { return p.serialOf(ctx, id) }

// OpenURL implements device.Controller: a VIEW intent.
func (p *Provider) OpenURL(ctx context.Context, id, url string) error {
	serial, err := p.serialOf(ctx, id)
	if err != nil {
		return err
	}
	out, err := p.shell(ctx, serial, "am", "start", "-a", "android.intent.action.VIEW", "-d", url)
	if err != nil {
		return err
	}
	if strings.Contains(out, "Error") {
		return fmt.Errorf("open url: %s", strings.TrimSpace(out))
	}
	return nil
}

// LogStream implements device.Controller: `logcat -v threadtime` on an
// emulator or a physical device.
func (p *Provider) LogStream(ctx context.Context, id string) (<-chan logs.Line, error) {
	serial, err := p.serialOf(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := p.Run.Stream(ctx, p.Adb, "-s", serial, "logcat", "-v", "threadtime")
	if err != nil {
		return nil, err
	}
	out := make(chan logs.Line, 256)
	go func() {
		defer close(out)
		for s := range raw {
			out <- logs.ParseLogcat(s)
		}
	}()
	return out, nil
}

// Logs implements device.Controller: the last lines of logcat.
func (p *Provider) Logs(ctx context.Context, id string, lines int) (string, error) {
	serial, err := p.serialOf(ctx, id)
	if err != nil {
		return "", err
	}
	if lines <= 0 {
		lines = 200
	}
	return p.adb(ctx, "-s", serial, "logcat", "-d", "-t", strconv.Itoa(lines))
}
