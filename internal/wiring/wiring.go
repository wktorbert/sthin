// Package wiring builds the real Providers and Doctor for this host. It is the
// CLI's only route to the platform packages and the real Runner, so cmd/sthin
// and internal/tui depend on device.Provider alone.
package wiring

import (
	"github.com/wktorbert/lean-sim/internal/android"
	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/doctor"
	"github.com/wktorbert/lean-sim/internal/exec"
	"github.com/wktorbert/lean-sim/internal/host"
	"github.com/wktorbert/lean-sim/internal/ios"
)

// NewRegistry returns iOS then Android Providers backed by the real host.
func NewRegistry() *device.Registry {
	run := exec.New()
	env := host.Real()
	return &device.Registry{Providers: []device.Provider{
		ios.New(run, env),
		android.New(run, env),
	}}
}

// NewRunner returns the real Runner (for hand-offs and artifact inspection).
func NewRunner() device.Runner { return exec.New() }

// NewEnv returns the real host environment.
func NewEnv() host.Env { return host.Real() }

// NewDoctor returns a Doctor over the real host.
func NewDoctor() doctor.Doctor {
	return doctor.Doctor{Env: host.Real(), Runner: exec.New()}
}
