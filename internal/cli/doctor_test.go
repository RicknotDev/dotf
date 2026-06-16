package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorHelp(t *testing.T) {
	err := Doctor([]string{"--help"}, t.TempDir())
	if err != nil {
		t.Fatalf("Doctor --help returned error: %v", err)
	}
}

func TestDoctorRunsChecks(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Doctor([]string{}, t.TempDir())
	if err != nil {
		t.Fatalf("Doctor() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "DOTF Doctor") {
		t.Fatal("expected 'DOTF Doctor' header")
	}

	if !strings.Contains(output, "Checking lock status") {
		t.Fatal("expected lock status check")
	}
	if !strings.Contains(output, "Checking state integrity") {
		t.Fatal("expected state integrity check")
	}
	if !strings.Contains(output, "Checking repository structure") {
		t.Fatal("expected repository structure check")
	}
}

func TestDoctorFixUnlock(t *testing.T) {
	stateDir := t.TempDir()

	// Create a stale lock file
	lockPath := filepath.Join(stateDir, "dotf.lock")
	if err := os.WriteFile(lockPath, []byte("99999\nstale\n2024-01-01T00:00:00Z\n"), 0644); err != nil {
		t.Fatal(err)
	}

	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Use --unlock to release the stale lock
	err := Doctor([]string{"--unlock"}, stateDir)
	if err != nil {
		t.Fatalf("Doctor --unlock failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Lock released") && !strings.Contains(output, "Stale lock released") {
		t.Fatalf("expected lock to be released, output: %s", output)
	}
}

func TestDoctorVerifyBackups(t *testing.T) {
	stateDir := t.TempDir()

	// Create a valid backup file with proper naming format (path__to__file.TIMESTAMP.bak)
	backupDir := filepath.Join(stateDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "zshrc.1700000000000000000.bak"), []byte("backup data"), 0644); err != nil {
		t.Fatal(err)
	}

	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Doctor([]string{"--verify-backups"}, stateDir)
	if err != nil {
		t.Fatalf("Doctor --verify-backups failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "backups verified") || !strings.Contains(output, "Verifying backup integrity") {
		t.Fatalf("expected backup verification output: %s", output)
	}
}

func TestDoctorListHooks(t *testing.T) {
	dir := setupTestRepo(t)

	// Add a hook file
	hookDir := filepath.Join(dir, "layers", "base", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hookDir, "post-install.sh"), []byte("echo hello\n"), 0755); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Doctor([]string{"--list-hooks"}, t.TempDir())
	if err != nil {
		t.Fatalf("Doctor --list-hooks failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Available hooks") {
		t.Fatal("expected hooks listing")
	}
	if !strings.Contains(output, "post-install") {
		t.Fatal("expected post-install hook to be listed")
	}
}

func TestDoctorEmptyStateDir(t *testing.T) {
	dir := setupTestRepo(t)
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Fresh state dir should not cause errors
	err := Doctor([]string{}, t.TempDir())
	if err != nil {
		t.Fatalf("Doctor() with fresh state dir failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "All checks passed") {
		t.Fatal("expected all checks to pass on fresh state")
	}
}
