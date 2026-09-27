package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/wktorbert/lean-sim/internal/serve"
	"github.com/wktorbert/lean-sim/internal/wiring"
)

// newServeCmd starts the Serve session the Desktop (or any other process)
// uses to reach Providers: JSON-RPC 2.0 over stdin/stdout, one message per
// line, with live progress, log and device-list notifications. stderr is for
// human-readable diagnostics only. See docs/serve-protocol.md.
func newServeCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve Sthin's operations to another process over stdio (JSON-RPC, used by the desktop app)",
		Long:  "Runs a long-lived JSON-RPC 2.0 session on stdin/stdout with live notifications for boot progress, log lines and device list changes. The desktop app starts it as a sidecar. Every method uses the same code path as the CLI, TUI and MCP. Protocol: docs/serve-protocol.md.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d := wiring.NewDoctor()
			s := serve.New(serve.Deps{
				Reg:     a.reg,
				Pool:    a.pool(),
				Prefs:   a.prefs,
				Version: version,
				Doctor:  d.Run,
			})
			return s.Run(a.ctx, os.Stdin, os.Stdout)
		},
	}
}
