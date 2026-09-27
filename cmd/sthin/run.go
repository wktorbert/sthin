package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wktorbert/lean-sim/internal/project"
)

// newRunCmd is `sthin run`: detect the project in the current directory and
// run it on a device.
func newRunCmd(a *app) *cobra.Command {
	var o project.Opts
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the app in this directory on a device (Flutter, React Native, Xcode, Gradle)",
		Long: `Detects the project in --dir (default: the current directory), boots the device if needed, then:
  Flutter / React Native: hands off to "flutter run" or "react-native run-*" for that device (hot reload works).
  Xcode / Gradle, or --no-handoff / --app: installs the newest debug build (.app / .apk) and launches it.
--url opens a deep link after launch (artifact mode only).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.Device == "" {
				return usageErr("--device is required (see sthin list)")
			}
			if o.Dir == "" {
				o.Dir, _ = os.Getwd()
			}
			c := &stageCollector{errw: cmd.ErrOrStderr(), quiet: asJSON}
			res, err := project.Run(a.ctx, project.Deps{Reg: a.reg, Run: a.run, Env: a.env}, o, c.report)
			if asJSON {
				doc := map[string]any{"schema_version": 1, "result": res, "stages": c.stages}
				if err != nil {
					doc["error"] = err.Error()
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if encErr := enc.Encode(doc); encErr != nil {
					return encErr
				}
				if err != nil {
					return &exitError{code: exitCode(err)}
				}
				return nil
			}
			if err != nil {
				return err
			}
			if res.Handoff == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%s running on %s\n", res.AppID, res.Device.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&o.Device, "device", "d", "", "device ID or exact name")
	cmd.Flags().StringVar(&o.Dir, "dir", "", "project directory (default: current)")
	cmd.Flags().StringVar(&o.App, "app", "", "built .app or .apk to install instead of detecting one")
	cmd.Flags().StringVar(&o.AppID, "app-id", "", "bundle identifier or package to launch (read from the artifact when omitted)")
	cmd.Flags().StringVar(&o.URL, "url", "", "deep link to open after launch")
	cmd.Flags().BoolVar(&o.NoHandoff, "no-handoff", false, "for Flutter/React Native: install the built app instead of running the framework command")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON (artifact mode)")
	return cmd
}
