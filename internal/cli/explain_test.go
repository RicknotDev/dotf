package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a minimal DOTF repository structure for testing.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create layers directory with a base layer
	baseDir := filepath.Join(dir, "layers", "base")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test config file in base
	if err := os.WriteFile(filepath.Join(baseDir, ".zshrc"), []byte("export ZSH=$HOME/.oh-my-zsh\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create nested file with parent directories
	alacrittyDir := filepath.Join(baseDir, ".config", "alacritty")
	if err := os.MkdirAll(alacrittyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alacrittyDir, "alacritty.toml"), []byte("[window]\nopacity = 0.95\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a distro layer
	distroDir := filepath.Join(dir, "layers", "distro", "arch")
	if err := os.MkdirAll(distroDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distroDir, ".zshrc"), []byte("export PKG=arch\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestExplainHelp(t *testing.T) {
	// --help should print usage and return nil
	err := Explain([]string{"--help"})
	if err != nil {
		t.Fatalf("Explain --help returned error: %v", err)
	}
}

func TestExplainInRepo(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Explain([]string{})
	if err != nil {
		t.Fatalf("Explain() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should contain key sections
	if !strings.Contains(output, "Detected:") {
		t.Fatal("expected 'Detected:' section in explain output")
	}
	if !strings.Contains(output, "Activated Layers:") {
		t.Fatal("expected 'Activated Layers:' section in explain output")
	}
	if !strings.Contains(output, "base") {
		t.Fatal("expected 'base' layer in explain output")
	}
	if !strings.Contains(output, "Overrides:") {
		t.Fatal("expected 'Overrides:' section in explain output")
	}
}

func TestExplainShowsOverrides(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Explain([]string{})
	if err != nil {
		t.Fatalf("Explain() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// The arch/distro layer overrides base for .zshrc
	if !strings.Contains(output, "overrides") {
		t.Fatal("expected override info in explain output when layers have overlapping files")
	}
}

func TestExplainWithoutLayers(t *testing.T) {
	dir := t.TempDir()
	// No layers directory
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Explain([]string{})
	if err != nil {
		t.Fatalf("Explain() in dir without layers failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Activated Layers:") {
		t.Fatal("expected 'Activated Layers:' section even without layers")
	}
}

func TestExplainParsesFlagsCorrectly(t *testing.T) {
	// Unknown flags should produce an error
	err := Explain([]string{"--unknown-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestExplainNoArgsIsOK(t *testing.T) {
	// No args should not error (runs explain in current dir)
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	err := Explain([]string{})
	if err != nil {
		t.Fatalf("Explain() with no args failed: %v", err)
	}
}
