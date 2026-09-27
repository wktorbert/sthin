package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/device"
)

type measureJSON struct {
	SchemaVersion int `json:"schema_version"`
	device.Measurement
}

func newMeasureCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "measure <id|name>",
		Short: "Print a running device's memory footprint",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			if err := requireVirtual(d, "measure"); err != nil {
				return err
			}
			notBooted := &exitError{code: 2, err: fmt.Errorf("%s (%s) is not booted", d.Name, d.ID)}
			if d.State != device.Booted {
				return notBooted
			}
			m, err := p.Measure(a.ctx, d.ID)
			if errors.Is(err, device.ErrNotBooted) {
				return notBooted
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(measureJSON{SchemaVersion: 1, Measurement: m})
			}
			printMeasurement(out, m)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func printMeasurement(out interface{ Write([]byte) (int, error) }, m device.Measurement) {
	fmt.Fprintf(out, "%d MB across %d processes\n", m.FootprintMB, m.ProcessCount)
	if m.DirtyMB != nil {
		fmt.Fprintf(out, "dirty: %d MB\n", *m.DirtyMB)
	}
	if m.GuestTotalMB != nil && m.GuestUsedMB != nil {
		fmt.Fprintf(out, "guest RAM: %d MB used of %d MB\n", *m.GuestUsedMB, *m.GuestTotalMB)
	}
}
