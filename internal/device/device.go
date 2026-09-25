// Package device defines the platform-neutral model Lean works with: Devices,
// the Provider seam each platform implements, and the Runner seam through which
// Providers execute host commands.
package device

import (
	"context"
	"errors"
	"github.com/wktorbert/lean-sim/internal/logs"
)

// Platform identifies the kind of virtual device.
type Platform string

const (
	IOS     Platform = "ios"
	Android Platform = "android"
)

// State is the power state of a Device.
type State string

const (
	Shutdown State = "shutdown"
	Booting  State = "booting"
	Booted   State = "booted"
)

// SlimState says how much of the Profile is currently applied to a Device.
type SlimState string

const (
	Stock         SlimState = "stock"
	Slim          SlimState = "slim"
	Partial       SlimState = "partial"
	Unknown       SlimState = "unknown"
	NotApplicable SlimState = "n/a" // physical devices are never slimmed
)

// Device is one iOS simulator or Android virtual device as Lean lists it.
// Kind says what a Device physically is. Physical devices are listed and
// used, never slimmed or otherwise modified.
type Kind string

const (
	Simulator Kind = "simulator"
	Emulator  Kind = "emulator"
	Physical  Kind = "physical"
)

// WirelessADB is the optional seam for Android wireless debugging; the Android
// Provider implements it, callers type-assert for it.
type WirelessADB interface {
	Pair(ctx context.Context, addr, code string) (string, error)
	Connect(ctx context.Context, addr string) (string, error)
	Disconnect(ctx context.Context, addr string) (string, error)
}

type Device struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Platform    Platform  `json:"platform"`
	Kind        Kind      `json:"kind"`
	Model       string    `json:"model,omitempty"` // physical devices: marketing name and transport
	OSVersion   string    `json:"os_version"`
	State       State     `json:"state"`
	Slim        SlimState `json:"slim"`
	FootprintMB *int      `json:"footprint_mb"`
	Warnings    []string  `json:"warnings"`
}

// BootOptions narrows or disables slimming for one boot.
type BootOptions struct {
	Stock  bool
	Except []string // Category IDs to leave enabled; nil means the device's saved preference
	RAMMB  int      // Android only; 0 means the default
	// Headless skips the window: no Simulator.app on iOS, -no-window on Android.
	Headless bool
}

// Stage status values reported through a Reporter.
const (
	StageRunning = "running"
	StageOK      = "ok"
	StageFail    = "fail"
	StageWarn    = "warn"
	StageSkip    = "skip"
)

// Stage is one named step of a boot or restore.
type Stage struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Command string `json:"command,omitempty"` // the failed command, when Status is fail
	// Measurement is set on the final measure stage.
	Measurement *Measurement `json:"measurement,omitempty"`
}

// Reporter receives stage progress. It may be nil.
type Reporter func(Stage)

// Report calls r when it is non-nil.
func (r Reporter) Report(s Stage) {
	if r != nil {
		r(s)
	}
}

// Measurement is what a Device costs the host.
type Measurement struct {
	ID           string   `json:"id"`
	Platform     Platform `json:"platform"`
	FootprintMB  int      `json:"footprint_mb"`
	ProcessCount int      `json:"process_count"`
	DirtyMB      *int     `json:"dirty_mb,omitempty"`
	GuestTotalMB *int     `json:"guest_total_mb,omitempty"`
	GuestUsedMB  *int     `json:"guest_used_mb,omitempty"`
}

// Provider is the platform module behind a set of Devices.
type Provider interface {
	Platform() Platform
	Available() (bool, string)
	List(ctx context.Context) ([]Device, error)
	Boot(ctx context.Context, id string, opts BootOptions, r Reporter) error
	Restore(ctx context.Context, id string, r Reporter) error
	Shutdown(ctx context.Context, id string) error
	Measure(ctx context.Context, id string) (Measurement, error)
}

// Controller is the optional seam for using a device once it runs: what an
// agent needs after boot. Both Providers implement it; callers type-assert.
type Controller interface {
	Screenshot(ctx context.Context, id, path string) error // PNG written to path
	Tap(ctx context.Context, id string, x, y int) error
	Install(ctx context.Context, id, path string) error // .app / .apk
	Launch(ctx context.Context, id, app string) error   // bundle ID / package
	Logs(ctx context.Context, id string, lines int) (string, error)
	// Serial is the address other tools use for this device: the UDID on iOS,
	// the adb serial on Android (a booted emulator's, or a phone's own).
	Serial(ctx context.Context, id string) (string, error)
	// OpenURL opens a deep link or web URL on the device.
	OpenURL(ctx context.Context, id, url string) error
	// LogStream follows the device log live until ctx is cancelled, one
	// normalised line at a time (see internal/logs).
	LogStream(ctx context.Context, id string) (<-chan logs.Line, error)
}

// ErrNotBooted is returned by Measure (and friends) for a Device that is not running.
var ErrNotBooted = errors.New("not booted")

// StageError is returned by Boot/Restore when a stage fails; it names the command.
type StageError struct {
	Stage   string
	Command string
	Err     error
}

func (e *StageError) Error() string {
	msg := "stage " + e.Stage + " failed"
	if e.Command != "" {
		msg += ": " + e.Command
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *StageError) Unwrap() error { return e.Err }

// Runner executes host commands. It is the only way Lean spawns a process.
type Runner interface {
	// Run executes name with args and returns stdout and stderr.
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
	// Start launches a detached process in its own session with output appended to logPath.
	Start(ctx context.Context, logPath, name string, args ...string) error
	// Passthrough runs name in dir with the caller's terminal attached (stdin,
	// stdout, stderr inherited) and waits for it: framework hand-offs such as
	// `flutter run` that own the terminal until the user quits them.
	Passthrough(ctx context.Context, dir, name string, args ...string) error
	// Stream starts name and delivers its stdout line by line until it exits or
	// ctx is cancelled; the channel is closed afterwards. Long-running readers
	// such as log streams use it.
	Stream(ctx context.Context, name string, args ...string) (<-chan string, error)
}
