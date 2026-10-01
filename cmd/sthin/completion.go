package main

import (
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/complete"
	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/profile"
)

// cacheTTL is how long a device listing serves completions before a TAB
// relists. Long enough that a burst of commands pays once, short enough that
// a device booted outside Sthin shows its real state soon.
const cacheTTL = 30 * time.Second

// mutating commands drop the completion cache when they succeed, so the next
// TAB reflects the new state instead of waiting the TTL out.
var mutating = map[string]bool{"boot": true, "shutdown": true, "restore": true, "delete": true, "rename": true, "lease": true, "release": true}

// newCache places the completion cache under the Sthin home.
func newCache(home string) complete.Cache {
	if home == "" {
		return complete.Cache{}
	}
	return complete.Cache{Path: filepath.Join(home, "cache", "devices.json"), TTL: cacheTTL}
}

// devices returns the cached listing when fresh, otherwise lists through the
// registry, whose AfterList hook refreshes the cache as a side effect.
func (a *app) devices() []device.Device {
	if devs, ok := a.cache.Read(); ok {
		return devs
	}
	var out []device.Device
	for _, pl := range a.reg.ListAll(a.ctx) {
		if pl.Available && pl.Err == nil {
			out = append(out, pl.Devices...)
		}
	}
	return out
}

// completeDevice completes the first positional argument (or a flag value)
// with the Devices passing f.
func (a *app) completeDevice(f complete.Filter) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return complete.Devices(a.devices(), f, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeLeased offers the Devices with an active lease.
func (a *app) completeLeased(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	leases, err := a.pool().All()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	ids := make([]string, 0, len(leases))
	for _, l := range leases {
		ids = append(ids, l.Device)
	}
	return complete.Devices(a.devices(), complete.Only(ids), toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeExcept offers the Profile categories of the typed Device's platform,
// or both platforms' categories when no single Device is named yet.
func (a *app) completeExcept(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var platforms []string
	if len(args) > 0 {
		if pl := complete.PlatformOf(a.devices(), args[0]); pl != "" {
			platforms = []string{string(pl)}
		}
	}
	if platforms == nil {
		platforms = complete.Platforms
	}
	var ids []string
	for _, pl := range platforms {
		if p, err := profile.Load(pl); err == nil {
			ids = append(ids, p.CategoryIDs()...)
		}
	}
	return complete.ListValues(ids, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeValues(all []string) cobra.CompletionFunc {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return complete.Values(all, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

// attachCompletions wires dynamic completion onto the commands by name, so
// the command files stay about their command. Unknown names are skipped,
// which keeps this list harmless if a command is renamed.
func attachCompletions(root *cobra.Command, a *app) {
	byArg := map[string]cobra.CompletionFunc{
		"boot":     a.completeDevice(complete.Virtual),
		"restore":  a.completeDevice(complete.Virtual),
		"rename":   a.completeDevice(complete.Virtual),
		"delete":   a.completeDevice(complete.ShutdownVirtual),
		"shutdown": a.completeDevice(complete.Booted),
		"measure":  a.completeDevice(complete.Booted),
		"logs":     a.completeDevice(complete.Booted),
		"release":  a.completeLeased,
	}
	for _, cmd := range root.Commands() {
		if fn, ok := byArg[cmd.Name()]; ok {
			cmd.ValidArgsFunction = fn
		}
		if cmd.Flags().Lookup("device") != nil {
			_ = cmd.RegisterFlagCompletionFunc("device", a.completeDevice(complete.Booted))
		}
		if cmd.Flags().Lookup("except") != nil {
			_ = cmd.RegisterFlagCompletionFunc("except", a.completeExcept)
		}
		if cmd.Flags().Lookup("level") != nil {
			_ = cmd.RegisterFlagCompletionFunc("level", completeValues(complete.Levels))
		}
		if cmd.Flags().Lookup("platform") != nil {
			_ = cmd.RegisterFlagCompletionFunc("platform", completeValues(complete.Platforms))
		}
	}
	// PersistentPostRunE only runs after a RunE that returned nil, so a failed
	// command keeps the cache and a successful change drops it.
	root.PersistentPostRunE = func(cmd *cobra.Command, _ []string) error {
		if mutating[cmd.Name()] {
			return a.cache.Drop()
		}
		return nil
	}
}
