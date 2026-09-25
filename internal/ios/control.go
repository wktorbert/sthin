package ios

import (
	"context"
	"fmt"
	"strings"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/logs"
)

// device.Controller for simulators, all through simctl.

// Screenshot implements device.Controller.
func (p *Provider) Screenshot(ctx context.Context, udid, path string) error {
	_, err := p.cmd(ctx, "io", udid, "screenshot", path)
	return err
}

// Tap implements device.Controller. simctl has no input injection, so this is
// an honest refusal rather than a half-working shim.
func (p *Provider) Tap(context.Context, string, int, int) error {
	return fmt.Errorf("%w: iOS simulators have no tap command in simctl; drive the UI with XCTest, idb, or maestro", device.ErrUsage)
}

// Install implements device.Controller: an .app bundle path.
func (p *Provider) Install(ctx context.Context, udid, path string) error {
	_, err := p.cmd(ctx, "install", udid, path)
	return err
}

// Launch implements device.Controller: a bundle identifier.
func (p *Provider) Launch(ctx context.Context, udid, bundleID string) error {
	_, err := p.cmd(ctx, "launch", udid, bundleID)
	return err
}

// Serial implements device.Controller: the UDID is the address every tool uses.
func (p *Provider) Serial(_ context.Context, udid string) (string, error) { return udid, nil }

// OpenURL implements device.Controller.
func (p *Provider) OpenURL(ctx context.Context, udid, url string) error {
	_, err := p.cmd(ctx, "openurl", udid, url)
	return err
}

// LogStream implements device.Controller: `log stream --style compact` on a
// booted simulator. Physical iOS devices have no streaming log through
// devicectl, so they are refused.
func (p *Provider) LogStream(ctx context.Context, udid string) (<-chan logs.Line, error) {
	s, err := p.find(ctx, udid)
	if err != nil {
		return nil, fmt.Errorf("%w: live logs need a simulator; physical iOS devices are not supported", device.ErrUsage)
	}
	if mapState(s.State) != device.Booted {
		return nil, fmt.Errorf("%w: %s", device.ErrNotBooted, s.Name)
	}
	// `log stream` block-buffers when stdout is a pipe and nothing arrives for
	// minutes; under a pseudo-terminal (script) it line-buffers. script emits
	// CRLF, hence the trim.
	raw, err := p.Run.Stream(ctx, "script", "-q", "/dev/null", "xcrun", "simctl", "spawn", udid, "log", "stream", "--style", "compact", "--color", "none")
	if err != nil {
		return nil, err
	}
	out := make(chan logs.Line, 256)
	go func() {
		defer close(out)
		for s := range raw {
			s = logs.Clean(s)
			if s == "" || strings.HasPrefix(s, "Filtering the log data") || strings.HasPrefix(s, "Timestamp ") || strings.HasPrefix(s, "getpwuid_r") {
				continue // log stream's own header lines and script's noise
			}
			out <- logs.ParseIOS(s)
		}
	}()
	return out, nil
}

// Logs implements device.Controller: the last lines of the unified log.
func (p *Provider) Logs(ctx context.Context, udid string, lines int) (string, error) {
	if s, err := p.find(ctx, udid); err == nil && mapState(s.State) != device.Booted {
		return "", fmt.Errorf("%w: %s", device.ErrNotBooted, s.Name)
	}
	out, err := p.cmd(ctx, "spawn", udid, "log", "show", "--last", "30m", "--style", "compact")
	if err != nil {
		return "", err
	}
	return lastLines(out, lines), nil
}

// lastLines keeps the final n lines of s (all when n <= 0).
func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if n > 0 && len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n")
}
