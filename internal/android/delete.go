package android

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wktorbert/sthin/internal/device"
)

// Delete implements device.Deleter. An AVD is its <name>.avd folder plus the
// <name>.ini pointer beside it in the AVD home, which is exactly what
// `avdmanager delete avd` removes; doing it directly needs neither the
// command-line tools nor Java. A running emulator is refused so its files are
// never pulled out from under it. Sthin's saved preferences and change record
// for the AVD are dropped as well; the slim backup of config.ini lives inside
// the folder and goes with it.
func (p *Provider) Delete(ctx context.Context, id string) error {
	a, err := p.findAVD(id)
	if err != nil {
		return err
	}
	if _, running := p.running(ctx)[id]; running {
		return fmt.Errorf("%w: %s is running; shut it down before deleting", device.ErrUsage, a.displayName())
	}
	if err := os.RemoveAll(a.Dir); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(p.AVDHome, a.Name+".ini")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return p.State.Forget(id)
}
