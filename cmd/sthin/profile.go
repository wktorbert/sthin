package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wktorbert/lean-sim/internal/profile"
)

type profileJSON struct {
	SchemaVersion int `json:"schema_version"`
	*profile.Profile
}

func newProfileCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "profile <ios|android>",
		Short: "Show a platform's Profile: categories, item counts, never-disable set",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(args[0])
			if err != nil {
				return usageErr("%v", err)
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(profileJSON{SchemaVersion: 1, Profile: p})
			}
			keys := make([]string, 0, len(p.ValidatedAgainst))
			for k := range p.ValidatedAgainst {
				keys = append(keys, k+" "+p.ValidatedAgainst[k])
			}
			sort.Strings(keys)
			fmt.Fprintf(out, "%s profile v%d, validated against %s\n\n", p.Platform, p.Version, strings.Join(keys, ", "))
			for _, c := range p.Categories {
				def := "on"
				if !c.Default {
					def = "off"
				}
				fmt.Fprintf(out, "  %-13s %-40s %3d items  %s\n", c.ID, c.Name, len(c.Items), def)
			}
			fmt.Fprintf(out, "\n  never disabled: %s\n", strings.Join(p.NeverDisable, ", "))
			if len(p.Aggressive) > 0 {
				fmt.Fprintf(out, "  aggressive (off by default): %s\n", strings.Join(p.Aggressive, ", "))
			}
			fmt.Fprintf(out, "  default slim boot disables %d items\n", len(profile.DesiredSet(p, nil)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}
