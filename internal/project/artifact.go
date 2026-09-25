package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/host"
)

// ErrNoArtifact means no built .app or .apk was found for the project.
var ErrNoArtifact = errors.New("no built app found; build first or pass --app")

// artifactGlobs lists where each project kind leaves a debug build, relative
// to the project directory ("~" expands to the home directory).
func artifactGlobs(p Project, ios bool) []string {
	switch p.Kind {
	case Flutter:
		if ios {
			// Default and flavored builds: iphonesimulator/ and Debug-<flavor>-iphonesimulator/.
			return []string{"build/ios/iphonesimulator/*.app", "build/ios/*-iphonesimulator/*.app"}
		}
		return []string{"build/app/outputs/flutter-apk/app-debug.apk", "build/app/outputs/apk/debug/*.apk"}
	case ReactNative:
		if ios {
			return []string{"ios/build/Build/Products/Debug-iphonesimulator/*.app"}
		}
		return []string{"android/app/build/outputs/apk/debug/*.apk"}
	case IOSNative:
		return []string{"build/Debug-iphonesimulator/*.app", "build/Build/Products/Debug-iphonesimulator/*.app",
			"~/Library/Developer/Xcode/DerivedData/" + p.Name + "-*/Build/Products/Debug-iphonesimulator/*.app"}
	case AndroidNative:
		return []string{"app/build/outputs/apk/debug/*.apk", "*/build/outputs/apk/debug/*.apk"}
	}
	return nil
}

// FindArtifact returns the newest built app for the platform.
func FindArtifact(p Project, ios bool, home string) (string, error) {
	var hits []string
	for _, g := range artifactGlobs(p, ios) {
		if strings.HasPrefix(g, "~/") {
			g = filepath.Join(home, g[2:])
		} else {
			g = filepath.Join(p.Dir, g)
		}
		m, _ := filepath.Glob(g)
		hits = append(hits, m...)
	}
	if len(hits) == 0 {
		return "", ErrNoArtifact
	}
	sort.Slice(hits, func(i, j int) bool { return mtime(hits[i]).After(mtime(hits[j])) })
	return hits[0], nil
}

func mtime(p string) (t time.Time) {
	if st, err := os.Stat(p); err == nil {
		t = st.ModTime()
	}
	return t
}

var (
	bundleIDRe = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9.-]+)\s*$`)
	packageRe  = regexp.MustCompile(`package: name='([^']+)'`)
)

// AppID extracts the launch identifier from a built artifact: CFBundleIdentifier
// from an .app's Info.plist (plutil), the package name from an .apk (aapt from
// the SDK's build-tools).
func AppID(ctx context.Context, run device.Runner, env host.Env, artifact string) (string, error) {
	switch strings.ToLower(filepath.Ext(artifact)) {
	case ".app":
		out, stderr, err := run.Run(ctx, "plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-", filepath.Join(artifact, "Info.plist"))
		if err != nil {
			return "", fmt.Errorf("read bundle identifier: %w: %s", err, strings.TrimSpace(string(stderr)))
		}
		if m := bundleIDRe.FindStringSubmatch(string(out)); m != nil {
			return m[1], nil
		}
		return "", fmt.Errorf("read bundle identifier: unexpected plutil output %q", strings.TrimSpace(string(out)))
	case ".apk":
		aapt, ok := findAapt(env)
		if !ok {
			return "", errors.New("aapt not found under the Android SDK build-tools; pass --app-id <package>")
		}
		out, stderr, err := run.Run(ctx, aapt, "dump", "badging", artifact)
		if err != nil {
			return "", fmt.Errorf("aapt dump badging: %w: %s", err, strings.TrimSpace(string(stderr)))
		}
		if m := packageRe.FindStringSubmatch(string(out)); m != nil {
			return m[1], nil
		}
		return "", errors.New("aapt did not report a package name; pass --app-id <package>")
	}
	return "", fmt.Errorf("%w: %s is neither .app nor .apk", device.ErrUsage, artifact)
}

// findAapt picks the newest build-tools directory that has aapt.
func findAapt(env host.Env) (string, bool) {
	if p, err := env.LookPath("aapt"); err == nil {
		return p, true
	}
	for _, root := range env.SDKRoots() {
		dirs, _ := filepath.Glob(filepath.Join(root, "build-tools", "*", "aapt"))
		sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
		for _, d := range dirs {
			if env.Exists(d) {
				return d, true
			}
		}
	}
	return "", false
}
