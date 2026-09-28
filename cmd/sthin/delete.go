package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/ops"
)

// newDeleteCmd is `sthin delete <id|name>`: removes a simulator or AVD for
// good, with Sthin's saved records and any lease on it. There is no undo, so
// it asks first unless --yes is given. Physical devices are refused.
func newDeleteCmd(a *app) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id|name>",
		Short: "Delete a simulator or AVD for good (asks first; --yes skips)",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			if err := requireVirtual(d, "delete"); err != nil {
				return err
			}
			if !yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "Delete %s (%s) and all its data? [y/N] ", d.Name, d.ID)
				if !confirmed(cmd.InOrStdin()) {
					return usageErr("delete cancelled; pass --yes to skip the prompt")
				}
			}
			if err := ops.Delete(a.ctx, p, d, a.pool()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s deleted\n", d.Name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "delete without asking")
	return cmd
}

// confirmed reads one line and accepts only an explicit yes.
func confirmed(r io.Reader) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
