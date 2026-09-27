package android

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/proc"
)

// hostSnapshot is one ps + top sample shared across AVDs.
type hostSnapshot struct {
	args string          // `ps -axo pid,args`
	mem  map[int]float64 // pid → phys_footprint bytes
}

// windowsProcessQuery lists processes as "pid|workingSetBytes|commandLine" lines.
const windowsProcessQuery = `Get-CimInstance Win32_Process | ForEach-Object { "$($_.ProcessId)|$($_.WorkingSetSize)|$($_.CommandLine)" }`

// takeSnapshot samples host processes and their memory the way each OS allows:
// macOS phys_footprint from top, Linux RSS from ps, Windows working set from
// PowerShell. Only macOS offers phys_footprint, so the other figures are the
// nearest equivalent and are labelled as such in the list.
func (p *Provider) takeSnapshot(ctx context.Context) (*hostSnapshot, error) {
	switch p.Env.GOOS {
	case "windows":
		out, stderr, err := p.Run.Run(ctx, "powershell", "-NoProfile", "-Command", windowsProcessQuery)
		if err != nil {
			return nil, fmt.Errorf("powershell process query: %w: %s", err, strings.TrimSpace(string(stderr)))
		}
		args, mem := parseWindowsProcesses(string(out))
		return &hostSnapshot{args: args, mem: mem}, nil
	case "darwin":
		psOut, _, err := p.Run.Run(ctx, "ps", "-axo", "pid,args")
		if err != nil {
			return nil, err
		}
		topOut, _, err := p.Run.Run(ctx, "top", "-l", "1", "-stats", "pid,mem")
		if err != nil {
			return nil, err
		}
		return &hostSnapshot{args: string(psOut), mem: proc.ParseTop(string(topOut))}, nil
	default:
		out, _, err := p.Run.Run(ctx, "ps", "-eo", "pid,rss,args")
		if err != nil {
			return nil, err
		}
		args, mem := parseLinuxPS(string(out))
		return &hostSnapshot{args: args, mem: mem}, nil
	}
}

// parseWindowsProcesses turns "pid|bytes|cmdline" lines into the ps-style
// "pid args" text qemuPID reads plus a pid → bytes map.
func parseWindowsProcesses(out string) (string, map[int]float64) {
	var b strings.Builder
	mem := map[int]float64{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) != 3 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if bytes, err := strconv.ParseFloat(parts[1], 64); err == nil {
			mem[pid] = bytes
		}
		// Command lines quote the executable path; strip quotes so the
		// qemu-system check and the -avd scan see plain tokens.
		fmt.Fprintf(&b, "%d %s\n", pid, strings.ReplaceAll(parts[2], "\"", ""))
	}
	return b.String(), mem
}

// parseLinuxPS turns `ps -eo pid,rss,args` into "pid args" text and a pid → bytes map (RSS in KiB).
func parseLinuxPS(out string) (string, map[int]float64) {
	var b strings.Builder
	mem := map[int]float64{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if kib, err := strconv.ParseFloat(f[1], 64); err == nil {
			mem[pid] = kib * 1024
		}
		fmt.Fprintf(&b, "%d %s\n", pid, strings.Join(f[2:], " "))
	}
	return b.String(), mem
}

// qemuPID finds the qemu-system-* process whose arguments contain `-avd <name>`.
func qemuPID(psArgs, name string) (int, bool) {
	for _, line := range strings.Split(psArgs, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || !strings.Contains(f[1], "qemu-system") {
			continue
		}
		for i := 2; i+1 < len(f); i++ {
			if f[i] == "-avd" && f[i+1] == name {
				pid, err := strconv.Atoi(f[0])
				return pid, err == nil
			}
		}
	}
	return 0, false
}

var footprintRE = regexp.MustCompile(`Footprint:\s+([\d.]+)\s*(B|KB|MB|GB)`)

// parseFootprint reads the dirty footprint from `footprint -p <pid>`'s header.
func parseFootprint(out string) (int, bool) {
	m := footprintRE.FindStringSubmatch(out)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	switch m[2] {
	case "B":
		v /= 1 << 20
	case "KB":
		v /= 1 << 10
	case "GB":
		v *= 1 << 10
	}
	return int(math.Round(v)), true
}

var meminfoRE = regexp.MustCompile(`(?m)^\s*(Total|Used) RAM:\s+([\d,]+)K`)

// parseMeminfo returns guest Total and Used RAM in MB from `dumpsys meminfo`.
func parseMeminfo(out string) (total, used int, ok bool) {
	var gotT, gotU bool
	for _, m := range meminfoRE.FindAllStringSubmatch(out, -1) {
		kb, err := strconv.Atoi(strings.ReplaceAll(m[2], ",", ""))
		if err != nil {
			continue
		}
		mb := int(math.Round(float64(kb) / 1024))
		if m[1] == "Total" {
			total, gotT = mb, true
		} else {
			used, gotU = mb, true
		}
	}
	return total, used, gotT && gotU
}

func (p *Provider) measureWith(ctx context.Context, snap *hostSnapshot, name, serial string) (device.Measurement, error) {
	pid, ok := qemuPID(snap.args, name)
	if !ok {
		return device.Measurement{}, device.ErrNotBooted
	}
	m := device.Measurement{ID: name, Platform: device.Android, ProcessCount: 1,
		FootprintMB: int(math.Round(snap.mem[pid] / (1 << 20)))}
	if p.Env.GOOS == "darwin" { // footprint(1) is macOS-only
		if out, _, err := p.Run.Run(ctx, "footprint", "-p", strconv.Itoa(pid)); err == nil {
			if d, ok := parseFootprint(string(out)); ok {
				m.DirtyMB = &d
			}
		}
	}
	if serial != "" {
		if out, err := p.shell(ctx, serial, "dumpsys", "meminfo"); err == nil {
			if t, u, ok := parseMeminfo(out); ok {
				m.GuestTotalMB, m.GuestUsedMB = &t, &u
			}
		}
	}
	return m, nil
}

// Measure implements device.Provider.
func (p *Provider) Measure(ctx context.Context, id string) (device.Measurement, error) {
	if _, err := p.findAVD(id); err != nil {
		return device.Measurement{}, err
	}
	serial, ok := p.running(ctx)[id]
	if !ok {
		return device.Measurement{}, device.ErrNotBooted
	}
	snap, err := p.takeSnapshot(ctx)
	if err != nil {
		return device.Measurement{}, err
	}
	return p.measureWith(ctx, snap, id, serial)
}

// measureStage reports the footprint; a measurement failure only warns.
func (p *Provider) measureStage(ctx context.Context, st device.Stages, id string) {
	st.R.Report(device.Stage{Name: stageMeasure, Status: device.StageRunning})
	m, err := p.Measure(ctx, id)
	if err != nil {
		st.Warn(stageMeasure, "could not measure: "+err.Error())
		return
	}
	d := fmt.Sprintf("%d MB host footprint", m.FootprintMB)
	if m.DirtyMB != nil {
		d += fmt.Sprintf(", %d MB dirty", *m.DirtyMB)
	}
	if m.GuestUsedMB != nil {
		d += fmt.Sprintf(", guest %d/%d MB used", *m.GuestUsedMB, *m.GuestTotalMB)
	}
	st.R.Report(device.Stage{Name: stageMeasure, Status: device.StageOK, Detail: d, Measurement: &m})
}
