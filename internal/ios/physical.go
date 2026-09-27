package ios

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wktorbert/sthin/internal/device"
)

// physicalDevices lists paired physical iPhones and iPads through devicectl.
// They are shown and usable but never slimmed. A missing or failing devicectl
// (Xcode < 15) simply yields no physical devices.
func (p *Provider) physicalDevices(ctx context.Context) ([]device.Device, error) {
	out, stderr, err := p.Run.Run(ctx, "xcrun", "devicectl", "list", "devices", "--json-output", "/dev/stdout", "--quiet")
	if err != nil {
		return nil, fmt.Errorf("devicectl: %w: %s", err, stderr)
	}
	return parseDevicectl(out)
}

func parseDevicectl(b []byte) ([]device.Device, error) {
	var doc struct {
		Result struct {
			Devices []struct {
				Identifier string `json:"identifier"`
				Dev        struct {
					Name string `json:"name"`
					OS   string `json:"osVersionNumber"`
				} `json:"deviceProperties"`
				HW struct {
					UDID      string `json:"udid"`
					Marketing string `json:"marketingName"`
					Reality   string `json:"reality"`
					Type      string `json:"deviceType"`
				} `json:"hardwareProperties"`
				Conn struct {
					Pairing   string `json:"pairingState"`
					Tunnel    string `json:"tunnelState"`
					Transport string `json:"transportType"`
				} `json:"connectionProperties"`
			} `json:"devices"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse devicectl: %w", err)
	}
	var out []device.Device
	for _, d := range doc.Result.Devices {
		if d.HW.Reality != "physical" || d.Conn.Pairing != "paired" {
			continue
		}
		id := d.HW.UDID
		if id == "" {
			id = d.Identifier
		}
		dev := device.Device{
			ID: id, Name: d.Dev.Name, Platform: device.IOS, Kind: device.Physical,
			OSVersion: d.Dev.OS, State: device.Shutdown, Slim: device.NotApplicable,
			Model: d.HW.Marketing, Warnings: []string{},
		}
		if d.Conn.Tunnel == "connected" {
			dev.State = device.Booted
			if d.Conn.Transport != "" {
				dev.Model += " · " + d.Conn.Transport
			}
		}
		out = append(out, dev)
	}
	return out, nil
}
