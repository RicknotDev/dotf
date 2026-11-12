package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotfConfigNonexistent(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() for dir without config should not error: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config for dir without dotf.yaml")
	}
}

func TestLoadDotfConfigValid(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `profile: laptop
layers:
  - base
  - shell/zsh
hooks:
  enabled: true
`
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Profile != "laptop" {
		t.Fatalf("expected profile 'laptop', got '%s'", cfg.Profile)
	}
	if len(cfg.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(cfg.Layers))
	}
	if cfg.Layers[0] != "base" {
		t.Fatalf("expected first layer 'base', got '%s'", cfg.Layers[0])
	}
	if cfg.Layers[1] != "shell/zsh" {
		t.Fatalf("expected second layer 'shell/zsh', got '%s'", cfg.Layers[1])
	}
	if !cfg.Hooks.Enabled {
		t.Fatal("expected hooks enabled")
	}
}

func TestLoadDotfConfigWithYml(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `profile: desktop
layers:
  - base
`
	if err := os.WriteFile(filepath.Join(dir, "dotf.yml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() with .yml failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Profile != "desktop" {
		t.Fatalf("expected profile 'desktop', got '%s'", cfg.Profile)
	}
}

func TestLoadDotfConfigYamlPreference(t *testing.T) {
	dir := t.TempDir()
	// Both dotf.yaml and dotf.yml exist — dotf.yaml should be preferred
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte("profile: from_yaml\nlayers:\n  - base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dotf.yml"), []byte("profile: from_yml\nlayers:\n  - base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() failed: %v", err)
	}
	if cfg.Profile != "from_yaml" {
		t.Fatalf("expected profile 'from_yaml', got '%s'", cfg.Profile)
	}
}

func TestLoadDotfConfigInvalidYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte("invalid: [yaml: broken\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if cfg != nil {
		t.Fatal("expected nil config for invalid YAML")
	}
}

func TestLoadDotfConfigEmptyYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() on empty file should not error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for empty file")
	}
}

func TestLoadDotfConfigWithUnknownFields(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `profile: server
layers:
  - base
unknown_field: value
another_unknown:
  key: val
`
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() with unknown fields should not error: %v", err)
	}
	if cfg.Profile != "server" {
		t.Fatalf("expected profile 'server', got '%s'", cfg.Profile)
	}
}

func TestLoadDotfConfigMinimal(t *testing.T) {
	dir := t.TempDir()
	// Minimal valid YAML with just layers
	yamlContent := `layers:
  - base
`
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() failed: %v", err)
	}
	if cfg.Profile != "" {
		t.Fatalf("expected empty profile, got '%s'", cfg.Profile)
	}
	if len(cfg.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(cfg.Layers))
	}
}

func TestLoadDotfConfigWithComments(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `# DOTF configuration
profile: gaming  # gaming rig profile
layers:
  # Base layer always included
  - base
  - shell/zsh
`
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() with comments failed: %v", err)
	}
	if cfg.Profile != "gaming" {
		t.Fatalf("expected profile 'gaming', got '%s'", cfg.Profile)
	}
}

func TestLoadDotfConfigPathTraversal(t *testing.T) {
	dir := t.TempDir()
	// Path traversal should not be a problem since we just read files
	yamlContent := `layers:
  - ../../etc
`
	if err := os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDotfConfig(dir)
	if err != nil {
		t.Fatalf("LoadDotfConfig() should parse without error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Layers) != 1 || cfg.Layers[0] != "../../etc" {
		t.Fatalf("expected ['../../etc'], got %v", cfg.Layers)
	}
}
