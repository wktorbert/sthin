package main

import (
	"encoding/json"
	"fmt"
	"github.com/wktorbert/sthin/internal/state"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wktorbert/sthin/internal/device"
)

// stagesJSON is the --json output of boot and restore.
type stagesJSON struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	Platform      device.Platform `json:"platform"`
	OK            bool            `json:"ok"`
	Error         string          `json:"error,omitempty"`
	Stages        []device.Stage  `json:"stages"`
	FootprintMB   *int            `json:"footprint_mb"`
}

// stageCollector prints progress to stderr (human mode) and keeps final stage results.
type stageCollector struct {
	errw   io.Writer
	quiet  bool
	stages []device.Stage
	fp     *int
}

func (c *stageCollector) report(s device.Stage) {
	if s.Status == device.StageRunning {
		if !c.quiet {
			fmt.Fprintf(c.errw, "  … %s\n", s.Name)
		}
		return
	}
	c.stages = append(c.stages, s)
	if s.Measurement != nil {
		fp := s.Measurement.FootprintMB
		c.fp = &fp
	}
	if c.quiet {
		return
	}
	line := fmt.Sprintf("  %-4s %s", s.Status, s.Name)
	if s.Detail != "" {
		line += ": " + s.Detail
	}
	if s.Command != "" {
		line += "\n       command: " + s.Command
	}
	fmt.Fprintln(c.errw, line)
}

func (c *stageCollector) finish(cmd *cobra.Command, d device.Device, asJSON bool, err error) error {
	if asJSON {
		doc := stagesJSON{SchemaVersion: 1, ID: d.ID, Platform: d.Platform, OK: err == nil, Stages: c.stages, FootprintMB: c.fp}
		if doc.Stages == nil {
			doc.Stages = []device.Stage{}
		}
		if err != nil {
			doc.Error = err.Error()
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(doc); encErr != nil {
			return encErr
		}
		if err != nil {
			return &exitError{code: exitCode(err), err: nil}
		}
		return nil
	}
	if err == nil && c.fp != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %d MB\n", d.Name, *c.fp)
	}
	return err
}

func newBootCmd(a *app) *cobra.Command {
	var opts device.BootOptions
	var asJSON, remember, coldBoot, audio, lowRAM bool
	cmd := &cobra.Command{
		Use:   "boot <id|name>",
		Short: "Boot a device slimmed (or stock with --stock)",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			if err := requireVirtual(d, "boot"); err != nil {
				return err
			}
			if opts.RAMMB < 0 {
				return usageErr("--ram must be positive")
			}
			// --except given (even empty) is an explicit list; absent means the
			// device's saved preference (nil).
			if cmd.Flags().Changed("except") {
				kept := []string{}
				for _, c := range opts.Except {
					if c = strings.TrimSpace(c); c != "" {
						kept = append(kept, c)
					}
				}
				opts.Except = kept
			} else {
				opts.Except = nil
			}
			// Launch switches: only flags that were passed become explicit; the
			// rest come from the AVD's saved preference.
			if cmd.Flags().Changed("cold-boot") {
				opts.Launch.ColdBoot = device.BoolPtr(coldBoot)
			}
			if cmd.Flags().Changed("audio") {
				opts.Launch.Audio = device.BoolPtr(audio)
			}
			if cmd.Flags().Changed("lowram") {
				opts.Launch.LowRAM = device.BoolPtr(lowRAM)
			}
			if cmd.Flags().Changed("headless") {
				opts.Launch.Headless = device.BoolPtr(opts.Headless)
			}
			if cmd.Flags().Changed("ram") {
				opts.Launch.RAMMB = device.IntPtr(opts.RAMMB)
			}
			if remember {
				if cmd.Flags().Changed("except") {
					if err := a.prefs.SavePrefs(state.Prefs{ID: d.ID, Except: opts.Except}); err != nil {
						return err
					}
				}
				if !opts.Launch.IsZero() {
					if err := a.prefs.MergeLaunch(d.ID, opts.Launch); err != nil {
						return err
					}
				}
			}
			c := &stageCollector{errw: cmd.ErrOrStderr(), quiet: asJSON}
			if !asJSON {
				mode := "slim"
				if opts.Stock {
					mode = "stock"
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "booting %s (%s) %s\n", d.Name, d.ID, mode)
			}
			err = p.Boot(a.ctx, d.ID, opts, c.report)
			return c.finish(cmd, d, asJSON, err)
		},
	}
	cmd.Flags().BoolVar(&opts.Stock, "stock", false, "boot without slimming (restores first if Sthin changed the device)")
	cmd.Flags().StringSliceVar(&opts.Except, "except", nil, "`categories` to leave enabled, comma-separated IDs from sthin profile")
	cmd.Flags().IntVar(&opts.RAMMB, "ram", 0, "Android guest RAM in MB (default 1024)")
	cmd.Flags().BoolVar(&opts.Headless, "headless", false, "do not open a window (no Simulator.app; emulator -no-window)")
	cmd.Flags().Bool("wait", true, "block until the device is ready (always on; kept for scripts)")
	cmd.Flags().StringVar(&opts.Name, "name", "", "rename the device before booting (simctl rename / avd.ini.displayname)")
	cmd.Flags().BoolVar(&coldBoot, "cold-boot", false, "Android: ignore the quick-boot snapshot (-no-snapshot-load)")
	cmd.Flags().BoolVar(&audio, "audio", false, "Android: keep audio on (default off)")
	cmd.Flags().BoolVar(&lowRAM, "lowram", false, "Android: pass -lowram (off by default; some images fail to boot with it)")
	cmd.Flags().BoolVar(&remember, "remember", false, "save the --except and Android launch flags given here as this device's defaults for later boots")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newRestoreCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "restore <id|name>",
		Short: "Undo everything Sthin changed on a device",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			if err := requireVirtual(d, "restore"); err != nil {
				return err
			}
			c := &stageCollector{errw: cmd.ErrOrStderr(), quiet: asJSON}
			err = p.Restore(a.ctx, d.ID, c.report)
			if err == nil && !asJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "%s restored to stock\n", d.Name)
			}
			return c.finish(cmd, d, asJSON, err)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newShutdownCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "shutdown <id|name>",
		Short: "Shut a device down",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			if err := requireVirtual(d, "shutdown"); err != nil {
				return err
			}
			if err := p.Shutdown(a.ctx, d.ID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s shut down\n", d.Name)
			return nil
		},
	}
}
