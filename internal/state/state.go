// Package state persists Change records: exactly what Lean disabled and every
// setting it changed on a Device, so Restore can replay it and nothing else.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Setting is one changed key with its prior value (nil when it was unset).
type Setting struct {
	Key     string  `json:"key"`
	Scope   string  `json:"scope"`
	Prior   *string `json:"prior"`
	New     string  `json:"new"`
	Skipped bool    `json:"skipped,omitempty"` // the runtime rejected the write
}

// Record is a Device's Change record.
type Record struct {
	ID             string    `json:"id"`
	Platform       string    `json:"platform"`
	AppliedAt      time.Time `json:"applied_at"`
	ProfileVersion int       `json:"profile_version"`
	Disabled       []string  `json:"disabled"`
	Settings       []Setting `json:"settings"`
	ConfigBackup   string    `json:"config_backup,omitempty"`
}

// Store reads and writes records under <Home>/state.
type Store struct {
	Home string
}

// Default returns the Store at $LEAN_HOME (or ~/.lean).
func Default() Store {
	if h := os.Getenv("LEAN_HOME"); h != "" {
		return Store{Home: h}
	}
	home, _ := os.UserHomeDir()
	return Store{Home: filepath.Join(home, ".lean")}
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func (s Store) path(id string) string {
	return filepath.Join(s.Home, "state", unsafe.ReplaceAllString(id, "_")+".json")
}

// Save writes the record atomically.
func (s Store) Save(r Record) error {
	p := s.path(r.ID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Load returns the record for id; ok is false when none exists.
func (s Store) Load(id string) (r Record, ok bool, err error) {
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, false, err
	}
	return r, true, nil
}

// Delete removes the record; a missing record is not an error.
func (s Store) Delete(id string) error {
	err := os.Remove(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
