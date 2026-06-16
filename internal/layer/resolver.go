// Package layer resolves configuration layers from the repository and installs them.
package layer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codebuff/dotf/internal/profile"
)

// ResolvedLayer represents an existing (on-disk) configuration layer.
type ResolvedLayer struct {
	Type    profile.LayerType
	Name    string
	DirPath string // absolute path to the layer directory
}

// Path returns the canonical layer path, e.g., "distro/arch".
func (l ResolvedLayer) Path() string {
	if l.Type == profile.LayerBase {
		return "base"
	}
	return fmt.Sprintf("%s/%s", l.Type, l.Name)
}

// ResolveResult contains the full resolution output.
type ResolveResult struct {
	Profile profile.Profile
	Layers  []ResolvedLayer // existing layers, from lowest to highest priority
	Missing []profile.Layer // layers that were detected but don't exist in the repo
}

// Resolve resolves layers from a profile against the repository's layers directory.
// Returns only layers that actually exist on disk.
func Resolve(repoRoot string, p profile.Profile) (*ResolveResult, error) {
	layersDir := filepath.Join(repoRoot, "layers")

	allLayers := p.ResolvedLayers(layersDir)

	var resolved []ResolvedLayer
	var missing []profile.Layer

	for _, l := range allLayers {
		// Check if the layer directory exists
		info, err := os.Stat(l.DirPath)
		if err != nil || !info.IsDir() {
			missing = append(missing, l)
			continue
		}
		resolved = append(resolved, ResolvedLayer{
			Type:    l.Type,
			Name:    l.Name,
			DirPath: l.DirPath,
		})
	}

	return &ResolveResult{
		Profile: p,
		Layers:  resolved,
		Missing: missing,
	}, nil
}

// FileOverride represents a file that exists in multiple layers.
type FileOverride struct {
	File       string // relative file path
	Overriding string // higher-priority layer path
	Overridden string // lower-priority layer path
}

// ComputeOverrides finds files that exist in multiple layers (for explain).
func ComputeOverrides(layers []ResolvedLayer) []FileOverride {
	type fileEntry struct {
		layerPath string
		layerType profile.LayerType
	}

	// Collect all files with their layers (lowest priority first)
	fileLayers := make(map[string][]fileEntry)

	for _, l := range layers {
		err := filepath.WalkDir(l.DirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(l.DirPath, path)
			if err != nil {
				return err
			}
			fileLayers[rel] = append(fileLayers[rel], fileEntry{
				layerPath: l.Path(),
				layerType: l.Type,
			})
			return nil
		})
		if err != nil {
			continue
		}
	}

	// Priority order (highest first) for determining which layers override which
	priorityOrder := profile.AllLayerTypes()
	priorityIndex := make(map[profile.LayerType]int)
	for i, lt := range priorityOrder {
		priorityIndex[lt] = i
	}

	var overrides []FileOverride
	for file, entries := range fileLayers {
		if len(entries) < 2 {
			continue
		}

		// Sort entries by priority (highest priority first = lowest index)
		sort.Slice(entries, func(i, j int) bool {
			return priorityIndex[entries[i].layerType] < priorityIndex[entries[j].layerType]
		})

		// The first entry is the highest priority (the winner)
		// The rest are overridden
		for i := 1; i < len(entries); i++ {
			overrides = append(overrides, FileOverride{
				File:       file,
				Overriding: entries[0].layerPath,
				Overridden: entries[i].layerPath,
			})
		}
	}

	return overrides
}

// FormatOverrideGroups groups overrides by the overriding layer for display.
func FormatOverrideGroups(overrides []FileOverride) string {
	if len(overrides) == 0 {
		return "  (none)\n"
	}

	// Group by overriding layer
	groups := make(map[string][]string)
	for _, o := range overrides {
		key := o.Overriding + " overrides " + o.Overridden
		groups[key] = append(groups[key], o.File)
	}

	// Sort keys for deterministic output
	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s:\n", k)
		for _, f := range groups[k] {
			fmt.Fprintf(&b, "    - %s\n", f)
		}
	}
	return b.String()
}
