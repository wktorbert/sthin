package ios

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"howett.net/plist"
)

// overrideStore is a simulator's host-side launchd Override store: a flat plist
// dict of label → disabled (true) / enabled (false). launchd_sim reads it at
// boot; writing it while the simulator is shut down makes the next boot slim.
type overrideStore struct {
	path string
}

func (p *Provider) store(udid string) overrideStore {
	return overrideStore{path: filepath.Join(p.OverrideRoot, "com.apple.CoreSimulator.SimDevice."+udid, "disabled.plist")}
}

// Read returns label → disabled. A missing file means nothing is overridden.
func (s overrideStore) Read() (map[string]bool, error) {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if _, err := plist.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	out := make(map[string]bool, len(raw))
	for k, v := range raw {
		bv, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s: entry %q is %T, not a bool; refusing to rewrite", s.path, k, v)
		}
		out[k] = bv
	}
	return out, nil
}

// Write merges transitions into the existing entries (every other entry kept
// as is), sorts keys, and replaces the file atomically with mode 0644.
func (s overrideStore) Write(toDisable, toEnable []string) error {
	cur, err := s.Read()
	if err != nil {
		return err
	}
	for _, l := range toDisable {
		cur[l] = true
	}
	for _, l := range toEnable {
		cur[l] = false
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".disabled.plist.lean-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(encodeBoolDict(cur)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// encodeBoolDict renders the XML plist format launchd itself writes.
func encodeBoolDict(m map[string]bool) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	for _, k := range keys {
		b.WriteString("\t<key>")
		xml.EscapeText(&b, []byte(k))
		b.WriteString("</key>\n")
		if m[k] {
			b.WriteString("\t<true/>\n")
		} else {
			b.WriteString("\t<false/>\n")
		}
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}
