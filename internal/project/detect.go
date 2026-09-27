// Package project runs the app in a mobile project on a Sthin device: detect
// the project kind, hand off to the framework's own run command (Flutter,
// React Native) or install and launch a built artifact (native), then open a
// deep link. It is the one module that knows what a mobile project looks like.
package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Kind is the project type.
type Kind string

const (
	Flutter       Kind = "flutter"
	ReactNative   Kind = "react-native"
	IOSNative     Kind = "ios"
	AndroidNative Kind = "android"
	Unknown       Kind = "unknown"
)

// Project is a detected mobile project.
type Project struct {
	Dir  string `json:"dir"`
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
}

// Detect inspects dir. Order matters: a Flutter or React Native project also
// contains native ios/ and android/ folders.
func Detect(dir string) Project {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	p := Project{Dir: abs, Name: filepath.Base(abs), Kind: Unknown}
	if b, err := os.ReadFile(filepath.Join(abs, "pubspec.yaml")); err == nil && strings.Contains(string(b), "flutter") {
		p.Kind = Flutter
		return p
	}
	if b, err := os.ReadFile(filepath.Join(abs, "package.json")); err == nil && strings.Contains(string(b), `"react-native"`) {
		p.Kind = ReactNative
		return p
	}
	if hasGlob(abs, "*.xcworkspace") || hasGlob(abs, "*.xcodeproj") {
		p.Kind = IOSNative
		return p
	}
	for _, f := range []string{"settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(abs, f)); err == nil {
			p.Kind = AndroidNative
			return p
		}
	}
	return p
}

func hasGlob(dir, pattern string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, pattern))
	return len(m) > 0
}

// HandoffCmd is Handoff as an *exec.Cmd rooted in the project directory, for
// callers that take over the terminal themselves (the TUI via tea.ExecProcess).
func (p Project) HandoffCmd(ios bool, serial string) *exec.Cmd {
	argv := p.Handoff(ios, serial)
	if argv == nil {
		return nil
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = p.Dir
	return c
}

// SupportsIOS and SupportsAndroid say which platforms the project can target.
func (p Project) SupportsIOS() bool {
	return p.Kind == Flutter || p.Kind == ReactNative || p.Kind == IOSNative
}
func (p Project) SupportsAndroid() bool {
	return p.Kind == Flutter || p.Kind == ReactNative || p.Kind == AndroidNative
}

// Handoff returns the framework command that builds, installs and launches on
// the device with the given serial, run from the project directory. Empty for
// native projects, which have no single run command.
func (p Project) Handoff(ios bool, serial string) []string {
	switch p.Kind {
	case Flutter:
		return []string{"flutter", "run", "-d", serial}
	case ReactNative:
		if ios {
			return []string{"npx", "react-native", "run-ios", "--udid", serial}
		}
		return []string{"npx", "react-native", "run-android", "--deviceId", serial}
	}
	return nil
}
