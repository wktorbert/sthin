// Package ops holds the rules every out-of-process Surface (MCP, Serve) applies
// before calling a Provider: how a device ID or name resolves, that physical
// devices are never modified, how a boot is run and measured, and what a device
// listing carries. Keeping them here means MCP and Serve cannot drift apart.
package ops

import (
	"context"
	"fmt"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/lease"
)

// ResolveVirtual finds a device by ID or exact name and refuses physical
// devices for op, wrapping device.ErrUsage so callers map it to exit code 2.
func ResolveVirtual(ctx context.Context, reg *device.Registry, id, op string) (device.Provider, device.Device, error) {
	p, d, err := reg.Resolve(ctx, id)
	if err != nil {
		return nil, device.Device{}, err
	}
	if d.Kind == device.Physical {
		return nil, device.Device{}, fmt.Errorf("%w: %s is a physical device; Sthin never modifies physical devices (%s refused)", device.ErrUsage, d.Name, op)
	}
	return p, d, nil
}

// Controller returns the Provider's Controller seam or a clear error.
func Controller(p device.Provider) (device.Controller, error) {
	c, ok := p.(device.Controller)
	if !ok {
		return nil, fmt.Errorf("%s provider cannot drive devices", p.Platform())
	}
	return c, nil
}

// BootResult is what a boot reports back: the device, every completed stage,
// and the footprint from the final measure stage.
type BootResult struct {
	Device      device.Device  `json:"device"`
	Stages      []device.Stage `json:"stages"`
	FootprintMB *int           `json:"footprint_mb,omitempty"`
	Error       string         `json:"error,omitempty"`
	// Err is the boot's error as returned, kept typed (for example a
	// *device.StageError naming the failed stage and command).
	Err error `json:"-"`
}

// RunBoot boots d with opts, collecting completed stages into the result. Every
// stage, including running ones, is also forwarded to live when it is non-nil,
// so a streaming Surface can show progress while the boot is in flight.
func RunBoot(ctx context.Context, p device.Provider, d device.Device, opts device.BootOptions, live device.Reporter) BootResult {
	res := BootResult{Device: d, Stages: []device.Stage{}}
	err := p.Boot(ctx, d.ID, opts, func(s device.Stage) {
		live.Report(s)
		if s.Status == device.StageRunning {
			return
		}
		res.Stages = append(res.Stages, s)
		if s.Measurement != nil {
			mb := s.Measurement.FootprintMB
			res.FootprintMB = &mb
		}
	})
	if err != nil {
		res.Error, res.Err = err.Error(), err
	} else {
		res.Device.State = device.Booted
	}
	return res
}

// PlatformStatus is one Provider's availability in a listing.
type PlatformStatus struct {
	Platform  device.Platform `json:"platform"`
	Available bool            `json:"available"`
	Reason    string          `json:"reason,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ListedDevice is a Device plus the lease held on it, if any.
type ListedDevice struct {
	device.Device
	Lease *lease.Lease `json:"lease,omitempty"`
}

// Listing is what devices_list returns on MCP and Serve.
type Listing struct {
	SchemaVersion int              `json:"schema_version"`
	Platforms     []PlatformStatus `json:"platforms"`
	Devices       []ListedDevice   `json:"devices"`
}

// List lists every Provider (or only platform when non-empty) and annotates
// each device with its active lease.
func List(ctx context.Context, reg *device.Registry, pool lease.Pool, platform string) Listing {
	out := Listing{SchemaVersion: 1, Platforms: []PlatformStatus{}, Devices: []ListedDevice{}}
	for _, pl := range reg.ListAll(ctx) {
		if platform != "" && string(pl.Platform) != platform {
			continue
		}
		ps := PlatformStatus{Platform: pl.Platform, Available: pl.Available, Reason: pl.Reason}
		if pl.Err != nil {
			ps.Error = pl.Err.Error()
		}
		out.Platforms = append(out.Platforms, ps)
		for _, d := range pl.Devices {
			ld := ListedDevice{Device: d}
			if l, held, _ := pool.Get(d.ID); held {
				ld.Lease = &l
			}
			out.Devices = append(out.Devices, ld)
		}
	}
	return out
}
