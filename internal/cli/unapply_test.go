package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codebuff/dotf/internal/state"
)

func TestUnapplyHelp(t *testing.T) {
	cfg := OutputConfig{}
	err := Unapply([]string{"--help"}, t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("Unapply --help returned error: %v", err)
	}
}

func TestUnapplyNoFiles(t *testing.T) {
	stateDir := t.TempDir()
	cfg := OutputConfig{}
	err := Unapply([]string{}, stateDir, cfg)
	if err == nil {
		t.Fatal("expected error for no installed files")
	}
	if !strings.Contains(err.Error(), "nothing to do") {
		t.Fatalf("expected 'nothing to do' error, got: %v", err)
	}
}

func TestUnapplyDryRun(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create a dummy installed file
	symlinkPath := filepath.Join(homeDir, ".zshrc")
	_ = os.WriteFile(filepath.Join(homeDir, "target.txt"), []byte("data"), 0644)
	_ = os.Symlink(filepath.Join(homeDir, "target.txt"), symlinkPath)
	sm.RecordFile(".zshrc", "base", "symlink", filepath.Join(homeDir, "target.txt"))
	sm.RecordInstall([]string{"base"})
	_ = sm.Save()

	cfg := OutputConfig{}
	err = Unapply([]string{"--dry-run"}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Unapply --dry-run failed: %v", err)
	}

	// Verify file still exists after dry run
	if _, err := os.Stat(symlinkPath); err != nil {
		t.Fatal("dry run should not remove files")
	}
}

func TestUnapplyWithInstalledFiles(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create a symlink as if it was installed
	symlinkPath := filepath.Join(homeDir, ".zshrc")
	sourcePath := filepath.Join(homeDir, "source.txt")
	_ = os.WriteFile(sourcePath, []byte("data"), 0644)
	_ = os.Symlink(sourcePath, symlinkPath)
	sm.RecordFile(".zshrc", "base", "symlink", sourcePath)
	sm.RecordInstall([]string{"base"})
	_ = sm.Save()

	cfg := OutputConfig{}
	err = Unapply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Unapply() failed: %v", err)
	}

	// Verify symlink was removed
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Fatal("unapply should remove the symlink")
	}
}

func TestUnapplyJSON(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	symlinkPath := filepath.Join(homeDir, ".zshrc")
	sourcePath := filepath.Join(homeDir, "source.txt")
	_ = os.WriteFile(sourcePath, []byte("data"), 0644)
	_ = os.Symlink(sourcePath, symlinkPath)
	sm.RecordFile(".zshrc", "base", "symlink", sourcePath)
	_ = sm.Save()

	cfg := OutputConfig{JSON: true}
	err = Unapply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Unapply() with JSON failed: %v", err)
	}
}

func TestUnapplyIdempotent(t *testing.T) {
	// Running unapply twice should be OK (second time = nothing to do)
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	symlinkPath := filepath.Join(homeDir, ".zshrc")
	sourcePath := filepath.Join(homeDir, "source.txt")
	_ = os.WriteFile(sourcePath, []byte("data"), 0644)
	_ = os.Symlink(sourcePath, symlinkPath)
	sm.RecordFile(".zshrc", "base", "symlink", sourcePath)
	_ = sm.Save()

	cfg := OutputConfig{}
	err = Unapply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("First Unapply() failed: %v", err)
	}

	// Second unapply should say nothing to do
	err = Unapply([]string{}, stateDir, cfg)
	if err == nil {
		t.Fatal("expected 'nothing to do' error for second unapply")
	}
}

func TestUnapplyNonExistentFile(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Record a file but don't create it on disk
	sm.RecordFile("missing.txt", "base", "symlink", filepath.Join(homeDir, "source.txt"))
	_ = sm.Save()

	cfg := OutputConfig{}
	err = Unapply([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Unapply() with missing file should not error: %v", err)
	}
}
