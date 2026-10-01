package complete

import (
	"sort"
	"strings"

	"github.com/wktorbert/sthin/internal/device"
)

// Filter decides which Devices a command can act on, so completion offers
// only what Enter would accept.
type Filter func(device.Device) bool

// Any offers every Device.
func Any(device.Device) bool { return true }

// Virtual omits physical devices: for commands that change a Device.
func Virtual(d device.Device) bool { return d.Kind != device.Physical }

// Booted offers running Devices: measure, logs, shutdown, run.
func Booted(d device.Device) bool { return d.State == device.Booted }

// ShutdownVirtual is what delete accepts.
func ShutdownVirtual(d device.Device) bool { return Virtual(d) && d.State == device.Shutdown }

// Only keeps Devices whose ID is in ids: release offers leased Devices.
func Only(ids []string) Filter {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return func(d device.Device) bool { return set[d.ID] }
}

// Devices returns completion candidates, each "value\tdescription" as Cobra
// expects, for the Devices passing f whose ID or name starts with prefix.
//
// Every Device yields its ID. It also yields its name, but only when no other
// listed Device shares that name: a shared name is not a Device reference
// (Resolve rejects it as ambiguous), so offering it would complete to an
// error. Shared-name Devices are told apart by the OS version in their
// description. Names sort first, then IDs.
func Devices(devs []device.Device, f Filter, prefix string) []string {
	shared := map[string]int{}
	for _, d := range devs {
		shared[d.Name]++
	}
	var names, ids []string
	for _, d := range devs {
		if !f(d) {
			continue
		}
		desc := describe(d)
		if shared[d.Name] == 1 && strings.HasPrefix(strings.ToLower(d.Name), strings.ToLower(prefix)) {
			names = append(names, d.Name+"\t"+desc)
		}
		if strings.HasPrefix(strings.ToLower(d.ID), strings.ToLower(prefix)) {
			ids = append(ids, d.ID+"\t"+desc)
		}
	}
	sort.Strings(names)
	sort.Strings(ids)
	return append(names, ids...)
}

// describe is the text shown next to a candidate: platform, OS version, the
// name (so an ID is still recognisable), and the state.
func describe(d device.Device) string {
	parts := []string{string(d.Platform)}
	if d.OSVersion != "" {
		parts = append(parts, d.OSVersion)
	}
	parts = append(parts, d.Name, string(d.State))
	return strings.Join(parts, " · ")
}

// Values filters a fixed value set by prefix, for flags such as --level.
func Values(all []string, prefix string) []string {
	var out []string
	for _, v := range all {
		if strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	return out
}

// ListValues completes the last item of a comma-separated flag value such as
// --except siri,sto: the already typed items stay in front of every candidate
// and are not offered again.
func ListValues(all []string, typed string) []string {
	head, last := "", typed
	if i := strings.LastIndex(typed, ","); i >= 0 {
		head, last = typed[:i+1], typed[i+1:]
	}
	chosen := map[string]bool{}
	for _, v := range strings.Split(head, ",") {
		chosen[v] = true
	}
	var out []string
	for _, v := range Values(all, last) {
		if !chosen[v] {
			out = append(out, head+v)
		}
	}
	return out
}

// Platforms are the values --platform accepts.
var Platforms = []string{string(device.IOS), string(device.Android)}

// Levels are the values logs --level accepts, in severity order.
var Levels = []string{"verbose", "debug", "info", "warn", "error", "fatal"}

// PlatformOf returns the platform of the Device a typed reference names, or
// "" when it names none or several, so --except can pick the right Profile.
func PlatformOf(devs []device.Device, ref string) device.Platform {
	var found device.Platform
	n := 0
	for _, d := range devs {
		if d.ID == ref || d.Name == ref {
			found = d.Platform
			n++
		}
	}
	if n != 1 {
		return ""
	}
	return found
}
