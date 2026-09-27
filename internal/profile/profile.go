// Package profile loads the embedded per-platform Profiles: which Managed items
// Sthin disables, grouped into Categories, plus the Never-disable set and the
// runtime each Profile was validated against.
package profile

import (
	"embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed ios.yaml android.yaml
var files embed.FS

// Category is a named group of Managed items kept or dropped together.
type Category struct {
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Default  bool     `yaml:"default" json:"default"`
	Items    []string `yaml:"items" json:"items"`
	ApproxMB int      `yaml:"approx_mb,omitempty" json:"approx_mb,omitempty"`
}

// Profile is one platform's slimming data.
type Profile struct {
	Platform         string            `yaml:"platform" json:"platform"`
	Version          int               `yaml:"version" json:"version"`
	ValidatedAgainst map[string]string `yaml:"validated_against" json:"validated_against"`
	NeverDisable     []string          `yaml:"never_disable" json:"never_disable"`
	Categories       []Category        `yaml:"categories" json:"categories"`
	Aggressive       []string          `yaml:"aggressive" json:"aggressive"`
}

// Load reads and validates the embedded Profile for "ios" or "android".
func Load(platform string) (*Profile, error) {
	b, err := files.ReadFile(platform + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("unknown platform %q (want ios or android)", platform)
	}
	return Parse(b)
}

// MustLoad is Load for the embedded files, which are validated by tests.
func MustLoad(platform string) *Profile {
	p, err := Load(platform)
	if err != nil {
		panic(err)
	}
	return p
}

// Parse decodes and validates a Profile.
func Parse(b []byte) (*Profile, error) {
	var p Profile
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks unique category IDs, no empty category, and no Never-disable
// item inside any category.
func (p *Profile) Validate() error {
	never := toSet(p.NeverDisable)
	seen := map[string]bool{}
	for _, c := range p.Categories {
		if c.ID == "" {
			return fmt.Errorf("profile %s: category with empty id", p.Platform)
		}
		if seen[c.ID] {
			return fmt.Errorf("profile %s: duplicate category id %q", p.Platform, c.ID)
		}
		seen[c.ID] = true
		if len(c.Items) == 0 {
			return fmt.Errorf("profile %s: category %q is empty", p.Platform, c.ID)
		}
		for _, it := range c.Items {
			if never[it] {
				return fmt.Errorf("profile %s: category %q contains never-disable item %q", p.Platform, c.ID, it)
			}
		}
	}
	for _, it := range p.Aggressive {
		if never[it] {
			return fmt.Errorf("profile %s: aggressive set contains never-disable item %q", p.Platform, it)
		}
	}
	return nil
}

// CategoryIDs returns every category ID in file order.
func (p *Profile) CategoryIDs() []string {
	ids := make([]string, len(p.Categories))
	for i, c := range p.Categories {
		ids[i] = c.ID
	}
	return ids
}

// CheckExcept returns an error naming any ID that is not a category.
func (p *Profile) CheckExcept(except []string) error {
	known := toSet(p.CategoryIDs())
	for _, e := range except {
		if !known[e] {
			return fmt.Errorf("unknown category %q (known: %s)", e, strings.Join(p.CategoryIDs(), ", "))
		}
	}
	return nil
}

// DesiredSet returns the items a slim boot disables: every default category's
// items, minus every item of an excepted category (even when another category
// lists it too), minus the Never-disable set. Order is file order, deduplicated.
func DesiredSet(p *Profile, except []string) []string {
	ex := toSet(except)
	keep := toSet(p.NeverDisable)
	for _, c := range p.Categories {
		if ex[c.ID] {
			for _, it := range c.Items {
				keep[it] = true
			}
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range p.Categories {
		if !c.Default || ex[c.ID] {
			continue
		}
		for _, it := range c.Items {
			if keep[it] || seen[it] {
				continue
			}
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}

// Newer reports whether installed is a later runtime than validated. Both are
// dotted numeric versions ("26.5", "27.0") or API levels ("33", "36").
func Newer(validated, installed string) bool {
	return CompareVersions(installed, validated) > 0
}

// CompareVersions compares dotted numeric versions: -1, 0, or 1.
func CompareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			fmt.Sscanf(as[i], "%d", &x)
		}
		if i < len(bs) {
			fmt.Sscanf(bs[i], "%d", &y)
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
