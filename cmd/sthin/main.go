// Command sthin boots iOS simulators and Android emulators already slimmed and
// restores them to stock.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/complete"
	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/host"
	"github.com/wktorbert/sthin/internal/lease"
	"github.com/wktorbert/sthin/internal/state"
	"github.com/wktorbert/sthin/internal/wiring"
)

var version = "0.1.0-dev"

// exitError carries a specific exit code up to main.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func usageErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", device.ErrUsage, fmt.Sprintf(format, a...))
}

// app holds what every subcommand needs.
type app struct {
	reg   *device.Registry
	ctx   context.Context
	prefs state.Store // per-device category preferences
	run   device.Runner
	env   host.Env
	cache complete.Cache // last device listing, for shell completion
}

// requireVirtual rejects physical devices for operations that modify a device.
func requireVirtual(d device.Device, op string) error {
	if d.Kind == device.Physical {
		return usageErr("%s is a physical device; Sthin never modifies physical devices (%s refused)", d.Name, op)
	}
	return nil
}

func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "sthin",
		Short:         "Boot iOS simulators and Android emulators already slimmed",
		Long:          "Sthin boots iOS simulators and Android emulators already slimmed, from one list,\nand restores them to stock with one command.\n\nExit codes: 0 success, 1 failure, 2 usage error or unknown device, 3 no device available to lease.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDefault(a, cmd)
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageErr("%v", err) })
	root.AddCommand(
		newListCmd(a),
		newBootCmd(a),
		newRestoreCmd(a),
		newShutdownCmd(a),
		newMeasureCmd(a),
		newAdbCmd(a),
		newRunCmd(a),
		newLogsCmd(a),
		newRenameCmd(a),
		newDeleteCmd(a),
		newMCPCmd(a),
		newServeCmd(a),
		newLeaseCmd(a),
		newReleaseCmd(a),
		newLeasesCmd(a),
		newDoctorCmd(a),
		newProfileCmd(a),
		&cobra.Command{
			Use:   "version",
			Short: "Print the version",
			Args:  cobra.NoArgs,
			Run:   func(cmd *cobra.Command, _ []string) { fmt.Fprintln(cmd.OutOrStdout(), "sthin", version) },
		},
	)
	attachCompletions(root, a)
	return root
}

// exactArgs wraps cobra.ExactArgs so wrong arity is a usage error (exit 2).
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(n)(cmd, args); err != nil {
			return usageErr("%v", err)
		}
		return nil
	}
}

func exitCode(err error) int {
	var ee *exitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		return ee.code
	case errors.Is(err, device.ErrUsage):
		return 2
	case errors.Is(err, lease.ErrNoDevice):
		return 3
	default:
		return 1
	}
}

func main() {
	a := &app{reg: wiring.NewRegistry(), ctx: context.Background(), prefs: state.Default(), run: wiring.NewRunner(), env: wiring.NewEnv()}
	a.cache = newCache(a.prefs.Home)
	a.reg.AfterList = a.cache.Write
	err := newRootCmd(a).Execute()
	if err != nil {
		var ee *exitError
		if !errors.As(err, &ee) || ee.err != nil {
			fmt.Fprintln(os.Stderr, "sthin:", err)
		}
	}
	os.Exit(exitCode(err))
}
