package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/logs"
)

// newLogsCmd is `lean logs`: a filtered snapshot, or a live follow until Ctrl-C.
func newLogsCmd(a *app) *cobra.Command {
	var follow, asJSON bool
	var filter, level, out string
	var lines int
	cmd := &cobra.Command{
		Use:   "logs <id|name>",
		Short: "Device log: unified log on iOS, logcat on Android; --follow streams live",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, d, err := a.reg.Resolve(a.ctx, args[0])
			if err != nil {
				return err
			}
			ctl, ok := p.(device.Controller)
			if !ok {
				return fmt.Errorf("%s provider cannot read logs", d.Platform)
			}
			f := logs.Filter{Text: filter}
			if level != "" {
				lvl, ok := logs.ParseLevel(level)
				if !ok {
					return usageErr("--level must be one of verbose, debug, info, warn, error, fatal")
				}
				f.MinLevel = lvl
			}
			var w io.Writer = cmd.OutOrStdout()
			if out != "" {
				fh, err := os.Create(out)
				if err != nil {
					return err
				}
				defer fh.Close()
				w = io.MultiWriter(w, fh)
			}
			bw := bufio.NewWriter(w)
			defer bw.Flush()
			emit := func(l logs.Line) {
				if !f.Match(l) {
					return
				}
				if asJSON {
					b, _ := json.Marshal(l)
					bw.Write(b)
					bw.WriteByte('\n')
				} else {
					bw.WriteString(logs.Format(l) + "\n")
				}
			}
			parse := logs.ParseLogcat
			if d.Platform == device.IOS {
				parse = logs.ParseIOS
			}
			if !follow {
				text, err := ctl.Logs(a.ctx, d.ID, lines)
				if err != nil {
					return err
				}
				for _, s := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
					if s != "" {
						emit(parse(s))
					}
				}
				return nil
			}
			ctx, stop := signal.NotifyContext(a.ctx, os.Interrupt)
			defer stop()
			ch, err := ctl.LogStream(ctx, d.ID)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "following %s logs (Ctrl-C to stop)\n", d.Name)
			for l := range ch {
				emit(l)
				if bw.Buffered() > 0 {
					bw.Flush()
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream live until Ctrl-C")
	cmd.Flags().StringVar(&filter, "filter", "", "case-insensitive substring to keep")
	cmd.Flags().StringVar(&level, "level", "", "minimum level: verbose, debug, info, warn, error, fatal")
	cmd.Flags().IntVarP(&lines, "lines", "n", 200, "snapshot size (without --follow)")
	cmd.Flags().StringVarP(&out, "out", "o", "", "also write the (filtered) lines to this file")
	cmd.Flags().BoolVar(&asJSON, "json", false, "one JSON object per line")
	return cmd
}
