// Package mcpserver exposes Sthin to agents over the Model Context Protocol:
// the same Provider operations as the CLI and TUI, plus the lease pool. It
// never shells out itself; every tool goes through device.Provider or
// device.Controller.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/host"
	"github.com/wktorbert/sthin/internal/lease"
	"github.com/wktorbert/sthin/internal/ops"
	"github.com/wktorbert/sthin/internal/project"
)

// Deps is what the server needs from the host process.
type Deps struct {
	Reg           *device.Registry
	Pool          lease.Pool
	Version       string
	ScreenshotDir string        // default for screenshots without a path
	Run           device.Runner // for run: artifact inspection
	Env           host.Env
}

// New builds the MCP server with every Sthin tool registered.
func New(d Deps) *mcp.Server {
	if d.ScreenshotDir == "" {
		d.ScreenshotDir = os.TempDir()
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "sthin", Version: d.Version}, nil)
	t := tools{d}
	mcp.AddTool(s, &mcp.Tool{Name: "devices_list", Description: "List simulators, emulators and physical devices with state, slim state, memory footprint and any active lease."}, t.devicesList)
	mcp.AddTool(s, &mcp.Tool{Name: "boot", Description: "Slim boot a simulator or emulator (headless by default) and wait until it is ready. Returns the stages and the memory footprint."}, t.boot)
	mcp.AddTool(s, &mcp.Tool{Name: "shutdown", Description: "Shut a simulator or emulator down."}, t.shutdown)
	mcp.AddTool(s, &mcp.Tool{Name: "restore", Description: "Return a device to stock: undo exactly what Sthin changed."}, t.restore)
	mcp.AddTool(s, &mcp.Tool{Name: "measure", Description: "Memory footprint of a booted device in MB (phys_footprint, as Activity Monitor shows)."}, t.measure)
	mcp.AddTool(s, &mcp.Tool{Name: "screenshot", Description: "Take a PNG screenshot. Returns the file path; set inline=true to also get the image."}, t.screenshot)
	mcp.AddTool(s, &mcp.Tool{Name: "tap", Description: "Tap at screen coordinates (Android only; iOS simulators have no tap API)."}, t.tap)
	mcp.AddTool(s, &mcp.Tool{Name: "install", Description: "Install an app: .app bundle on iOS, .apk on Android."}, t.install)
	mcp.AddTool(s, &mcp.Tool{Name: "launch", Description: "Launch an installed app by bundle identifier (iOS) or package name (Android)."}, t.launch)
	mcp.AddTool(s, &mcp.Tool{Name: "logs", Description: "Recent device log lines: unified log on iOS, logcat on Android."}, t.logs)
	mcp.AddTool(s, &mcp.Tool{Name: "lease", Description: "Acquire an idle device for this agent: reuses a booted one or slim boots one headless. The lease expires after ttl_seconds so a crashed agent never strands a device."}, t.lease)
	mcp.AddTool(s, &mcp.Tool{Name: "release", Description: "Release a leased device, optionally shutting it down."}, t.release)
	mcp.AddTool(s, &mcp.Tool{Name: "leases", Description: "List active leases."}, t.leases)
	mcp.AddTool(s, &mcp.Tool{Name: "run", Description: "Install and launch a mobile project's built debug app on a device (Flutter, React Native, Xcode or Gradle project; or an explicit .app/.apk), then optionally open a deep link. Build the app first; the interactive framework hand-off is CLI-only (sthin run)."}, t.run)
	mcp.AddTool(s, &mcp.Tool{Name: "open_url", Description: "Open a deep link or web URL on a device."}, t.openURL)
	mcp.AddTool(s, &mcp.Tool{Name: "rename", Description: "Rename a simulator or AVD (physical devices are refused)."}, t.rename)
	mcp.AddTool(s, &mcp.Tool{Name: "delete", Description: "Delete a simulator or AVD for good: its data, Sthin's saved records and any lease on it. The device must be shut down; physical devices are refused."}, t.deleteDevice)
	return s
}

type tools struct{ Deps }

// ---- inputs ----

type idIn struct {
	ID string `json:"id" jsonschema:"device ID (simulator UDID, AVD name, or adb serial) or exact name"`
}
type platformIn struct {
	Platform string `json:"platform,omitempty" jsonschema:"ios or android; empty for both"`
}
type bootIn struct {
	ID       string   `json:"id" jsonschema:"device ID or exact name"`
	Stock    bool     `json:"stock,omitempty" jsonschema:"boot without slimming"`
	Except   []string `json:"except,omitempty" jsonschema:"category IDs to keep enabled; omit to use the device's saved preference"`
	Window   bool     `json:"window,omitempty" jsonschema:"open a window (Simulator/DeviceHub or emulator UI); default headless"`
	RAMMB    int      `json:"ram_mb,omitempty" jsonschema:"Android guest RAM in MB (default 1024)"`
	ColdBoot *bool    `json:"cold_boot,omitempty" jsonschema:"Android: ignore the quick-boot snapshot; omit to use the AVD's saved preference"`
	Audio    *bool    `json:"audio,omitempty" jsonschema:"Android: keep audio on; omit to use the saved preference"`
	LowRAM   *bool    `json:"lowram,omitempty" jsonschema:"Android: pass -lowram; omit to use the saved preference"`
	Name     string   `json:"name,omitempty" jsonschema:"rename the device before booting"`
}
type renameIn struct {
	ID   string `json:"id" jsonschema:"device ID or exact name"`
	Name string `json:"name" jsonschema:"the new display name"`
}
type screenshotIn struct {
	ID     string `json:"id" jsonschema:"device ID or exact name"`
	Path   string `json:"path,omitempty" jsonschema:"PNG path to write; default a temp file"`
	Inline bool   `json:"inline,omitempty" jsonschema:"also return the image content"`
}
type tapIn struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
}
type installIn struct {
	ID   string `json:"id"`
	Path string `json:"path" jsonschema:".app bundle or .apk path on this host"`
}
type launchIn struct {
	ID  string `json:"id"`
	App string `json:"app" jsonschema:"bundle identifier or package name"`
}
type logsIn struct {
	ID    string `json:"id"`
	Lines int    `json:"lines,omitempty" jsonschema:"how many trailing lines (default 200)"`
}
type leaseIn struct {
	Platform   string `json:"platform,omitempty" jsonschema:"ios or android; empty for any"`
	TTLSeconds int    `json:"ttl_seconds,omitempty" jsonschema:"lease lifetime; default 1800"`
	Owner      string `json:"owner,omitempty" jsonschema:"who holds it, e.g. the agent or session name"`
}
type releaseIn struct {
	ID       string `json:"id" jsonschema:"device ID"`
	Shutdown bool   `json:"shutdown,omitempty" jsonschema:"also shut the device down"`
}
type emptyIn struct{}
type runIn struct {
	ID    string `json:"id" jsonschema:"device ID or exact name"`
	Dir   string `json:"dir,omitempty" jsonschema:"project directory; default the server's working directory"`
	App   string `json:"app,omitempty" jsonschema:"built .app or .apk path to install instead of detecting one"`
	AppID string `json:"app_id,omitempty" jsonschema:"bundle identifier or package to launch when it cannot be read from the artifact"`
	URL   string `json:"url,omitempty" jsonschema:"deep link to open after launch"`
}
type urlIn struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// ---- helpers ----

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

func textResult(s string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
}

func (t tools) resolve(ctx context.Context, id string) (device.Provider, device.Device, error) {
	return t.Reg.Resolve(ctx, id)
}

// resolveVirtual, controller and runBoot are the shared rules in internal/ops.
func (t tools) resolveVirtual(ctx context.Context, id, op string) (device.Provider, device.Device, error) {
	return ops.ResolveVirtual(ctx, t.Reg, id, op)
}

func controller(p device.Provider) (device.Controller, error) { return ops.Controller(p) }

func runBoot(ctx context.Context, p device.Provider, d device.Device, opts device.BootOptions) ops.BootResult {
	return ops.RunBoot(ctx, p, d, opts, nil)
}

// ---- tools ----

func (t tools) devicesList(ctx context.Context, _ *mcp.CallToolRequest, in platformIn) (*mcp.CallToolResult, any, error) {
	return jsonResult(ops.List(ctx, t.Reg, t.Pool, in.Platform))
}

func (t tools) boot(ctx context.Context, _ *mcp.CallToolRequest, in bootIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolveVirtual(ctx, in.ID, "boot")
	if err != nil {
		return nil, nil, err
	}
	res := runBoot(ctx, p, d, device.BootOptions{Stock: in.Stock, Except: in.Except, RAMMB: in.RAMMB, Headless: !in.Window, Launch: device.LaunchOptions{ColdBoot: in.ColdBoot, Audio: in.Audio, LowRAM: in.LowRAM}, Name: in.Name})
	if res.Error != "" {
		return nil, nil, fmt.Errorf("boot %s: %s", d.Name, res.Error)
	}
	return jsonResult(res)
}

func (t tools) shutdown(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolveVirtual(ctx, in.ID, "shutdown")
	if err != nil {
		return nil, nil, err
	}
	if err := p.Shutdown(ctx, d.ID); err != nil {
		return nil, nil, err
	}
	return textResult(d.Name + " shut down")
}

func (t tools) restore(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolveVirtual(ctx, in.ID, "restore")
	if err != nil {
		return nil, nil, err
	}
	var stages []device.Stage
	if err := p.Restore(ctx, d.ID, func(s device.Stage) {
		if s.Status != device.StageRunning {
			stages = append(stages, s)
		}
	}); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"device": d.ID, "stages": stages, "result": "restored to stock"})
}

func (t tools) measure(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolveVirtual(ctx, in.ID, "measure")
	if err != nil {
		return nil, nil, err
	}
	m, err := p.Measure(ctx, d.ID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"device": d.ID, "footprint_mb": m.FootprintMB, "process_count": m.ProcessCount})
}

func (t tools) screenshot(ctx context.Context, _ *mcp.CallToolRequest, in screenshotIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolve(ctx, in.ID)
	if err != nil {
		return nil, nil, err
	}
	c, err := controller(p)
	if err != nil {
		return nil, nil, err
	}
	path := in.Path
	if path == "" {
		path = filepath.Join(t.ScreenshotDir, fmt.Sprintf("sthin-%s-%d.png", sanitize(d.ID), time.Now().UnixMilli()))
	}
	if err := c.Screenshot(ctx, d.ID, path); err != nil {
		return nil, nil, err
	}
	content := []mcp.Content{&mcp.TextContent{Text: path}}
	if in.Inline {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		content = append(content, &mcp.ImageContent{Data: b, MIMEType: "image/png"})
	}
	return &mcp.CallToolResult{Content: content}, nil, nil
}

func (t tools) tap(ctx context.Context, _ *mcp.CallToolRequest, in tapIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolve(ctx, in.ID)
	if err != nil {
		return nil, nil, err
	}
	c, err := controller(p)
	if err != nil {
		return nil, nil, err
	}
	if err := c.Tap(ctx, d.ID, in.X, in.Y); err != nil {
		return nil, nil, err
	}
	return textResult(fmt.Sprintf("tapped %d,%d on %s", in.X, in.Y, d.Name))
}

func (t tools) install(ctx context.Context, _ *mcp.CallToolRequest, in installIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolve(ctx, in.ID)
	if err != nil {
		return nil, nil, err
	}
	c, err := controller(p)
	if err != nil {
		return nil, nil, err
	}
	if err := c.Install(ctx, d.ID, in.Path); err != nil {
		return nil, nil, err
	}
	return textResult("installed " + in.Path + " on " + d.Name)
}

func (t tools) launch(ctx context.Context, _ *mcp.CallToolRequest, in launchIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolve(ctx, in.ID)
	if err != nil {
		return nil, nil, err
	}
	c, err := controller(p)
	if err != nil {
		return nil, nil, err
	}
	if err := c.Launch(ctx, d.ID, in.App); err != nil {
		return nil, nil, err
	}
	return textResult("launched " + in.App + " on " + d.Name)
}

func (t tools) logs(ctx context.Context, _ *mcp.CallToolRequest, in logsIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolve(ctx, in.ID)
	if err != nil {
		return nil, nil, err
	}
	c, err := controller(p)
	if err != nil {
		return nil, nil, err
	}
	out, err := c.Logs(ctx, d.ID, in.Lines)
	if err != nil {
		return nil, nil, err
	}
	return textResult(out)
}

func (t tools) lease(ctx context.Context, _ *mcp.CallToolRequest, in leaseIn) (*mcp.CallToolResult, any, error) {
	ttl := time.Duration(in.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	var res ops.BootResult
	d, l, err := lease.Acquire(ctx, t.Reg, t.Pool, device.Platform(in.Platform), in.Owner, ttl, func(ctx context.Context, p device.Provider, d device.Device) error {
		res = runBoot(ctx, p, d, device.BootOptions{Headless: true})
		if res.Error != "" {
			return fmt.Errorf("boot %s: %s", d.Name, res.Error)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	res.Device = d
	if res.FootprintMB == nil {
		// Reused an already-booted device: no boot stages, so measure it now.
		if p, ok := t.Reg.Provider(d.Platform); ok {
			if m, err := p.Measure(ctx, d.ID); err == nil {
				mb := m.FootprintMB
				res.FootprintMB = &mb
			}
		}
	}
	if res.Stages == nil {
		res.Stages = []device.Stage{}
	}
	return jsonResult(map[string]any{"device": res.Device, "lease": l, "stages": res.Stages, "footprint_mb": res.FootprintMB})
}

func (t tools) release(ctx context.Context, _ *mcp.CallToolRequest, in releaseIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolveVirtual(ctx, in.ID, "release")
	if err != nil {
		return nil, nil, err
	}
	if err := t.Pool.Release(d.ID); err != nil {
		return nil, nil, err
	}
	msg := "released " + d.Name
	if in.Shutdown {
		if err := p.Shutdown(ctx, d.ID); err != nil {
			return nil, nil, err
		}
		msg += " and shut it down"
	}
	return textResult(msg)
}

func (t tools) leases(_ context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, any, error) {
	all, err := t.Pool.All()
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(all)
}

func sanitize(s string) string {
	out := []rune{}
	for _, r := range s {
		if r == '/' || r == ':' || r == ' ' {
			r = '_'
		}
		out = append(out, r)
	}
	return string(out)
}

func (t tools) run(ctx context.Context, _ *mcp.CallToolRequest, in runIn) (*mcp.CallToolResult, any, error) {
	dir := in.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	var stages []device.Stage
	res, err := project.Run(ctx, project.Deps{Reg: t.Reg, Run: t.Run, Env: t.Env},
		project.Opts{Dir: dir, Device: in.ID, App: in.App, AppID: in.AppID, URL: in.URL, NoHandoff: true},
		func(s device.Stage) {
			if s.Status != device.StageRunning {
				stages = append(stages, s)
			}
		})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"result": res, "stages": stages})
}

func (t tools) openURL(ctx context.Context, _ *mcp.CallToolRequest, in urlIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolve(ctx, in.ID)
	if err != nil {
		return nil, nil, err
	}
	c, err := controller(p)
	if err != nil {
		return nil, nil, err
	}
	if err := c.OpenURL(ctx, d.ID, in.URL); err != nil {
		return nil, nil, err
	}
	return textResult("opened " + in.URL + " on " + d.Name)
}

// deleteDevice removes a shut-down simulator or AVD together with everything
// Sthin remembers about it.
func (t tools) deleteDevice(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolveVirtual(ctx, in.ID, "delete")
	if err != nil {
		return nil, nil, err
	}
	if err := ops.Delete(ctx, p, d, t.Pool); err != nil {
		return nil, nil, err
	}
	return textResult(d.Name + " deleted")
}

func (t tools) rename(ctx context.Context, _ *mcp.CallToolRequest, in renameIn) (*mcp.CallToolResult, any, error) {
	p, d, err := t.resolveVirtual(ctx, in.ID, "rename")
	if err != nil {
		return nil, nil, err
	}
	rn, ok := p.(device.Renamer)
	if !ok {
		return nil, nil, fmt.Errorf("%s provider cannot rename devices", d.Platform)
	}
	if err := rn.Rename(ctx, d.ID, in.Name); err != nil {
		return nil, nil, err
	}
	return textResult(d.Name + " renamed to " + in.Name)
}
