// Package complete builds shell-completion candidates for Device references,
// Profile categories and the few flags with a fixed value set, and keeps the
// last device listing in a small cache so a TAB does not pay for simctl and
// adb every time.
package complete

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/wktorbert/sthin/internal/device"
)

// Cache is the last device listing on disk, read only by completion. A
// listing older than TTL is ignored; Drop removes it after a command changed
// a device so the next TAB relists instead of waiting the window out.
type Cache struct {
	Path string
	TTL  time.Duration
	Now  func() time.Time // tests override; nil means time.Now
}

type cached struct {
	At      time.Time       `json:"at"`
	Devices []device.Device `json:"devices"`
}

func (c Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Write stores the devices of every platform that listed successfully. A
// cache with no Path is disabled.
func (c Cache) Write(lists []device.PlatformList) {
	if c.Path == "" {
		return
	}
	entry := cached{At: c.now(), Devices: []device.Device{}}
	for _, pl := range lists {
		if pl.Available && pl.Err == nil {
			entry.Devices = append(entry.Devices, pl.Devices...)
		}
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return
	}
	tmp := c.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.Path)
}

// Read returns the cached devices when the cache exists and is fresher than TTL.
func (c Cache) Read() ([]device.Device, bool) {
	if c.Path == "" {
		return nil, false
	}
	b, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, false
	}
	var entry cached
	if json.Unmarshal(b, &entry) != nil || c.now().Sub(entry.At) > c.TTL {
		return nil, false
	}
	return entry.Devices, true
}

// Drop forgets the listing; a missing cache is not an error.
func (c Cache) Drop() error {
	if c.Path == "" {
		return nil
	}
	err := os.Remove(c.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
