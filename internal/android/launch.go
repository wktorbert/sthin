package android

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wktorbert/sthin/internal/device"
)

const (
	defaultPoll    = 2 * time.Second
	bootWaitCap    = 180 * time.Second
	powerOffCap    = 60 * time.Second
	persistWait    = 12 * time.Second
	stageRename    = "rename"
	stageTune      = "tune"
	stageLaunch    = "launch"
	stageWaitBoot  = "wait-boot"
	stageDisable   = "disable-packages"
	stageSettings  = "settings"
	stageMeasure   = "measure"
	stageEnable    = "enable-packages"
	stageUntune    = "untune"
	stageKill      = "power-off"
	stageUnchanged = "restore"
)

// launchArgs builds the emulator argv for a slim (or stock) launch.
//
// SPEC step 3 also lists -lowram. It is deliberately omitted: on emulator
// 36.4.9 with an android-33 image, -lowram alone makes qemu fail at startup
// with "could not load initrd <avd>/initrd" (verified 2026-09-25 on this Mac).
// The guest RAM cap comes from -memory and hw.ramSize instead.
//
// launchSwitches are the resolved per-AVD options (see device.LaunchOptions).
type launchSwitches struct {
	headless, coldBoot, audio, lowRAM bool
}

func launchArgs(name string, ramMB int, stock bool, sw launchSwitches, goos, display string) []string {
	gpu := "host"
	if goos == "linux" && display == "" {
		gpu = "swiftshader_indirect"
	}
	args := []string{"-avd", name, "-gpu", gpu}
	if !stock {
		args = append(args, "-memory", strconv.Itoa(ramMB))
		if sw.lowRAM {
			args = append(args, "-lowram")
		}
	}
	args = append(args, "-no-boot-anim", "-no-snapshot-save")
	if sw.coldBoot {
		args = append(args, "-no-snapshot-load")
	}
	if !sw.audio {
		args = append(args, "-no-audio")
	}
	args = append(args, "-camera-back", "none", "-camera-front", "none")
	if sw.headless {
		args = append(args, "-no-window")
	}
	return args
}

// describe summarises the switches for a stage detail.
func (sw launchSwitches) describe() string {
	var parts []string
	if sw.coldBoot {
		parts = append(parts, "cold boot")
	}
	if sw.audio {
		parts = append(parts, "audio on")
	}
	if sw.lowRAM {
		parts = append(parts, "lowram")
	}
	if sw.headless {
		parts = append(parts, "headless")
	}
	if len(parts) == 0 {
		return "defaults"
	}
	return strings.Join(parts, ", ")
}

func (p *Provider) poll() time.Duration {
	if p.PollInterval > 0 {
		return p.PollInterval
	}
	return defaultPoll
}

// launch starts the emulator detached with its log under $STHIN_HOME/logs.
func (p *Provider) launch(ctx context.Context, name string, ramMB int, stock bool, sw launchSwitches) (string, error) {
	if p.Emulator == "" {
		return "", fmt.Errorf("emulator binary not found (set ANDROID_HOME)")
	}
	goos, display := "", ""
	if p.Env.Getenv != nil {
		goos, display = p.Env.GOOS, p.Env.Getenv("DISPLAY")
	}
	args := launchArgs(name, ramMB, stock, sw, goos, display)
	logPath := filepath.Join(p.SthinHome, "logs", name+".log")
	if err := p.Run.Start(ctx, logPath, p.Emulator, args...); err != nil {
		return "", &device.StageError{Command: p.Emulator + " " + strings.Join(args, " "), Err: err}
	}
	return sw.describe() + "; log: " + logPath, nil
}

// waitBoot polls adb until an emulator serial reports this AVD's name and
// sys.boot_completed=1, returning the serial.
func (p *Provider) waitBoot(ctx context.Context, name string) (string, error) {
	if p.Adb == "" {
		return "", fmt.Errorf("adb not found; cannot wait for boot")
	}
	limit := p.WaitCap
	if limit == 0 {
		limit = bootWaitCap
	}
	start := time.Now()
	names := map[string]string{} // serial → avd name, cached once known
	serial := ""
	for attempt := 1; ; attempt++ {
		if serial == "" {
			if out, err := p.adb(ctx, "devices"); err == nil {
				for _, s := range emulatorSerials(out) {
					if _, known := names[s]; !known {
						if n, err := p.adb(ctx, "-s", s, "emu", "avd", "name"); err == nil && firstLine(n) != "" {
							names[s] = firstLine(n)
						}
					}
					if names[s] == name {
						serial = s
						break
					}
				}
			}
		}
		if serial != "" {
			if out, err := p.adb(ctx, "-s", serial, "shell", "getprop", "sys.boot_completed"); err == nil && strings.TrimSpace(out) == "1" {
				return serial, nil
			}
		}
		if time.Since(start) >= limit || (p.MaxPolls > 0 && attempt >= p.MaxPolls) {
			what := "no emulator serial for " + name
			if serial != "" {
				what = serial + " did not report sys.boot_completed=1"
			}
			return "", &device.StageError{Command: p.Adb + " -s <serial> shell getprop sys.boot_completed", Err: fmt.Errorf("timed out after %s: %s", limit, what)}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.poll()):
		}
	}
}

// Shutdown implements device.Provider with a graceful power-off (see powerOff).
func (p *Provider) Shutdown(ctx context.Context, id string) error {
	if _, err := p.findAVD(id); err != nil {
		return err
	}
	serial, ok := p.running(ctx)[id]
	if !ok {
		return nil
	}
	_, err := p.powerOff(ctx, serial)
	return err
}

// waitPersist gives PackageManager time to write package-restrictions.xml.
// It schedules the write about 10 s after a `pm enable`, and neither
// `reboot -p` nor `emu kill` flushes a pending write: packages re-enabled
// within the last few seconds came back disabled after the next boot
// (reproduced on Pixel_7_Pro, 2026-09-25; a 15 s wait fixed it).
func (p *Provider) waitPersist(ctx context.Context) error {
	d := p.PersistWait
	if d == 0 {
		d = persistWait
	}
	if d < 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// powerOff shuts the guest down cleanly with `reboot -p` and waits for the
// serial to leave adb, falling back to `emu kill`.
//
// SPEC says `emu kill`. That alone loses guest state: PackageManager writes
// package-restrictions lazily, and a `pm disable-user`/`pm enable` followed by
// an immediate `emu kill` is gone after the next boot (reproduced on
// Pixel_7_Pro, 2026-09-25). A clean power-off persists it.
func (p *Provider) powerOff(ctx context.Context, serial string) (string, error) {
	limit := p.PowerOffCap
	if limit == 0 {
		limit = powerOffCap
	}
	if _, err := p.shell(ctx, serial, "reboot", "-p"); err == nil {
		start := time.Now()
		for attempt := 1; ; attempt++ {
			out, err := p.adb(ctx, "devices")
			if err == nil && !contains(emulatorSerials(out), serial) {
				return "powered off cleanly", nil
			}
			if time.Since(start) >= limit || (p.MaxPolls > 0 && attempt >= p.MaxPolls) {
				break
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.poll()):
			}
		}
	}
	if _, err := p.adb(ctx, "-s", serial, "emu", "kill"); err != nil {
		return "", &device.StageError{Command: p.Adb + " -s " + serial + " emu kill", Err: err}
	}
	return "clean power-off timed out; killed", nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
