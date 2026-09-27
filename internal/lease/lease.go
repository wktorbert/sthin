// Package lease is the device pool for agents: a device is leased for a TTL,
// used, and released; an expired lease frees the device even if the agent
// that held it crashed. Leases are files under $STHIN_HOME/leases, one per
// device, so every Sthin process on the host sees the same pool.
package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wktorbert/sthin/internal/device"
)

// ErrNoDevice means nothing idle matched the request; exit code 3 on the CLI.
var ErrNoDevice = errors.New("no device available to lease")

// Lease is one agent's hold on one Device.
type Lease struct {
	ID        string    `json:"id"`     // lease token
	Device    string    `json:"device"` // Device ID
	Platform  string    `json:"platform"`
	Owner     string    `json:"owner"` // free text: agent or session name
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Expired reports whether the TTL has passed.
func (l Lease) Expired(now time.Time) bool { return !now.Before(l.ExpiresAt) }

// Pool stores leases under <Home>/leases.
type Pool struct {
	Home string
	Now  func() time.Time // nil = time.Now
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func (p Pool) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p Pool) path(deviceID string) string {
	return filepath.Join(p.Home, "leases", unsafe.ReplaceAllString(deviceID, "_")+".json")
}

// Get returns the active lease on deviceID; an expired one is removed and
// reported as absent.
func (p Pool) Get(deviceID string) (Lease, bool, error) {
	b, err := os.ReadFile(p.path(deviceID))
	if errors.Is(err, fs.ErrNotExist) {
		return Lease{}, false, nil
	}
	if err != nil {
		return Lease{}, false, err
	}
	var l Lease
	if err := json.Unmarshal(b, &l); err != nil {
		return Lease{}, false, err
	}
	if l.Expired(p.now()) {
		_ = os.Remove(p.path(deviceID))
		return Lease{}, false, nil
	}
	return l, true, nil
}

// All returns every active lease, pruning expired ones.
func (p Pool) All() ([]Lease, error) {
	entries, err := os.ReadDir(filepath.Join(p.Home, "leases"))
	if errors.Is(err, fs.ErrNotExist) {
		return []Lease{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Lease{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(p.Home, "leases", e.Name()))
		if err != nil {
			continue
		}
		var l Lease
		if json.Unmarshal(b, &l) != nil {
			continue
		}
		if l.Expired(p.now()) {
			_ = os.Remove(filepath.Join(p.Home, "leases", e.Name()))
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// Grant records a lease; it fails when the device already has an active one.
func (p Pool) Grant(deviceID, platform, owner string, ttl time.Duration) (Lease, error) {
	if _, held, err := p.Get(deviceID); err != nil {
		return Lease{}, err
	} else if held {
		return Lease{}, fmt.Errorf("%s is already leased", deviceID)
	}
	now := p.now()
	l := Lease{ID: fmt.Sprintf("%s-%d", unsafe.ReplaceAllString(deviceID, "_"), now.UnixNano()), Device: deviceID, Platform: platform, Owner: owner, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	if err := os.MkdirAll(filepath.Dir(p.path(deviceID)), 0o755); err != nil {
		return Lease{}, err
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	tmp := p.path(deviceID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return Lease{}, err
	}
	return l, os.Rename(tmp, p.path(deviceID))
}

// Release drops the lease on deviceID; releasing an unleased device is a no-op.
func (p Pool) Release(deviceID string) error {
	err := os.Remove(p.path(deviceID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Acquire picks an idle virtual device (booted first, then shut down), boots
// it through boot when needed, and grants the lease. platform "" means any.
func Acquire(ctx context.Context, reg *device.Registry, pool Pool, platform device.Platform, owner string, ttl time.Duration,
	boot func(ctx context.Context, p device.Provider, d device.Device) error) (device.Device, Lease, error) {
	var candidates []struct {
		p device.Provider
		d device.Device
	}
	for _, pl := range reg.ListAll(ctx) {
		if !pl.Available || pl.Err != nil || (platform != "" && pl.Platform != platform) {
			continue
		}
		p, _ := reg.Provider(pl.Platform)
		for _, d := range pl.Devices {
			if d.Kind == device.Physical || hasWarning(d, "data missing") {
				continue
			}
			if _, held, _ := pool.Get(d.ID); held {
				continue
			}
			candidates = append(candidates, struct {
				p device.Provider
				d device.Device
			}{p, d})
		}
	}
	if len(candidates) == 0 {
		return device.Device{}, Lease{}, ErrNoDevice
	}
	pick := -1
	for i, c := range candidates {
		if c.d.State == device.Booted {
			pick = i
			break
		}
	}
	if pick < 0 {
		for i, c := range candidates {
			if c.d.State == device.Shutdown {
				pick = i
				break
			}
		}
	}
	if pick < 0 {
		return device.Device{}, Lease{}, ErrNoDevice
	}
	c := candidates[pick]
	// Hold the lease before booting so a parallel caller cannot pick the same device.
	l, err := pool.Grant(c.d.ID, string(c.d.Platform), owner, ttl)
	if err != nil {
		return device.Device{}, Lease{}, err
	}
	if c.d.State != device.Booted {
		if err := boot(ctx, c.p, c.d); err != nil {
			_ = pool.Release(c.d.ID)
			return device.Device{}, Lease{}, err
		}
		c.d.State = device.Booted
	}
	return c.d, l, nil
}

func hasWarning(d device.Device, sub string) bool {
	for _, w := range d.Warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}
