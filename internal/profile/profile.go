// Package profile defines the environment profile and layer structure for DOTF.
package profile

import (
	"fmt"
	"strings"
)

// Profile represents a fully detected Linux environment.
type Profile struct {
	Distro     string `json:"distro"`
	Session    string `json:"session"` // "wayland" or "x11"
	WM         string `json:"wm"`      // window manager or compositor
	DE         string `json:"de"`      // desktop environment (if any)
	Shell      string `json:"shell"`
	Terminal   string `json:"terminal"`
	GPU        string `json:"gpu"`
	Hostname   string `json:"hostname"`
	DeviceType string `json:"device_type"` // laptop, desktop, server, vm, container
}

// LayerType is a category of configuration layer.
type LayerType string

const (
	LayerHost     LayerType = "host"
	LayerDevice   LayerType = "device"
	LayerGPU      LayerType = "gpu"
	LayerTerminal LayerType = "terminal"
	LayerShell    LayerType = "shell"
	LayerWM       LayerType = "wm"
	LayerDesktop  LayerType = "desktop"
	LayerDistro   LayerType = "distro"
	LayerBase     LayerType = "base"
)

// AllLayerTypes returns all layer types in priority order (highest first).
func AllLayerTypes() []LayerType {
	return []LayerType{
		LayerHost,
		LayerDevice,
		LayerGPU,
		LayerTerminal,
		LayerShell,
		LayerWM,
		LayerDesktop,
		LayerDistro,
		LayerBase,
	}
}

// Layer represents a resolved configuration layer with its directory path.
type Layer struct {
	Type    LayerType
	Name    string // e.g., "arch", "qtile", "fish"
	DirPath string // absolute path to the layer directory
}

// Path returns the canonical layer path, e.g., "distro/arch".
func (l Layer) Path() string {
	if l.Type == LayerBase {
		return "base"
	}
	return fmt.Sprintf("%s/%s", l.Type, l.Name)
}

// ResolvedLayers returns the ordered list of layers resolved from this profile.
// Returns them from lowest to highest priority (base first, host last).
func (p Profile) ResolvedLayers(layersDir string) []Layer {
	var layers []Layer

	if layersDir == "" {
		layersDir = "layers"
	}

	// Helper to append a layer if the name is non-empty
	add := func(lt LayerType, name string) {
		if name == "" {
			return
		}
		layers = append(layers, Layer{
			Type:    lt,
			Name:    name,
			DirPath: fmt.Sprintf("%s/%s/%s", layersDir, lt, strings.ToLower(name)),
		})
	}

	// Always include base
	layers = append(layers, Layer{
		Type:    LayerBase,
		Name:    "",
		DirPath: fmt.Sprintf("%s/base", layersDir),
	})

	add(LayerDistro, p.Distro)
	add(LayerDesktop, p.DE)
	add(LayerWM, p.WM)
	add(LayerShell, p.Shell)
	add(LayerTerminal, p.Terminal)
	add(LayerGPU, p.GPU)
	add(LayerDevice, p.DeviceType)
	add(LayerHost, p.Hostname)

	return layers
}

// String returns a human-readable representation of the profile.
func (p Profile) String() string {
	var b strings.Builder
	b.WriteString("Detected:\n")
	writeField(&b, "  distro", p.Distro)
	writeField(&b, "  session", p.Session)
	writeField(&b, "  wm", p.WM)
	writeField(&b, "  desktop", p.DE)
	writeField(&b, "  shell", p.Shell)
	writeField(&b, "  terminal", p.Terminal)
	writeField(&b, "  gpu", p.GPU)
	writeField(&b, "  hostname", p.Hostname)
	writeField(&b, "  device_type", p.DeviceType)
	return b.String()
}

func writeField(b *strings.Builder, key, value string) {
	if value == "" {
		fmt.Fprintf(b, "%s: -\n", key)
	} else {
		fmt.Fprintf(b, "%s: %s\n", key, value)
	}
}
