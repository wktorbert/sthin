package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wktorbert/lean-sim/internal/device"
)

// newAdbCmd exposes Android wireless debugging: pair, connect, disconnect.
func newAdbCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "adb", Short: "Android wireless debugging (pair, connect, disconnect)"}
	wireless := func() (device.WirelessADB, error) {
		for _, p := range a.reg.Providers {
			if w, ok := p.(device.WirelessADB); ok {
				if ok, reason := p.Available(); !ok {
					return nil, usageErr("android unavailable: %s", reason)
				}
				return w, nil
			}
		}
		return nil, usageErr("no Android provider")
	}
	cmd.AddCommand(&cobra.Command{
		Use: "pair <host:port> <code>", Short: "Pair with a phone showing a Wireless debugging pairing code", Args: exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := wireless()
			if err != nil {
				return err
			}
			out, err := w.Pair(a.ctx, args[0], args[1])
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}, &cobra.Command{
		Use: "connect <host:port>", Short: "Connect to a paired phone over Wi-Fi", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := wireless()
			if err != nil {
				return err
			}
			out, err := w.Connect(a.ctx, args[0])
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}, &cobra.Command{
		Use: "disconnect <host:port>", Short: "Drop a Wi-Fi connection", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := wireless()
			if err != nil {
				return err
			}
			out, err := w.Disconnect(a.ctx, args[0])
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	})
	return cmd
}
