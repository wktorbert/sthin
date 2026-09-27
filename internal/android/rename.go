package android

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wktorbert/lean-sim/internal/device"
)

// Rename implements device.Renamer: the display name lives in config.ini as
// avd.ini.displayname (what Android Studio's Edit sets). The AVD id, its
// folder name, never changes, so preferences and change records keep working.
// The slim backup of config.ini is updated too, otherwise restore would
// revert the name.
func (p *Provider) Rename(_ context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", device.ErrUsage)
	}
	a, err := p.findAVD(id)
	if err != nil {
		return err
	}
	kv := [][2]string{{"avd.ini.displayname", name}}
	cfg := filepath.Join(a.Dir, "config.ini")
	for _, path := range []string{cfg, backupPath(cfg)} {
		orig, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := writeFileAtomic(path, setINI(orig, kv)); err != nil {
			return err
		}
	}
	return nil
}
