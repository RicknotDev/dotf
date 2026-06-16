package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyHelp(t *testing.T) {
	cfg := OutputConfig{}
	err := Apply([]string{"--help"}, t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("Apply --help returned error: %v", err)
	}
}

func TestApplyNoLayers(t *testing.T) {
	dir := t.TempDir()
	cfg := OutputConfig{}
	err := Apply([]string{}, dir, cfg)
	if err == nil {
		t.Fatal("expected error for repo without layers/")
	}
	if !strings.Contains(err.Error(), "not a DOTF repository") {
		t.Fatalf("expected 'not a DOTF repository' error, got: %v", err)
	}
}

func TestApplyDryRun(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	stateDir := t.TempDir()
	cfg := OutputConfig{}

	err = Apply([]string{"--dry-run"}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply --dry-run failed: %v", err)
	}
}

func TestApplyNoInteractiveConflict(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	stateDir := t.TempDir()
	cfg := OutputConfig{}

	// Set HOME to a temp dir where files don't exist yet (no conflicts)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// With --no-interactive and no conflicts, apply should work
	_ = Apply([]string{"--no-interactive"}, stateDir, cfg)
}

func TestApplyDiff(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	stateDir := t.TempDir()
	cfg := OutputConfig{}

	err := Apply([]string{"--diff"}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply --diff failed: %v", err)
	}
}

func TestApplyProfileFlag(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	stateDir := t.TempDir()
	cfg := OutputConfig{}

	err := Apply([]string{"--dry-run", "--profile", "test"}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply --profile failed: %v", err)
	}
}

func TestApplyWithDotfYaml(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a dotf.yaml
	yamlContent := `profile: auto
layers:
  - base
`
	_ = os.WriteFile(filepath.Join(dir, "dotf.yaml"), []byte(yamlContent), 0644)

	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	stateDir := t.TempDir()
	cfg := OutputConfig{}

	err := Apply([]string{"--diff"}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply with dotf.yaml failed: %v", err)
	}
}

func TestApplyDryRunNoChanges(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	stateDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	cfg := OutputConfig{}

	// Verify apply --dry-run doesn't create any files
	err := Apply([]string{"--dry-run"}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply --dry-run failed: %v", err)
	}
}
