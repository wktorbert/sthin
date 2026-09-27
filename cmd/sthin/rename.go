package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/device"
)

// newRenameCmd is `sthin rename <id> <name>`: simctl rename on iOS,
// avd.ini.displayname on Android. Physical devices are refused.
func newRenameCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <id|name> <new name>",
		Short: "Rename a simulator or AVD",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			if err := requireVirtual(d, "rename"); err != nil {
				return err
			}
			rn, ok := p.(device.Renamer)
			if !ok {
				return fmt.Errorf("%s provider cannot rename devices", d.Platform)
			}
			if err := rn.Rename(a.ctx, d.ID, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s renamed to %q\n", d.Name, args[1])
			if d.Platform == device.Android && d.State == device.Booted {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: the emulator window shows the new name from its next start")
			}
			return nil
		},
	}
}
