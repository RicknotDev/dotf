package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}
	if m.backupDir != dir {
		t.Fatalf("expected %s, got %s", dir, m.backupDir)
	}
	if m.maxKeep != MaxBackupVersions {
		t.Fatalf("expected %d, got %d", MaxBackupVersions, m.maxKeep)
	}
}

func TestCreateBackup(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	// Create a file to back up
	origDir := filepath.Join(dir, "home")
	_ = os.MkdirAll(origDir, 0755)
	origFile := filepath.Join(origDir, ".zshrc")
	if err := os.WriteFile(origFile, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := m.Create(".zshrc", origFile)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if b == nil {
		t.Fatal("Create() returned nil backup")
	}
	if b.OriginalPath != ".zshrc" {
		t.Fatalf("expected .zshrc, got %s", b.OriginalPath)
	}
	if !strings.HasPrefix(b.Checksum, "sha256:") {
		t.Fatalf("expected sha256 checksum, got %s", b.Checksum)
	}

	// Verify backup file exists
	if _, err := os.Stat(b.BackupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
}

func TestCreateBackupNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	b, err := m.Create("nonexistent", filepath.Join(dir, "nonexistent"))
	if err != nil {
		t.Fatalf("Create() for nonexistent file failed: %v", err)
	}
	if b != nil {
		t.Fatal("expected nil backup for nonexistent file")
	}
}

func TestCreateBackupSymlink(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	// Create a symlink (should not be backed up)
	target := filepath.Join(dir, "target")
	_ = os.WriteFile(target, []byte("data"), 0644)
	link := filepath.Join(dir, "link")
	_ = os.Symlink(target, link)

	b, err := m.Create("link", link)
	if err != nil {
		t.Fatalf("Create() for symlink failed: %v", err)
	}
	if b != nil {
		t.Fatal("expected nil backup for symlink")
	}
}

func TestListBackups(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple backups of the same file
	origDir := filepath.Join(dir, "home")
	_ = os.MkdirAll(origDir, 0755)
	origFile := filepath.Join(origDir, ".zshrc")

	for i := 0; i < 3; i++ {
		content := []byte("export VERSION=" + string(rune('0'+i)))
		_ = os.WriteFile(origFile, content, 0644)
		_, _ = m.Create(".zshrc", origFile)
	}

	backups, err := m.List(".zshrc")
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("expected at least 1 backup")
	}
	// Should be sorted newest first
	for i := 1; i < len(backups); i++ {
		if backups[i-1].Created < backups[i].Created {
			t.Fatal("backups not sorted by date descending")
		}
	}
}

func TestListAllBackups(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	// Create backups for different files
	home := filepath.Join(dir, "home")
	_ = os.MkdirAll(home, 0755)

	for _, pair := range [][2]string{
		{".zshrc", "export A=1"},
		{".gitconfig", "[user]\nname=test"},
		{".config/nvim/init.lua", "vim.opt.number=true"},
	} {
		f := filepath.Join(home, pair[0])
		_ = os.MkdirAll(filepath.Dir(f), 0755)
		_ = os.WriteFile(f, []byte(pair[1]), 0644)
		_, _ = m.Create(pair[0], f)
	}

	all, err := m.ListAll()
	if err != nil {
		t.Fatalf("ListAll() failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 backup groups, got %d", len(all))
	}
}

func TestVerifyBackup(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	_ = os.MkdirAll(home, 0755)
	origFile := filepath.Join(home, "test.txt")
	_ = os.WriteFile(origFile, []byte("content"), 0644)

	b, err := m.Create("test.txt", origFile)
	if err != nil {
		t.Fatal(err)
	}

	valid, err := m.Verify(*b)
	if err != nil {
		t.Fatalf("Verify() failed: %v", err)
	}
	if !valid {
		t.Fatal("backup should be valid")
	}

	// Corrupt the backup
	_ = os.WriteFile(b.BackupPath, []byte("corrupted"), 0644)
	valid, err = m.Verify(*b)
	if err != nil {
		t.Fatalf("Verify() after corruption failed: %v", err)
	}
	if valid {
		t.Fatal("backup should be invalid after corruption")
	}
}

func TestRestoreBackup(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	_ = os.MkdirAll(home, 0755)
	origContent := []byte("original content")
	origFile := filepath.Join(home, "restore-test.txt")
	_ = os.WriteFile(origFile, origContent, 0644)

	b, err := m.Create("restore-test.txt", origFile)
	if err != nil {
		t.Fatal(err)
	}

	// Change the original
	_ = os.WriteFile(origFile, []byte("modified content"), 0644)

	// Restore
	targetDir := filepath.Join(dir, "restore-target")
	_ = os.MkdirAll(targetDir, 0755)
	targetFile := filepath.Join(targetDir, "restore-test.txt")

	if err := m.Restore(*b, targetFile); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(origContent) {
		t.Fatalf("expected %q, got %q", origContent, data)
	}
}

func TestBackupPruning(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	// Reduce max keep for testing
	m.maxKeep = 2

	home := filepath.Join(dir, "home")
	_ = os.MkdirAll(home, 0755)
	origFile := filepath.Join(home, "prune-test.txt")

	// Create 4 backups (only 2 should remain after pruning)
	for i := 0; i < 4; i++ {
		_ = os.WriteFile(origFile, []byte("content"), 0644)
		_, _ = m.Create("prune-test.txt", origFile)
	}

	backups, _ := m.List("prune-test.txt")
	if len(backups) > m.maxKeep {
		t.Fatalf("expected at most %d backups, got %d", m.maxKeep, len(backups))
	}
}

func TestBackupNameFormat(t *testing.T) {
	name := backupName("path/to/file.conf")
	if !strings.HasSuffix(name, ".bak") {
		t.Fatalf("expected .bak suffix, got %s", name)
	}
	if !strings.Contains(name, "path__to__file.conf") {
		t.Fatalf("expected path__to__file.conf in name, got %s", name)
	}
}

func TestVerifyMissingFile(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	b := Backup{
		OriginalPath: "test",
		BackupPath:   filepath.Join(dir, "nonexistent.bak"),
		Checksum:     "sha256:abc",
	}

	valid, err := m.Verify(b)
	if err == nil {
		t.Fatal("expected error for missing backup file")
	}
	if valid {
		t.Fatal("expected invalid for missing backup file")
	}
}
