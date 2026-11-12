package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RicknotDev/dotf/internal/detect"
	"github.com/RicknotDev/dotf/internal/layer"
)

func TestInstallHelp(t *testing.T) {
	err := Install([]string{"--help"}, t.TempDir(), OutputConfig{})
	if err != nil {
		t.Fatalf("Install --help returned error: %v", err)
	}
}

func TestInstallDryRun(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Install([]string{"--dry-run"}, t.TempDir(), OutputConfig{})
	if err != nil {
		t.Fatalf("Install --dry-run failed: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Detected:") {
		t.Fatal("expected 'Detected:' section")
	}
	if !strings.Contains(output, "Active layers") {
		t.Fatal("expected 'Active layers' section")
	}
	if !strings.Contains(output, "Dry run complete") {
		t.Fatal("expected 'Dry run complete' message")
	}
}

func TestInstallWithoutLayers(t *testing.T) {
	dir := t.TempDir()
	// No layers/ directory
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	err := Install([]string{}, t.TempDir(), OutputConfig{})
	if err == nil {
		t.Fatal("expected error for repo without layers/")
	}
	if !strings.Contains(err.Error(), "not a DOTF repository") {
		t.Fatalf("expected 'not a DOTF repository' error, got: %v", err)
	}
}

func TestInstallWithLocalPath(t *testing.T) {
	dir := setupTestRepo(t)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Install([]string{"--dry-run", dir}, t.TempDir(), OutputConfig{})
	if err != nil {
		t.Fatalf("Install --dry-run with path failed: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Detected:") {
		t.Fatal("expected detection output")
	}
}

func TestInstallDryRunShowsCopyMode(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Install([]string{"--dry-run", "--copy"}, t.TempDir(), OutputConfig{})
	if err != nil {
		t.Fatalf("Install --dry-run --copy failed: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Dry run complete") {
		t.Fatal("expected dry run to complete")
	}
}

func TestInstallDryRunShowsLayers(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Install([]string{"--dry-run"}, t.TempDir(), OutputConfig{})
	if err != nil {
		t.Fatalf("Install --dry-run failed: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Should show base layer
	if !strings.Contains(output, "base") {
		t.Fatal("expected base layer to be listed")
	}
}

func TestNonEmptyHelper(t *testing.T) {
	tests := []struct {
		input    string
		fallback string
		expected string
	}{
		{"hello", "-", "hello"},
		{"", "-", "-"},
		{"", "default", "default"},
		{"value", "", "value"},
	}

	for _, tt := range tests {
		got := nonEmpty(tt.input, tt.fallback)
		if got != tt.expected {
			t.Errorf("nonEmpty(%q, %q) = %q, want %q", tt.input, tt.fallback, got, tt.expected)
		}
	}
}

func TestIsSymlinkHelper(t *testing.T) {
	dir := t.TempDir()

	// Regular file
	regFile := filepath.Join(dir, "regular.txt")
	_ = os.WriteFile(regFile, []byte("data"), 0644)
	if isSymlink(regFile) {
		t.Fatal("regular file should not be a symlink")
	}

	// Symlink
	linkFile := filepath.Join(dir, "link.txt")
	_ = os.Symlink(regFile, linkFile)
	if !isSymlink(linkFile) {
		t.Fatal("symlink should be detected as symlink")
	}

	// Nonexistent
	if isSymlink(filepath.Join(dir, "nonexistent")) {
		t.Fatal("nonexistent file should not be a symlink")
	}
}

func TestGetLayerPaths(t *testing.T) {
	dir := setupTestRepo(t)
	layerResolved, err := layer.Resolve(dir, detect.Detect())
	if err != nil {
		t.Fatal(err)
	}
	paths := getLayerPaths(layerResolved.Layers)
	if len(paths) == 0 {
		t.Fatal("expected at least one layer path")
	}
	for _, p := range paths {
		if p == "" {
			t.Fatal("layer path should not be empty")
		}
	}
}
