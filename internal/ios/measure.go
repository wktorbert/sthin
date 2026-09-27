package ios

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/proc"
)

// snapshot is one host-wide view of processes: the parent tree from ps and
// per-process phys_footprint (bytes) from a single `top -l 1` sample.
type snapshot struct {
	children map[int][]int
	mem      map[int]float64
}

// takeSnapshot runs ps and top once; callers share it across devices.
func (p *Provider) takeSnapshot(ctx context.Context) (*snapshot, error) {
	psOut, _, err := p.Run.Run(ctx, "ps", "-axo", "pid,ppid,comm")
	if err != nil {
		return nil, err
	}
	topOut, _, err := p.Run.Run(ctx, "top", "-l", "1", "-stats", "pid,mem")
	if err != nil {
		return nil, err
	}
	return &snapshot{children: proc.ParseTree(string(psOut)), mem: proc.ParseTop(string(topOut))}, nil
}

// tree sums phys_footprint over root and all its descendants.
func (s *snapshot) tree(root int) (mb int, count int) {
	var total float64
	queue := []int{root}
	seen := map[int]bool{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		count++
		total += s.mem[pid]
		queue = append(queue, s.children[pid]...)
	}
	return int(math.Round(total / (1 << 20))), count
}

// launchdSim finds the simulator's launchd_sim pid; ErrNotBooted when absent.
func (p *Provider) launchdSim(ctx context.Context, udid string) (int, error) {
	out, _, err := p.Run.Run(ctx, "pgrep", "-f", udid+"/data/var/run/launchd_bootstrap")
	if err != nil {
		return 0, device.ErrNotBooted // pgrep exits 1 when nothing matches
	}
	for _, line := range strings.Split(string(out), "\n") {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			return pid, nil
		}
	}
	return 0, device.ErrNotBooted
}

// measureWith measures one simulator against a shared snapshot.
func (p *Provider) measureWith(ctx context.Context, snap *snapshot, udid string) (device.Measurement, error) {
	pid, err := p.launchdSim(ctx, udid)
	if err != nil {
		return device.Measurement{}, err
	}
	mb, n := snap.tree(pid)
	return device.Measurement{ID: udid, Platform: device.IOS, FootprintMB: mb, ProcessCount: n}, nil
}

// Measure implements device.Provider.
func (p *Provider) Measure(ctx context.Context, id string) (device.Measurement, error) {
	s, err := p.find(ctx, id)
	if err != nil {
		return device.Measurement{}, err
	}
	if mapState(s.State) != device.Booted {
		return device.Measurement{}, device.ErrNotBooted
	}
	snap, err := p.takeSnapshot(ctx)
	if err != nil {
		return device.Measurement{}, err
	}
	return p.measureWith(ctx, snap, id)
}

// fillFootprints sets FootprintMB on booted devices using one shared snapshot.
func (p *Provider) fillFootprints(ctx context.Context, devs []device.Device) {
	var snap *snapshot
	for i := range devs {
		if devs[i].State != device.Booted {
			continue
		}
		if snap == nil {
			var err error
			if snap, err = p.takeSnapshot(ctx); err != nil {
				return
			}
		}
		m, err := p.measureWith(ctx, snap, devs[i].ID)
		if errors.Is(err, device.ErrNotBooted) || err != nil {
			continue
		}
		mb := m.FootprintMB
		devs[i].FootprintMB = &mb
	}
}
