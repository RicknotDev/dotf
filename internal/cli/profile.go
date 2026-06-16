package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DotfConfig represents the dotf.yaml configuration file.
type DotfConfig struct {
	Profile string   `yaml:"profile,omitempty"`
	Layers  []string `yaml:"layers,omitempty"`
	Hooks   struct {
		Enabled bool `yaml:"enabled,omitempty"`
	} `yaml:"hooks,omitempty"`
}

// LoadDotfConfig loads the dotf.yaml configuration from a repository.
// Looks for dotf.yaml in the repository root.
func LoadDotfConfig(repoRoot string) (*DotfConfig, error) {
	candidates := []string{
		filepath.Join(repoRoot, "dotf.yaml"),
		filepath.Join(repoRoot, "dotf.yml"),
	}

	for _, path := range candidates {
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

	return nil, nil // no config file found
}

// InferDotfConfig generates a dotf.yaml configuration from the repository structure.
func InferDotfConfig(repoRoot string) (*DotfConfig, error) {
	layersDir := filepath.Join(repoRoot, "layers")
	entries, err := os.ReadDir(layersDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read layers directory: %w", err)
	}

	var layerPaths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "base" {
			layerPaths = append(layerPaths, "base")
			continue
		}
		// Check for sub-layers (e.g., distro/arch)
		subEntries, err := os.ReadDir(filepath.Join(layersDir, name))
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if sub.IsDir() {
				layerPaths = append(layerPaths, name+"/"+sub.Name())
			}
		}
	}

	return &DotfConfig{
		Profile: "auto",
		Layers:  layerPaths,
	}, nil
}
