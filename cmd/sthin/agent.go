package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/lease"
	"github.com/wktorbert/sthin/internal/mcpserver"
)

// Agent-facing commands: the MCP server and the lease pool.

func (a *app) pool() lease.Pool { return lease.Pool{Home: a.prefs.Home} }

func newMCPCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve Sthin's operations to agents over MCP (stdio)",
		Long:  "Runs a Model Context Protocol server on stdin/stdout. Register it in an agent's MCP config as `sthin mcp`. Every tool uses the same code path as the CLI and TUI.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := mcpserver.New(mcpserver.Deps{Reg: a.reg, Pool: a.pool(), Version: version, Run: a.run, Env: a.env})
			return s.Run(a.ctx, &mcp.StdioTransport{})
		},
	}
}

func newLeaseCmd(a *app) *cobra.Command {
	var platform, owner string
	var ttl time.Duration
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "lease",
		Short: "Acquire an idle device for a script or agent (boots one slim and headless if none is running)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if platform != "" && platform != "ios" && platform != "android" {
				return usageErr("--platform must be ios or android")
			}
			c := &stageCollector{errw: cmd.ErrOrStderr(), quiet: asJSON}
			d, l, err := lease.Acquire(a.ctx, a.reg, a.pool(), device.Platform(platform), owner, ttl, func(ctx context.Context, p device.Provider, d device.Device) error {
				return p.Boot(ctx, d.ID, device.BootOptions{Headless: true}, c.report)
			})
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema_version": 1, "device": d, "lease": l})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\tuntil %s\n", d.ID, d.Name, d.Platform, l.ExpiresAt.Local().Format(time.RFC3339))
			return nil
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "", "ios or android (default any)")
	cmd.Flags().StringVar(&owner, "owner", "", "who holds the lease, e.g. an agent or CI job name")
	cmd.Flags().DurationVar(&ttl, "ttl", 30*time.Minute, "lease lifetime; the device is free again after this even if never released")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newReleaseCmd(a *app) *cobra.Command {
	var shutdown bool
	cmd := &cobra.Command{
		Use:   "release <id|name>",
		Short: "Release a leased device",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			if err := a.pool().Release(d.ID); err != nil {
				return err
			}
			if shutdown {
				if err := requireVirtual(d, "shutdown"); err != nil {
					return err
				}
				if err := p.Shutdown(a.ctx, d.ID); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "released %s\n", d.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&shutdown, "shutdown", false, "also shut the device down")
	return cmd
}

func newLeasesCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "leases",
		Short: "List active leases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := a.pool().All()
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema_version": 1, "leases": all})
			}
			if len(all) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no active leases")
				return nil
			}
			for _, l := range all {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\texpires %s\n", l.Device, l.Platform, l.Owner, l.ExpiresAt.Local().Format(time.RFC3339))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}
