package ios

import (
	"github.com/wktorbert/sthin/internal/device"
	"github.com/wktorbert/sthin/internal/profile"
)

// minPersistVersion is the first runtime whose launchd honours the Override store across boots.
const minPersistVersion = "18.5"

func (p *Provider) profile() *profile.Profile {
	if p.Profile == nil {
		p.Profile = profile.MustLoad("ios")
	}
	return p.Profile
}

// slimState derives the slim state from the Override store: every default
// Profile item disabled = slim, none = stock, some = partial, unreadable = unknown.
func (p *Provider) slimState(udid string) device.SlimState {
	cur, err := p.store(udid).Read()
	if err != nil {
		return device.Unknown
	}
	// Slim means "matches this device's own selection": kept categories do
	// not count against it.
	except, _ := p.State.ExceptFor(udid, nil)
	return classify(profile.DesiredSet(p.profile(), except), cur)
}

func classify(desired []string, disabled map[string]bool) device.SlimState {
	n := 0
	for _, l := range desired {
		if disabled[l] {
			n++
		}
	}
	switch {
	case len(desired) == 0 || n == 0:
		return device.Stock
	case n == len(desired):
		return device.Slim
	default:
		return device.Partial
	}
}

// persistsOverrides reports whether a runtime keeps Override store entries across boots.
func persistsOverrides(version string) bool {
	return profile.CompareVersions(version, minPersistVersion) >= 0
}

// warnings lists what the user should know about a runtime before slimming it.
func (p *Provider) warnings(version string) []string {
	w := []string{}
	if v := p.profile().ValidatedAgainst["ios"]; v != "" && profile.Newer(v, version) {
		w = append(w, "runtime newer than profile (validated against iOS "+v+")")
	}
	if !persistsOverrides(version) {
		w = append(w, "runtime does not persist overrides (needs iOS "+minPersistVersion+"+)")
	}
	return w
}
