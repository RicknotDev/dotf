package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RicknotDev/dotf/internal/detect"
	"github.com/RicknotDev/dotf/internal/layer"
	"github.com/RicknotDev/dotf/internal/profile"
)

// DetectOverrides force detected values. "none" disables that layer type.
type DetectOverrides struct {
	Distro   string `yaml:"distro,omitempty"`
	Desktop  string `yaml:"desktop,omitempty"`
	WM       string `yaml:"wm,omitempty"`
	Shell    string `yaml:"shell,omitempty"`
	Terminal string `yaml:"terminal,omitempty"`
	GPU      string `yaml:"gpu,omitempty"`
	Device   string `yaml:"device,omitempty"`
	Host     string `yaml:"host,omitempty"`
}

// ProfileSpec selects layers: detection overrides plus extra and removed layers.
type ProfileSpec struct {
	Detect  DetectOverrides `yaml:"detect,omitempty"`
	Include []string        `yaml:"include,omitempty"`
	Exclude []string        `yaml:"exclude,omitempty"`
}

// DotfConfig is the optional dotf.yaml (or dotf.yml) at the repository root.
//
//	profile: laptop          # profile used when --profile is not given
//	detect: {shell: zsh}     # applies to every profile
//	include: [wm/sway]
//	exclude: [gpu/nvidia]
//	profiles:
//	  laptop:
//	    detect: {device: laptop}
//	    include: [host/work]
type DotfConfig struct {
	ProfileSpec `yaml:",inline"`

	Profile  string                 `yaml:"profile,omitempty"`
	Profiles map[string]ProfileSpec `yaml:"profiles,omitempty"`

	// Layers is the v0.6 auto-generated layer list. It is accepted for
	// compatibility and ignored: layers come from detection.
	Layers []string `yaml:"layers,omitempty"`
	// Hooks.Enabled is ignored: a repository cannot enable its own hooks.
	// Use --allow-hooks or DOTF_ALLOW_HOOKS=1.
	Hooks struct {
		Enabled bool `yaml:"enabled,omitempty"`
	} `yaml:"hooks,omitempty"`
}

// LoadDotfConfig loads dotf.yaml (preferred) or dotf.yml from repoRoot.
// It returns nil, nil when neither exists.
func LoadDotfConfig(repoRoot string) (*DotfConfig, error) {
	for _, name := range []string{"dotf.yaml", "dotf.yml"} {
		path := filepath.Join(repoRoot, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("cannot read %s: %w", path, err)
		}
		var cfg DotfConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("cannot parse %s: %w", path, err)
		}
		return &cfg, nil
	}
	return nil, nil
}

// ProfileNames returns the names of the configured profiles, sorted.
func (c *DotfConfig) ProfileNames() []string {
	var names []string
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Select merges the top-level settings with the named profile. name is the
// --profile flag; when empty, the config's own "profile:" is used. It
// returns the effective spec, the profile name in use ("" for none) and
// warnings.
func (c *DotfConfig) Select(name string) (ProfileSpec, string, []string, error) {
	var warnings []string
	if c == nil {
		if name != "" {
			return ProfileSpec{}, "", nil, fmt.Errorf("unknown profile %q: no dotf.yaml in the repository", name)
		}
		return ProfileSpec{}, "", nil, nil
	}
	if len(c.Layers) > 0 {
		warnings = append(warnings, "dotf.yaml: 'layers:' is ignored (layers come from detection; use include/exclude)")
	}
	if c.Hooks.Enabled {
		warnings = append(warnings, "dotf.yaml: 'hooks.enabled' is ignored; use --allow-hooks or DOTF_ALLOW_HOOKS=1")
	}

	spec := c.ProfileSpec
	fromFlag := name != ""
	if !fromFlag {
		name = c.Profile
		if name == "auto" {
			name = ""
		}
	}
	if name == "" {
		return spec, "", warnings, nil
	}
	p, ok := c.Profiles[name]
	if !ok {
		msg := fmt.Sprintf("unknown profile %q", name)
		if names := c.ProfileNames(); len(names) > 0 {
			msg += " (available: " + strings.Join(names, ", ") + ")"
		}
		if fromFlag {
			return spec, "", warnings, fmt.Errorf("%s", msg)
		}
		return spec, "", append(warnings, "dotf.yaml: "+msg+"; using detection only"), nil
	}
	d := &spec.Detect
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&d.Distro, p.Detect.Distro}, {&d.Desktop, p.Detect.Desktop}, {&d.WM, p.Detect.WM},
		{&d.Shell, p.Detect.Shell}, {&d.Terminal, p.Detect.Terminal}, {&d.GPU, p.Detect.GPU},
		{&d.Device, p.Detect.Device}, {&d.Host, p.Detect.Host},
	} {
		if f.src != "" {
			*f.dst = f.src
		}
	}
	spec.Include = append(append([]string{}, spec.Include...), p.Include...)
	spec.Exclude = append(append([]string{}, spec.Exclude...), p.Exclude...)
	return spec, name, warnings, nil
}

// applyDetect overrides detected values with the spec.
func applyDetect(p *profile.Profile, d DetectOverrides) error {
	for _, f := range []struct {
		key string
		dst *string
		val string
	}{
		{"distro", &p.Distro, d.Distro}, {"desktop", &p.DE, d.Desktop}, {"wm", &p.WM, d.WM},
		{"shell", &p.Shell, d.Shell}, {"terminal", &p.Terminal, d.Terminal}, {"gpu", &p.GPU, d.GPU},
		{"device", &p.DeviceType, d.Device}, {"host", &p.Hostname, d.Host},
	} {
		switch f.val {
		case "":
		case "none":
			*f.dst = ""
		default:
			v := profile.SafeName(f.val)
			if v == "" {
				return fmt.Errorf("dotf.yaml: invalid %s value %q", f.key, f.val)
			}
			*f.dst = v
		}
	}
	return nil
}

// selection is the outcome of detection plus dotf.yaml.
type selection struct {
	Profile     profile.Profile
	ProfileName string
	Config      *DotfConfig
	Result      *layer.ResolveResult
	Warnings    []string
}

// selectLayers detects the environment, applies dotf.yaml and the --profile
// flag, and resolves the active layers.
func selectLayers(repoRoot, profileFlag string) (*selection, error) {
	cfg, err := LoadDotfConfig(repoRoot)
	if err != nil {
		return nil, err
	}
	spec, name, warnings, err := cfg.Select(profileFlag)
	if err != nil {
		return nil, err
	}
	p := detect.Detect()
	if err := applyDetect(&p, spec.Detect); err != nil {
		return nil, err
	}
	result, err := layer.ResolveSelection(repoRoot, p, spec.Include, spec.Exclude)
	if err != nil {
		return nil, err
	}
	return &selection{Profile: p, ProfileName: name, Config: cfg, Result: result, Warnings: warnings}, nil
}
