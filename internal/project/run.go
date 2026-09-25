package project

import (
	"context"
	"fmt"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/host"
)

// Deps is what Run needs from the host process.
type Deps struct {
	Reg *device.Registry
	Run device.Runner
	Env host.Env
}

// Opts selects what to run and where.
type Opts struct {
	Dir       string // project directory
	Device    string // device ID or exact name
	App       string // built artifact to install instead of detecting one
	AppID     string // bundle identifier / package to launch, when it cannot be read from the artifact
	URL       string // deep link to open after launch
	NoHandoff bool   // for framework projects: install a built artifact instead of running the framework command
}

// Stage names reported through the Reporter.
const (
	StageDetect  = "detect"
	StageDevice  = "device"
	StageBoot    = "boot"
	StageHandoff = "handoff"
	StageInstall = "install"
	StageLaunch  = "launch"
	StageOpenURL = "open-url"
)

// Result is what a non-interactive run reports.
type Result struct {
	Project  Project       `json:"project"`
	Device   device.Device `json:"device"`
	Artifact string        `json:"artifact,omitempty"`
	AppID    string        `json:"app_id,omitempty"`
	Handoff  []string      `json:"handoff,omitempty"`
}

// Run detects the project, makes sure the device is up, then either hands
// off to the framework or installs and launches the built artifact. A
// framework hand-off owns the terminal until the user quits it.
func Run(ctx context.Context, d Deps, o Opts, r device.Reporter) (Result, error) {
	sr := device.Stages{R: r}
	var res Result
	res.Project = Detect(o.Dir)
	if res.Project.Kind == Unknown && o.App == "" {
		return res, fmt.Errorf("%w: no Flutter, React Native, Xcode or Gradle project in %s", device.ErrUsage, res.Project.Dir)
	}
	sr.R.Report(device.Stage{Name: StageDetect, Status: device.StageOK, Detail: string(res.Project.Kind) + " project " + res.Project.Name})

	p, dev, err := d.Reg.Resolve(ctx, o.Device)
	if err != nil {
		return res, err
	}
	res.Device = dev
	ios := dev.Platform == device.IOS
	if o.App == "" && ((ios && !res.Project.SupportsIOS()) || (!ios && !res.Project.SupportsAndroid())) {
		return res, fmt.Errorf("%w: %s project cannot run on %s", device.ErrUsage, res.Project.Kind, dev.Platform)
	}
	ctl, ok := p.(device.Controller)
	if !ok {
		return res, fmt.Errorf("%s provider cannot drive devices", dev.Platform)
	}
	sr.R.Report(device.Stage{Name: StageDevice, Status: device.StageOK, Detail: dev.Name + " (" + string(dev.Kind) + ")"})

	// Bring a virtual device up, with a window: the user is about to look at the app.
	if dev.Kind != device.Physical && dev.State != device.Booted {
		if err := p.Boot(ctx, dev.ID, device.BootOptions{}, r); err != nil {
			return res, err
		}
		res.Device.State = device.Booted
	}

	serial, err := ctl.Serial(ctx, dev.ID)
	if err != nil {
		return res, err
	}
	if handoff := res.Project.Handoff(ios, serial); handoff != nil && !o.NoHandoff && o.App == "" {
		res.Handoff = handoff
		if o.URL != "" {
			sr.Warn(StageOpenURL, "--url is ignored with a framework hand-off; use --no-handoff with a built app")
		}
		sr.R.Report(device.Stage{Name: StageHandoff, Status: device.StageRunning, Detail: joinArgv(handoff)})
		if err := d.Run.Passthrough(ctx, res.Project.Dir, handoff[0], handoff[1:]...); err != nil {
			return res, &device.StageError{Stage: StageHandoff, Command: joinArgv(handoff), Err: err}
		}
		sr.R.Report(device.Stage{Name: StageHandoff, Status: device.StageOK, Detail: joinArgv(handoff) + " exited"})
		return res, nil
	}

	artifact := o.App
	if artifact == "" {
		if artifact, err = FindArtifact(res.Project, ios, d.Env.HomeDir); err != nil {
			return res, err
		}
	}
	res.Artifact = artifact
	if err := sr.Do(StageInstall, func() (string, error) { return artifact, ctl.Install(ctx, dev.ID, artifact) }); err != nil {
		return res, err
	}
	appID := o.AppID
	if appID == "" {
		if appID, err = AppID(ctx, d.Run, d.Env, artifact); err != nil {
			return res, err
		}
	}
	res.AppID = appID
	if err := sr.Do(StageLaunch, func() (string, error) { return appID, ctl.Launch(ctx, dev.ID, appID) }); err != nil {
		return res, err
	}
	if o.URL != "" {
		if err := sr.Do(StageOpenURL, func() (string, error) { return o.URL, ctl.OpenURL(ctx, dev.ID, o.URL) }); err != nil {
			return res, err
		}
	}
	return res, nil
}

func joinArgv(a []string) string {
	s := ""
	for i, x := range a {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}
