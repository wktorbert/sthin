package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrUsage marks errors that should exit with code 2 (unknown or ambiguous device, bad arguments).
var ErrUsage = errors.New("usage")

// Registry holds the Providers for this host and resolves Devices across them.
type Registry struct {
	Providers []Provider
	// AfterList, when set, sees the result of every ListAll. Shell completion
	// caches the last listing through it so a running TUI or desktop app keeps
	// completions fresh for every terminal.
	AfterList func([]PlatformList)
}

// PlatformList is the result of listing one Provider.
type PlatformList struct {
	Platform  Platform
	Available bool
	Reason    string
	Devices   []Device
	Err       error
}

// ListAll lists every Provider, iOS before Android, never failing as a whole.
func (r *Registry) ListAll(ctx context.Context) []PlatformList {
	out := make([]PlatformList, 0, len(r.Providers))
	for _, p := range r.Providers {
		pl := PlatformList{Platform: p.Platform()}
		pl.Available, pl.Reason = p.Available()
		if pl.Available {
			pl.Devices, pl.Err = p.List(ctx)
		}
		out = append(out, pl)
	}
	if r.AfterList != nil {
		r.AfterList(out)
	}
	return out
}

// Provider returns the Provider for a platform.
func (r *Registry) Provider(pl Platform) (Provider, bool) {
	for _, p := range r.Providers {
		if p.Platform() == pl {
			return p, true
		}
	}
	return nil, false
}

// Resolve finds a Device by exact ID, then by exact name, across available Providers.
// Unknown and ambiguous names wrap ErrUsage.
func (r *Registry) Resolve(ctx context.Context, idOrName string) (Provider, Device, error) {
	type hit struct {
		p Provider
		d Device
	}
	var byName []hit
	for _, pl := range r.ListAll(ctx) {
		if !pl.Available || pl.Err != nil {
			continue
		}
		p, _ := r.Provider(pl.Platform)
		for _, d := range pl.Devices {
			if d.ID == idOrName {
				return p, d, nil
			}
			if d.Name == idOrName {
				byName = append(byName, hit{p, d})
			}
		}
	}
	switch len(byName) {
	case 0:
		return nil, Device{}, fmt.Errorf("%w: unknown device %q", ErrUsage, idOrName)
	case 1:
		return byName[0].p, byName[0].d, nil
	default:
		ids := make([]string, len(byName))
		for i, h := range byName {
			ids[i] = h.d.ID
		}
		return nil, Device{}, fmt.Errorf("%w: name %q is ambiguous, use an ID: %s", ErrUsage, idOrName, strings.Join(ids, ", "))
	}
}
