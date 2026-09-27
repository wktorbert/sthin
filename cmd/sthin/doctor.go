package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/doctor"
	"github.com/wktorbert/sthin/internal/wiring"
)

type doctorJSON struct {
	SchemaVersion int            `json:"schema_version"`
	Checks        []doctor.Check `json:"checks"`
	Blocking      bool           `json:"blocking"`
}

func newDoctorCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the host toolchain and Profiles (exit 1 when a slim boot cannot work)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := wiring.NewDoctor().Run(a.ctx)
			blocking := doctor.Blocking(checks)
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(doctorJSON{SchemaVersion: 1, Checks: checks, Blocking: blocking}); err != nil {
					return err
				}
			} else {
				for _, c := range checks {
					fmt.Fprintf(out, "  %-4s %-24s %s\n", c.Status, c.Name, c.Detail)
				}
			}
			if blocking {
				return &exitError{code: 1, err: nil}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}
