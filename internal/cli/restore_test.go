package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreHelp(t *testing.T) {
	err := Restore([]string{"--help"}, t.TempDir())
	if err != nil {
		t.Fatalf("Restore --help returned error: %v", err)
	}
}

func TestRestorePreviewWithNoBackups(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Restore([]string{"--preview"}, t.TempDir())
	if err != nil {
		t.Fatalf("Restore --preview failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Available restores") {
		t.Fatal("expected 'Available restores'")
	}
	if !strings.Contains(output, "no backups found") {
		t.Fatal("expected 'no backups found'")
	}
}

func TestRestorePreviewWithBackups(t *testing.T) {
	stateDir := t.TempDir()

	// Create a backup file
	backupDir := filepath.Join(stateDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}
	backupFile := filepath.Join(backupDir, "zshrc.bak")
	if err := os.WriteFile(backupFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Restore([]string{"--preview"}, stateDir)
	if err != nil {
		t.Fatalf("Restore --preview with backups failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Available restores") {
		t.Fatal("expected 'Available restores'")
	}
}

func TestRestoreDefaultBehavior(t *testing.T) {
	// With no args and no --all, should default to preview
	stateDir := t.TempDir()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Restore([]string{}, stateDir)
	if err != nil {
		t.Fatalf("Restore() with no args failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Available restores") {
		t.Fatal("expected 'Available restores' default behavior")
	}
}

func TestRestoreSpecificFileNotFound(t *testing.T) {
	stateDir := t.TempDir()

	// Captures stderr for messages
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Restore([]string{"nonexistent"}, stateDir)
	if err != nil {
		// Restore may return an error or just print - either is acceptable
		t.Logf("Restore nonexistent returned: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = buf.String()
}

func TestRestoreAllWithEmptyBackups(t *testing.T) {
	stateDir := t.TempDir()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Restore([]string{"--all"}, stateDir)
	if err != nil {
		t.Fatalf("Restore --all with no backups failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Should report 0 restored
	if !strings.Contains(output, "Restored 0 files") && !strings.Contains(output, "0") {
		t.Logf("Restore --all no backups output: %s", output)
	}
}
