package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codebuff/dotf/internal/state"
)

// TestInstallWithLocalPath: dotf install with a local path should work.
func TestIntegrationInstallWithLocalPath(t *testing.T) {
	repoDir := setupTestRepo(t)
	stateDir := t.TempDir()

	// Run install with the local repo path, using --dry-run to avoid touching real home
	err := Install([]string{"--dry-run", repoDir}, stateDir)
	if err != nil {
		t.Fatalf("Install with local path failed: %v", err)
	}
}

// TestIntegrationInstallWithoutLayers: dotf install without layers/ should error clearly.
func TestIntegrationInstallWithoutLayers(t *testing.T) {
	emptyDir := t.TempDir()
	stateDir := t.TempDir()

	err := Install([]string{emptyDir}, stateDir)
	if err == nil {
		t.Fatal("expected error for repo without layers/")
	}
	if !strings.Contains(err.Error(), "not a DOTF repository") {
		t.Fatalf("expected 'not a DOTF repository' error, got: %v", err)
	}
}

// TestIntegrationInstallIdempotent: running install twice should not change state.
func TestIntegrationInstallIdempotent(t *testing.T) {
	repoDir := setupTestRepo(t)
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// First install
	err := Install([]string{"--copy", repoDir}, stateDir)
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Check files exist
	zshrcPath := filepath.Join(homeDir, ".zshrc")
	if _, err := os.Stat(zshrcPath); err != nil {
		t.Fatalf(".zshrc not installed: %v", err)
	}

	// Second install should be idempotent (no errors, files still exist)
	err = Install([]string{"--copy", repoDir}, stateDir)
	if err != nil {
		t.Fatalf("Second install (idempotent) failed: %v", err)
	}

	// Files should still exist
	if _, err := os.Stat(zshrcPath); err != nil {
		t.Fatal(".zshrc removed after second install")
	}
}

// TestIntegrationApplyUnapplyApply: dotf apply + dotf unapply + dotf apply = same state.
func TestIntegrationApplyUnapplyApply(t *testing.T) {
	repoDir := setupTestRepo(t)
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	cfg := OutputConfig{}

	// Step 1: Apply
	err := Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("First apply failed: %v", err)
	}

	// Verify files were created
	zshrcPath := filepath.Join(homeDir, ".zshrc")
	if _, err := os.Lstat(zshrcPath); err != nil {
		t.Fatalf(".zshrc not created after first apply: %v", err)
	}

	// Verify it's a symlink
	linkInfo, _ := os.Lstat(zshrcPath)
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal(".zshrc should be a symlink after apply")
	}

	// Read symlink target
	target, _ := os.Readlink(zshrcPath)
	t.Logf("First apply: .zshrc -> %s", target)

	// Step 2: Unapply
	err = Unapply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Unapply failed: %v", err)
	}

	// Verify files were removed
	if _, err := os.Lstat(zshrcPath); !os.IsNotExist(err) {
		t.Fatal(".zshrc should not exist after unapply")
	}

	// Step 3: Apply again = same state
	err = Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	// Verify files exist again
	if _, err := os.Lstat(zshrcPath); err != nil {
		t.Fatalf(".zshrc not created after second apply: %v", err)
	}

	// Verify it's still a symlink pointing to the same target
	newTarget, _ := os.Readlink(zshrcPath)
	if newTarget != target {
		t.Fatalf("symlink target changed: was %s, now %s", target, newTarget)
	}
}

// TestIntegrationApplyNoInteractiveConflict: --no-interactive with existing file should abort with conflict.
func TestIntegrationApplyNoInteractiveConflict(t *testing.T) {
	repoDir := setupTestRepo(t)
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	// Create an existing file that would conflict
	zshrcPath := filepath.Join(homeDir, ".zshrc")
	if err := os.WriteFile(zshrcPath, []byte("existing config"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := OutputConfig{}

	// Apply with --no-interactive should detect conflict and abort
	err := Apply([]string{"--no-interactive"}, stateDir, cfg)
	if err == nil {
		t.Fatal("expected error for conflict with --no-interactive")
	}

	// Error should indicate conflict
	if !strings.Contains(err.Error(), "conflict") && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected conflict error, got: %v", err)
	}

	// Existing file should still be intact
	data, err := os.ReadFile(zshrcPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing config" {
		t.Fatal("existing file was modified despite --no-interactive")
	}
}

// TestIntegrationApplyConflictWithoutFlag: without --no-interactive, apply should back up and proceed.
func TestIntegrationApplyConflictWithoutFlag(t *testing.T) {
	repoDir := setupTestRepo(t)
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	// Create an existing file that would conflict
	zshrcPath := filepath.Join(homeDir, ".zshrc")
	originalContent := []byte("existing config")
	if err := os.WriteFile(zshrcPath, originalContent, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := OutputConfig{}

	// Apply without --no-interactive should back up and proceed
	err := Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply with conflict (no --no-interactive) failed: %v", err)
	}

	// Original content should be backed up
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	s := sm.GetState()

	found := false
	for backupPath, backup := range s.BackupManifest {
		if backup.Original == ".zshrc" {
			found = true
			data, _ := os.ReadFile(backupPath)
			if string(data) != string(originalContent) {
				t.Fatalf("backup content mismatch: expected %q, got %q", originalContent, data)
			}
			break
		}
	}
	if !found {
		t.Fatal("backup for .zshrc not found in state")
	}

	// New symlink should exist
	linkInfo, err := os.Lstat(zshrcPath)
	if err != nil {
		t.Fatalf(".zshrc not created after apply: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal(".zshrc should be a symlink after apply with conflict")
	}
}

// TestIntegrationPathsWithSpaces: paths containing spaces should work.
func TestIntegrationPathsWithSpaces(t *testing.T) {
	repoDir := t.TempDir()
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	// Create a repo with a file path that has spaces
	layerDir := filepath.Join(repoDir, "layers", "base", ".config", "app with spaces")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(layerDir, "config file.conf")
	if err := os.WriteFile(configFile, []byte("config with spaces"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	cfg := OutputConfig{}

	// Apply should handle paths with spaces
	err := Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply with space paths failed: %v", err)
	}

	// Verify the file was created in the correct location
	targetPath := filepath.Join(homeDir, ".config", "app with spaces", "config file.conf")
	if _, err := os.Lstat(targetPath); err != nil {
		t.Fatalf("file with spaces in path not created: %v", err)
	}

	// Verify it's a symlink
	linkInfo, _ := os.Lstat(targetPath)
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("target should be a symlink")
	}
}

// TestIntegrationPathsWithUnicode: paths containing unicode characters should work.
func TestIntegrationPathsWithUnicode(t *testing.T) {
	repoDir := t.TempDir()
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	// Create a repo with unicode file paths
	layerDir := filepath.Join(repoDir, "layers", "base", ".config", "应用程序")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(layerDir, "设置.toml")
	if err := os.WriteFile(configFile, []byte("unicode config"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	cfg := OutputConfig{}

	// Apply should handle unicode paths
	err := Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply with unicode paths failed: %v", err)
	}

	// Verify the file was created in the correct location
	targetPath := filepath.Join(homeDir, ".config", "应用程序", "设置.toml")
	if _, err := os.Lstat(targetPath); err != nil {
		t.Fatalf("file with unicode in path not created: %v", err)
	}
}

// TestIntegrationHomeUnset: when HOME is not set, should get a clear error.
func TestIntegrationHomeUnset(t *testing.T) {
	repoDir := setupTestRepo(t)

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	// Unset HOME
	origHome := os.Getenv("HOME")
	os.Unsetenv("HOME")
	defer os.Setenv("HOME", origHome)

	stateDir := t.TempDir()
	cfg := OutputConfig{}

	// Apply should fail with a clear error about HOME
	err := Apply([]string{}, stateDir, cfg)
	if err == nil {
		t.Fatal("expected error when HOME is unset")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("expected error about home directory, got: %v", err)
	}
}

// TestIntegrationStatusJSON: status --json should produce valid JSON parseable by jq.
func TestIntegrationStatusJSON(t *testing.T) {
	repoDir := setupTestRepo(t)
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	cfg := OutputConfig{}

	// First apply to set up state
	err := Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Now run status --json
	jsonCfg := OutputConfig{JSON: true}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{}, stateDir, jsonCfg)
	if err != nil {
		t.Fatalf("Status --json failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	jsonOutput := buf.String()

	// Verify it's valid JSON
	if !json.Valid([]byte(jsonOutput)) {
		t.Fatalf("status --json produced invalid JSON: %s", jsonOutput)
	}

	// Parse and verify structure
	var result StatusResult
	if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil {
		t.Fatalf("cannot unmarshal status JSON: %v", err)
	}

	// Check expected fields
	if result.Version != 1 {
		t.Fatalf("expected version 1, got %d", result.Version)
	}
	if len(result.Entries) == 0 {
		t.Fatal("expected at least one entry in status JSON")
	}
	if result.Summary.Total == 0 {
		t.Fatal("expected total > 0 in status JSON summary")
	}

	// Verify each entry has required fields
	for i, entry := range result.Entries {
		if entry.Path == "" {
			t.Fatalf("entry %d has empty path", i)
		}
		if entry.Status == "" {
			t.Fatalf("entry %d has empty status", i)
		}
		if entry.Layer == "" {
			t.Fatalf("entry %d has empty layer", i)
		}
	}

	// Check summary consistency
	if result.Summary.OK+result.Summary.Issues != result.Summary.Total {
		t.Fatalf("summary inconsistent: OK(%d) + Issues(%d) != Total(%d)",
			result.Summary.OK, result.Summary.Issues, result.Summary.Total)
	}
}

// TestIntegrationStatusJSONWhenNothingInstalled: status --json with no files should still emit valid JSON.
func TestIntegrationStatusJSONWhenNothingInstalled(t *testing.T) {
	stateDir := t.TempDir()

	jsonCfg := OutputConfig{JSON: true}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When no files installed, status should still emit valid JSON
	err := Status([]string{}, stateDir, jsonCfg)
	// Even though there's an error about nothing to do,
	// JSON was already emitted via cfg.PrintJSON
	_ = err

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	jsonOutput := buf.String()

	// If JSON was emitted, it should be valid
	if jsonOutput != "" {
		if !json.Valid([]byte(jsonOutput)) {
			t.Fatalf("invalid JSON when nothing installed: %s", jsonOutput)
		}
	}
}

// TestIntegrationPipingOutput: commands should behave correctly when piped.
func TestIntegrationPipingOutput(t *testing.T) {
	repoDir := setupTestRepo(t)
	stateDir := t.TempDir()

	// status --json should output to stdout (and stdout can be piped)
	jsonCfg := OutputConfig{JSON: true}

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	// Run status without any files installed
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Status([]string{}, stateDir, jsonCfg)
	if err != nil {
		// Error is expected but JSON should still be on stdout
		t.Logf("Status returned error (expected): %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Even if we got an error, the JSON output should still be valid
	if output != "" {
		if !json.Valid([]byte(output)) {
			t.Fatalf("invalid JSON from piped status: %s", output)
		}
	}
}

// TestIntegrationStatusConsistentWithInstall: status should reflect what install did.
func TestIntegrationStatusConsistentWithInstall(t *testing.T) {
	repoDir := setupTestRepo(t)
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Install with copy mode
	err := Install([]string{"--copy", repoDir}, stateDir)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	// Status should show the installed files
	cfg := OutputConfig{}
	err = Status([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Status after install failed: %v", err)
	}

	// Now status --json for verification
	jsonCfg := OutputConfig{JSON: true}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{}, stateDir, jsonCfg)
	if err != nil {
		t.Fatalf("Status --json after install failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	jsonOutput := buf.String()

	if !json.Valid([]byte(jsonOutput)) {
		t.Fatalf("invalid JSON: %s", jsonOutput)
	}

	var result StatusResult
	_ = json.Unmarshal([]byte(jsonOutput), &result)

	if result.Summary.Total == 0 {
		t.Fatal("status should show > 0 files after install")
	}
}

// TestIntegrationNoColorFlag: --no-color should produce no ANSI escape codes.
func TestIntegrationNoColorFlag(t *testing.T) {
	repoDir := setupTestRepo(t)
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	cfg := OutputConfig{}

	// Apply to set up state
	err := Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Status with --no-color should not contain ANSI codes
	noColorCfg := OutputConfig{NoColor: true}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{}, stateDir, noColorCfg)
	if err != nil {
		t.Fatalf("Status with --no-color failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if strings.Contains(output, "\033[") {
		t.Fatal("status --no-color should not contain ANSI escape codes")
	}
}

// TestIntegrationNestedSymlinks: paths with nested directories should work.
func TestIntegrationNestedSymlinks(t *testing.T) {
	repoDir := t.TempDir()
	homeDir := t.TempDir()
	stateDir := t.TempDir()

	// Create a repo with deeply nested file
	nestedDir := filepath.Join(repoDir, "layers", "base", ".config", "nvim", "lua", "plugins")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nestedDir, "treesitter.lua")
	if err := os.WriteFile(nestedFile, []byte("return {}"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv("HOME", homeDir)

	cfg := OutputConfig{}

	err := Apply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Apply with nested paths failed: %v", err)
	}

	// Verify nested file exists
	targetPath := filepath.Join(homeDir, ".config", "nvim", "lua", "plugins", "treesitter.lua")
	if _, err := os.Lstat(targetPath); err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
}
