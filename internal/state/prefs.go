package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wktorbert/sthin/internal/device"
)

// Prefs is what the user chose for one Device: the Categories to keep enabled
// on a slim boot. Unlike the Change record it survives restore, because it is
// a preference, not a record of what changed.
type Prefs struct {
	ID     string               `json:"id"`
	Except []string             `json:"except"`           // Category IDs kept enabled
	Launch device.LaunchOptions `json:"launch,omitempty"` // Android emulator switches
}

func (s Store) prefsPath(id string) string {
	return filepath.Join(s.Home, "prefs", unsafe.ReplaceAllString(id, "_")+".json")
}

// SavePrefs writes the preference atomically.
func (s Store) SavePrefs(p Prefs) error {
	if p.Except == nil {
		p.Except = []string{}
	}
	path := s.prefsPath(p.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadPrefs returns the preference for id; ok is false when none was saved.
func (s Store) LoadPrefs(id string) (p Prefs, ok bool, err error) {
	b, err := os.ReadFile(s.prefsPath(id))
	if errors.Is(err, fs.ErrNotExist) {
		return Prefs{ID: id, Except: []string{}}, false, nil
	}
	if err != nil {
		return Prefs{}, false, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return Prefs{}, false, err
	}
	if p.Except == nil {
		p.Except = []string{}
	}
	return p, true, nil
}

// MergeLaunch saves the given launch switches on top of the saved ones,
// leaving the Category preference and unmentioned switches as they were.
func (s Store) MergeLaunch(id string, l device.LaunchOptions) error {
	p, _, err := s.LoadPrefs(id)
	if err != nil {
		return err
	}
	p.ID = id
	p.Launch = p.Launch.Merge(l)
	return s.SavePrefs(p)
}

// LaunchFor resolves the launch switches for one boot: the saved preference
// with the explicit fields applied on top.
func (s Store) LaunchFor(id string, given device.LaunchOptions) (device.LaunchOptions, error) {
	p, _, err := s.LoadPrefs(id)
	if err != nil {
		return device.LaunchOptions{}, err
	}
	return p.Launch.Merge(given), nil
}

// ExceptFor resolves the Categories to keep for one boot: an explicit list
// (non-nil, possibly empty) wins; nil means "use the saved preference".
func (s Store) ExceptFor(id string, given []string) ([]string, error) {
	if given != nil {
		return given, nil
	}
	p, _, err := s.LoadPrefs(id)
	if err != nil {
		return nil, err
	}
	return p.Except, nil
}
