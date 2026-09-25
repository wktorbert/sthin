package android

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultRAMMB = 1024
	backupSuffix = ".lean.bak"
)

// is16k reports whether the AVD uses a 16 KB page-size image, which Lean refuses to tune.
func is16k(cfg map[string]string) (bool, string) {
	for _, k := range []string{"image.sysdir.1", "tag.id"} {
		v := strings.ToLower(cfg[k])
		for _, marker := range []string{"ps16k", "page_size_16kb", "16kb"} {
			if strings.Contains(v, marker) {
				return true, fmt.Sprintf("%s=%s is a 16 KB page-size image; Lean does not tune these", k, cfg[k])
			}
		}
	}
	return false, ""
}

// tuneKeys are the config.ini values a slim boot sets.
func tuneKeys(ramMB int) [][2]string {
	return [][2]string{
		{"hw.ramSize", strconv.Itoa(ramMB)},
		{"hw.gpu.enabled", "yes"},
		{"hw.gpu.mode", "host"},
		{"hw.audioInput", "no"},
		{"hw.audioOutput", "no"},
		{"hw.camera.back", "none"},
		{"hw.camera.front", "none"},
	}
}

// tune backs config.ini up (only if no backup exists), sets the low-RAM keys,
// and deletes hardware-qemu.ini so the emulator regenerates it. It returns the backup path.
func tune(dir string, ramMB int) (string, error) {
	cfgPath := filepath.Join(dir, "config.ini")
	orig, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", err
	}
	cfg, err := readINI(cfgPath)
	if err != nil {
		return "", err
	}
	if bad, why := is16k(cfg); bad {
		return "", errors.New(why)
	}
	backup := cfgPath + backupSuffix
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(backup, orig, 0o644); err != nil {
			return "", err
		}
	}
	if err := writeFileAtomic(cfgPath, setINI(orig, tuneKeys(ramMB))); err != nil {
		return "", err
	}
	removeHardwareINI(dir)
	return backup, nil
}

// untune restores config.ini from the backup and deletes the backup.
// It reports whether a backup existed.
func untune(dir string) (bool, error) {
	cfgPath := filepath.Join(dir, "config.ini")
	backup := cfgPath + backupSuffix
	b, err := os.ReadFile(backup)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := writeFileAtomic(cfgPath, b); err != nil {
		return false, err
	}
	removeHardwareINI(dir)
	return true, os.Remove(backup)
}

func removeHardwareINI(dir string) {
	os.Remove(filepath.Join(dir, "hardware-qemu.ini"))
	os.Remove(filepath.Join(dir, "hardware-qemu.ini.lock"))
}

// setINI rewrites key=value lines in place, keeping every other line and its
// order, and appends keys that were absent.
func setINI(orig []byte, kv [][2]string) []byte {
	want := map[string]string{}
	for _, p := range kv {
		want[p[0]] = p[1]
	}
	done := map[string]bool{}
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(orig))
	for sc.Scan() {
		line := sc.Text()
		if k, _, ok := strings.Cut(line, "="); ok {
			k = strings.TrimSpace(k)
			if v, hit := want[k]; hit {
				if done[k] {
					continue // drop duplicate keys
				}
				line = k + "=" + v
				done[k] = true
			}
		}
		out.WriteString(line + "\n")
	}
	for _, p := range kv {
		if !done[p[0]] {
			out.WriteString(p[0] + "=" + p[1] + "\n")
		}
	}
	return out.Bytes()
}

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".lean.tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
