package android

import (
	"context"
	"strings"

	"github.com/wktorbert/lean-sim/internal/device"
)

// physicalDevices lists phones adb can see over USB or Wi-Fi. They are shown
// and usable but never slimmed.
func (p *Provider) physicalDevices(ctx context.Context) []device.Device {
	if p.Adb == "" {
		return nil
	}
	out, err := p.adb(ctx, "devices", "-l")
	if err != nil {
		return nil
	}
	var devs []device.Device
	for _, d := range parseAdbDevices(out) {
		if d.State == device.Booted {
			if v, err := p.shell(ctx, d.ID, "getprop", "ro.build.version.release"); err == nil {
				if v = strings.TrimSpace(v); v != "" {
					d.OSVersion = "Android " + v
				}
			}
		}
		devs = append(devs, d)
	}
	return devs
}

// parseAdbDevices turns `adb devices -l` output into physical Devices,
// skipping emulator serials. Version is filled in by the caller.
func parseAdbDevices(out string) []device.Device {
	var devs []device.Device
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] == "List" || strings.HasPrefix(f[0], "emulator-") || f[0] == "*" {
			continue
		}
		serial, state := f[0], f[1]
		model := ""
		for _, kv := range f[2:] {
			if strings.HasPrefix(kv, "model:") {
				model = strings.ReplaceAll(strings.TrimPrefix(kv, "model:"), "_", " ")
			}
		}
		transport := "usb"
		if strings.Contains(serial, ":") {
			transport = "wireless"
		}
		d := device.Device{
			ID: serial, Name: model, Platform: device.Android, Kind: device.Physical,
			State: device.Shutdown, Slim: device.NotApplicable, Model: model + " · " + transport, Warnings: []string{},
		}
		if d.Name == "" {
			d.Name = serial
			d.Model = transport
		}
		switch state {
		case "device":
			d.State = device.Booted
		case "unauthorized":
			d.Warnings = append(d.Warnings, "unauthorized: accept the USB debugging prompt on the phone")
		default:
			d.Warnings = append(d.Warnings, "adb state: "+state)
		}
		devs = append(devs, d)
	}
	return devs
}

// Pair implements device.WirelessADB: `adb pair host:port code`.
func (p *Provider) Pair(ctx context.Context, addr, code string) (string, error) {
	out, err := p.adb(ctx, "pair", addr, code)
	return strings.TrimSpace(out), err
}

// Connect implements device.WirelessADB: `adb connect host:port`. adb exits 0
// even when it fails, so the message is checked.
func (p *Provider) Connect(ctx context.Context, addr string) (string, error) {
	out, err := p.adb(ctx, "connect", addr)
	out = strings.TrimSpace(out)
	if err != nil {
		return out, err
	}
	if strings.Contains(out, "failed") || strings.Contains(out, "cannot") || strings.Contains(out, "unable") {
		return out, &device.StageError{Command: p.Adb + " connect " + addr, Err: errorString(out)}
	}
	return out, nil
}

// Disconnect implements device.WirelessADB.
func (p *Provider) Disconnect(ctx context.Context, addr string) (string, error) {
	out, err := p.adb(ctx, "disconnect", addr)
	return strings.TrimSpace(out), err
}

type errorString string

func (e errorString) Error() string { return string(e) }
