package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/tui"
)

type platformJSON struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Error     string `json:"error,omitempty"`
}

type listJSON struct {
	SchemaVersion int                              `json:"schema_version"`
	Platforms     map[device.Platform]platformJSON `json:"platforms"`
	Devices       []device.Device                  `json:"devices"`
}

func newListCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List simulators and emulators with slim state and footprint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(a, cmd.OutOrStdout(), cmd.ErrOrStderr(), asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func runList(a *app, out, errw io.Writer, asJSON bool) error {
	lists := a.reg.ListAll(a.ctx)
	doc := listJSON{SchemaVersion: 1, Platforms: map[device.Platform]platformJSON{}, Devices: []device.Device{}}
	for _, pl := range lists {
		pj := platformJSON{Available: pl.Available, Reason: pl.Reason}
		if pl.Err != nil {
			pj.Error = pl.Err.Error()
		}
		doc.Platforms[pl.Platform] = pj
		doc.Devices = append(doc.Devices, pl.Devices...)
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PLATFORM\tKIND\tNAME\tOS\tSTATE\tSLIM\tFOOTPRINT\tID")
	for _, d := range doc.Devices {
		fp := ""
		if d.FootprintMB != nil {
			fp = fmt.Sprintf("%d MB", *d.FootprintMB)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", d.Platform, d.Kind, d.Name, d.OSVersion, d.State, d.Slim, fp, d.ID)
	}
	tw.Flush()
	for _, pl := range lists {
		switch {
		case !pl.Available:
			fmt.Fprintf(errw, "%s: unavailable: %s\n", pl.Platform, pl.Reason)
		case pl.Err != nil:
			fmt.Fprintf(errw, "%s: %v\n", pl.Platform, pl.Err)
		}
		for _, d := range pl.Devices {
			if len(d.Warnings) > 0 {
				fmt.Fprintf(errw, "warning: %s (%s): %s\n", d.Name, d.ID, strings.Join(d.Warnings, "; "))
			}
		}
	}
	return nil
}

// runDefault is `lean` with no arguments: the TUI when stdin and stdout are a
// terminal, otherwise the same output as `lean list`.
func runDefault(a *app, cmd *cobra.Command) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return runList(a, cmd.OutOrStdout(), cmd.ErrOrStderr(), false)
	}
	_, err := tea.NewProgram(tui.New(a.ctx, a.reg, version, a.prefs, a.run, a.env), tea.WithAltScreen()).Run()
	return err
}
