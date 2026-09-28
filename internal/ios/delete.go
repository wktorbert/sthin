package ios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wktorbert/sthin/internal/device"
)

// Delete implements device.Deleter: `simctl delete` removes the simulator and
// its data directory for good. CoreSimulator refuses while the device is
// booted, so anything but a shut-down simulator is turned away before a
// command runs. The host-side launchd override store and Sthin's saved
// records for the UDID go with it, otherwise they would linger for a device
// that no longer exists.
func (p *Provider) Delete(ctx context.Context, udid string) error {
	s, err := p.find(ctx, udid)
	if err != nil {
		return err
	}
	if st := mapState(s.State); st != device.Shutdown {
		return fmt.Errorf("%w: %s is %s; shut it down before deleting", device.ErrUsage, s.Name, st)
	}
	if _, err := p.cmd(ctx, "delete", udid); err != nil {
		return err
	}
	if p.OverrideRoot != "" {
		_ = os.RemoveAll(filepath.Join(p.OverrideRoot, "com.apple.CoreSimulator.SimDevice."+udid))
	}
	return p.State.Forget(udid)
}
