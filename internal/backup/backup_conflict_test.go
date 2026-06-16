package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConflictRegularFile(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0755)
	target := filepath.Join(home, ".zshrc")

	// Create a regular file (conflict)
	os.WriteFile(target, []byte("Existing content"), 0644)

	// Backup should succeed
	b, err := m.Create(".zshrc", target)
	if err != nil {
		t.Fatalf("Create() failed for regular file conflict: %v", err)
	}
	if b == nil {
		t.Fatal("expected backup for regular file conflict")
	}

	// Backup file should exist
	if _, err := os.Stat(b.BackupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}

	// Original should still exist
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Existing content" {
		t.Fatalf("expected original content preserved, got %q", data)
	}
}

func TestConflictBrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	// Create a broken symlink
	brokenTarget := filepath.Join(dir, "nonexistent.txt")
	linkPath := filepath.Join(dir, "broken.lnk")
	if err := os.Symlink(brokenTarget, linkPath); err != nil {
		t.Fatal(err)
	}

	// Backup a broken symlink - should detect it IS a symlink and skip
	// The Lstat check in Create should catch this
	b, err := m.Create("broken.lnk", linkPath)
	if err != nil {
		t.Fatalf("Create() failed for broken symlink: %v", err)
	}
	if b != nil {
		t.Fatal("expected nil backup for broken symlink")
	}
}

func TestConflictSymlinkToOtherLocation(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0755)

	// Create a real file somewhere else
	otherDir := filepath.Join(dir, "other")
	os.MkdirAll(otherDir, 0755)
	realFile := filepath.Join(otherDir, "real_config")
	os.WriteFile(realFile, []byte("other config"), 0644)

	// Create a symlink in "home" pointing to that other file
	linkPath := filepath.Join(home, ".gitconfig")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Fatal(err)
	}

	// Backup a symlink to another valid file - should detect it's a symlink and skip
	b, err := m.Create(".gitconfig", linkPath)
	if err != nil {
		t.Fatalf("Create() failed for symlink to other location: %v", err)
	}
	if b != nil {
		t.Fatal("expected nil backup for symlink to other location")
	}
}

func TestSymlinkParentDirCreation(t *testing.T) {
	// Direct test of the backup mechanism: create a backup for a nested path
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0755)

	// Create nested file
	nestedDir := filepath.Join(home, ".config", "nvim")
	os.MkdirAll(nestedDir, 0755)
	nestedFile := filepath.Join(nestedDir, "init.lua")
	os.WriteFile(nestedFile, []byte("vim.opt.number=true"), 0644)

	// Backup should handle nested paths
	b, err := m.Create(".config/nvim/init.lua", nestedFile)
	if err != nil {
		t.Fatalf("Create() failed for nested path: %v", err)
	}
	if b == nil {
		t.Fatal("expected backup for nested path")
	}

	// Verify backup file exists
	if _, err := os.Stat(b.BackupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
}

func TestConflictFileWithSpaces(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0755)
	target := filepath.Join(home, "my config file.conf")

	// Create file with spaces in name
	os.WriteFile(target, []byte("config with spaces"), 0644)

	b, err := m.Create("my config file.conf", target)
	if err != nil {
		t.Fatalf("Create() failed for file with spaces: %v", err)
	}
	if b == nil {
		t.Fatal("expected backup for file with spaces")
	}

	// Backup file should exist
	if _, err := os.Stat(b.BackupPath); err != nil {
		t.Fatalf("backup file not created for spaces path: %v", err)
	}
}

func TestConflictFileUnicode(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0755)
	target := filepath.Join(home, "应用程序.toml")

	// Create file with unicode in name
	os.WriteFile(target, []byte("unicode config"), 0644)

	b, err := m.Create("应用程序.toml", target)
	if err != nil {
		t.Fatalf("Create() failed for unicode file: %v", err)
	}
	if b == nil {
		t.Fatal("expected backup for unicode file")
	}

	// Backup file should exist
	if _, err := os.Stat(b.BackupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
}
