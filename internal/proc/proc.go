// Package proc parses host process listings (ps, top) shared by both Providers.
package proc

import (
	"strconv"
	"strings"
)

// ParseTree returns ppid → child pids from `ps -axo pid,ppid,comm`.
func ParseTree(out string) map[int][]int {
	kids := map[int][]int{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		kids[ppid] = append(kids[ppid], pid)
	}
	return kids
}

// ParseTop returns pid → bytes from `top -l 1 -stats pid,mem`. The MEM column
// is phys_footprint with a B/K/M/G suffix and an optional +/- trend marker.
func ParseTop(out string) map[int]float64 {
	mem := map[int]float64{}
	inTable := false
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "PID" && f[1] == "MEM" {
			inTable = true
			continue
		}
		if !inTable || len(f) < 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if b, ok := ParseSize(f[1]); ok {
			mem[pid] = b
		}
	}
	return mem
}

// ParseSize parses top's "3552K", "17M+", "1.2G-", "512B" into bytes.
func ParseSize(s string) (float64, bool) {
	s = strings.TrimRight(s, "+-")
	if s == "" {
		return 0, false
	}
	mult := 1.0
	switch s[len(s)-1] {
	case 'B':
		s = s[:len(s)-1]
	case 'K':
		mult, s = 1<<10, s[:len(s)-1]
	case 'M':
		mult, s = 1<<20, s[:len(s)-1]
	case 'G':
		mult, s = 1<<30, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v * mult, true
}
