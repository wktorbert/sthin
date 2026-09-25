package ios

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wktorbert/lean-sim/internal/device"
)

// RuntimeVersions returns the versions of every available iOS simulator runtime.
func RuntimeVersions(ctx context.Context, run device.Runner) ([]string, error) {
	out, stderr, err := run.Run(ctx, "xcrun", "simctl", "list", "-j", "runtimes")
	if err != nil {
		return nil, fmt.Errorf("xcrun simctl list -j runtimes: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	var rts struct {
		Runtimes []struct {
			Identifier  string `json:"identifier"`
			Version     string `json:"version"`
			IsAvailable bool   `json:"isAvailable"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal(out, &rts); err != nil {
		return nil, fmt.Errorf("parse simctl runtimes: %w", err)
	}
	var v []string
	for _, r := range rts.Runtimes {
		if r.IsAvailable && strings.Contains(r.Identifier, "SimRuntime.iOS-") {
			v = append(v, r.Version)
		}
	}
	return v, nil
}

// PersistsOverrides reports whether a runtime keeps Override store entries across boots.
func PersistsOverrides(version string) bool { return persistsOverrides(version) }
