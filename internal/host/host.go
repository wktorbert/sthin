// Package host discovers the platform toolchains (xcrun, emulator, adb) and
// Lean's own directories. Everything that touches the environment is behind
// Env so tests can fake it.
package host

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Env is the view of the host Lean needs.
type Env struct {
	GOOS     string
	Getenv   func(string) string
	HomeDir  string
	LookPath func(string) (string, error)
	Exists   func(string) bool
	IsDir    func(string) bool // nil in fixtures that do not care about directories
}

// Real returns the Env of this process.
func Real() Env {
	home, _ := os.UserHomeDir()
	return Env{
		GOOS:     runtime.GOOS,
		Getenv:   os.Getenv,
		HomeDir:  home,
		LookPath: osexec.LookPath,
		Exists: func(p string) bool {
			st, err := os.Stat(p)
			return err == nil && !st.IsDir()
		},
		IsDir: func(p string) bool {
			st, err := os.Stat(p)
			return err == nil && st.IsDir()
		},
	}
}

// SDKRoots returns candidate Android SDK roots in priority order.
func (e Env) SDKRoots() []string {
	var roots []string
	for _, k := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := e.Getenv(k); v != "" {
			roots = append(roots, v)
		}
	}
	switch e.GOOS {
	case "darwin":
		if e.HomeDir != "" {
			roots = append(roots, filepath.Join(e.HomeDir, "Library", "Android", "sdk"))
		}
	case "windows":
		// Android Studio's default on Windows.
		if v := e.Getenv("LOCALAPPDATA"); v != "" {
			roots = append(roots, filepath.Join(v, "Android", "Sdk"))
		}
	default:
		if e.HomeDir != "" {
			roots = append(roots, filepath.Join(e.HomeDir, "Android", "Sdk"))
		}
	}
	return roots
}

// exeName appends .exe on Windows for tools probed by path (LookPath already
// applies PATHEXT for names found on PATH).
func (e Env) exeName(name string) string {
	if e.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func (e Env) findSDKTool(name, subdir string) (string, bool) {
	if p, err := e.LookPath(name); err == nil {
		return p, true
	}
	for _, r := range e.SDKRoots() {
		p := filepath.Join(r, subdir, e.exeName(name))
		if e.Exists(p) {
			return p, true
		}
	}
	return "", false
}

// Adb locates adb on PATH or under an SDK root's platform-tools.
func (e Env) Adb() (string, bool) { return e.findSDKTool("adb", "platform-tools") }

// Emulator locates the emulator binary on PATH or under an SDK root.
func (e Env) Emulator() (string, bool) { return e.findSDKTool("emulator", "emulator") }

// Xcrun locates xcrun; only meaningful on macOS.
func (e Env) Xcrun() (string, bool) {
	if e.GOOS != "darwin" {
		return "", false
	}
	p, err := e.LookPath("xcrun")
	return p, err == nil
}

// IOSSupported reports whether iOS simulators can exist on this OS, with a reason when not.
// MissingDir reports a path that should be a directory but is not. It is false
// when the path is empty or the Env has no IsDir probe.
func (e Env) MissingDir(p string) bool {
	return p != "" && e.IsDir != nil && !e.IsDir(p)
}

// DeveloperDir is Xcode's developer directory: $DEVELOPER_DIR or the App Store
// install's default. It is not verified to exist.
func (e Env) DeveloperDir() string {
	if d := e.Getenv("DEVELOPER_DIR"); d != "" {
		return d
	}
	return "/Applications/Xcode.app/Contents/Developer"
}

// SimulatorApps returns the installed apps that show a simulator window, in
// preference order. Xcode up to 26 ships Simulator.app under the developer
// directory; Xcode 27 replaced it with DeviceHub.app next to Instruments.
func (e Env) SimulatorApps() []string {
	dev := e.DeveloperDir()
	xcode := strings.TrimSuffix(dev, "/Contents/Developer")
	var out []string
	for _, p := range []string{
		filepath.Join(dev, "Applications", "Simulator.app"),
		filepath.Join(xcode, "Contents", "Applications", "DeviceHub.app"),
	} {
		// Exists only accepts regular files, so probe the bundle's Info.plist.
		if e.Exists(filepath.Join(p, "Contents", "Info.plist")) {
			out = append(out, p)
		}
	}
	return out
}

func (e Env) IOSSupported() (bool, string) {
	if e.GOOS != "darwin" {
		return false, "iOS simulators require macOS"
	}
	if _, ok := e.Xcrun(); !ok {
		return false, "xcrun not found; install Xcode"
	}
	return true, ""
}

// AVDHome is where AVD .ini files live.
func (e Env) AVDHome() string {
	if v := e.Getenv("ANDROID_AVD_HOME"); v != "" {
		return v
	}
	return filepath.Join(e.HomeDir, ".android", "avd")
}

// LeanHome is $LEAN_HOME or ~/.lean.
func (e Env) LeanHome() string {
	if v := e.Getenv("LEAN_HOME"); v != "" {
		return v
	}
	return filepath.Join(e.HomeDir, ".lean")
}
