package serve

import (
	"context"
	"fmt"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/doctor"
	"github.com/wktorbert/sthin/internal/profile"
)

// Host-level methods: doctor, profiles, saved preferences, leases, wireless ADB.

func (s *Server) doctor(r *request) (any, error) {
	if s.Doctor == nil {
		return nil, fmt.Errorf("doctor is not available in this session")
	}
	checks := s.Doctor(r.ctx)
	if checks == nil {
		checks = []doctor.Check{}
	}
	return map[string]any{"checks": checks, "blocking": doctor.Blocking(checks)}, nil
}

func (s *Server) profile(r *request) (any, error) {
	var in platformIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	p, err := profile.Load(in.Platform)
	if err != nil {
		return nil, invalidParams("%v", err)
	}
	return p, nil
}

// prefsGet returns the Categories a device keeps enabled on a slim boot, so
// the category picker opens pre-ticked. saved is false when none was stored.
func (s *Server) prefsGet(r *request) (any, error) {
	var in idIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	p, saved, err := s.Prefs.LoadPrefs(in.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": in.ID, "except": p.Except, "saved": saved}, nil
}

func (s *Server) leases(r *request) (any, error) {
	all, err := s.Pool.All()
	if err != nil {
		return nil, err
	}
	return map[string]any{"leases": all}, nil
}

type adbIn struct {
	Addr string `json:"addr"`
	Code string `json:"code,omitempty"`
}

// wireless finds the Android Provider's wireless ADB seam, refusing when
// Android is unavailable on this host.
func (s *Server) wireless() (device.WirelessADB, error) {
	p, ok := s.Reg.Provider(device.Android)
	if !ok {
		return nil, fmt.Errorf("%w: no Android provider", device.ErrUsage)
	}
	w, ok := p.(device.WirelessADB)
	if !ok {
		return nil, fmt.Errorf("%w: Android provider has no wireless debugging", device.ErrUsage)
	}
	if ok, reason := p.Available(); !ok {
		return nil, fmt.Errorf("%w: android unavailable: %s", device.ErrUsage, reason)
	}
	return w, nil
}

func (s *Server) adb(r *request, needCode bool, op func(context.Context, device.WirelessADB, adbIn) (string, error)) (any, error) {
	var in adbIn
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	if in.Addr == "" || (needCode && in.Code == "") {
		return nil, invalidParams("addr (host:port) is required, and code for pairing")
	}
	w, err := s.wireless()
	if err != nil {
		return nil, err
	}
	msg, err := op(r.ctx, w, in)
	if err != nil {
		return nil, err
	}
	return map[string]any{"message": msg}, nil
}

func (s *Server) adbPair(r *request) (any, error) {
	return s.adb(r, true, func(ctx context.Context, w device.WirelessADB, in adbIn) (string, error) {
		return w.Pair(ctx, in.Addr, in.Code)
	})
}

func (s *Server) adbConnect(r *request) (any, error) {
	return s.adb(r, false, func(ctx context.Context, w device.WirelessADB, in adbIn) (string, error) {
		return w.Connect(ctx, in.Addr)
	})
}

func (s *Server) adbDisconnect(r *request) (any, error) {
	return s.adb(r, false, func(ctx context.Context, w device.WirelessADB, in adbIn) (string, error) {
		return w.Disconnect(ctx, in.Addr)
	})
}
