package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectHelp(t *testing.T) {
	err := Inspect([]string{"--help"}, t.TempDir())
	if err != nil {
		t.Fatalf("Inspect --help returned error: %v", err)
	}
}

func TestInspectNoSubcommand(t *testing.T) {
	err := Inspect([]string{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for inspect without subcommand")
	}
	if !strings.Contains(err.Error(), "subcommand") {
		t.Fatalf("expected error about subcommand, got: %v", err)
	}
}

func TestInspectInvalidSubcommand(t *testing.T) {
	err := Inspect([]string{"nonexistent"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for invalid subcommand")
	}
}

func TestInspectLayer(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Inspect([]string{"layer", "base"}, t.TempDir())
	if err != nil {
		t.Fatalf("Inspect layer base failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Layer: base") {
		t.Fatal("expected 'Layer: base'")
	}
	if !strings.Contains(output, ".zshrc") {
		t.Fatal("expected .zshrc in layer listing")
	}
	if !strings.Contains(output, ".config/alacritty/alacritty.toml") {
		t.Fatal("expected alacritty config in layer listing")
	}
}

func TestInspectLayerNotFound(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	err := Inspect([]string{"layer", "nonexistent"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for nonexistent layer")
	}
}

func TestInspectState(t *testing.T) {
	stateDir := t.TempDir()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Inspect([]string{"state"}, stateDir)
	if err != nil {
		t.Fatalf("Inspect state failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "DOTF State") {
		t.Fatal("expected 'DOTF State' header")
	}
	if !strings.Contains(output, "Version:") {
		t.Fatal("expected Version field")
	}
}

func TestInspectOverrides(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Inspect([]string{"overrides"}, t.TempDir())
	if err != nil {
		t.Fatalf("Inspect overrides failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "File Overrides") {
		t.Fatal("expected 'File Overrides' section")
	}
}

func TestInspectFileNoArg(t *testing.T) {
	err := Inspect([]string{"file"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for inspect file without path")
	}
}

func TestInspectBackup(t *testing.T) {
	stateDir := t.TempDir()

	// Create a backup file to list
	backupDir := filepath.Join(stateDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "zshrc.bak"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Inspect([]string{"backup"}, stateDir)
	if err != nil {
		t.Fatalf("Inspect backup failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Backup Manifest") {
		t.Fatal("expected 'Backup Manifest'")
	}
}
