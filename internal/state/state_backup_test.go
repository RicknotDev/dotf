package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordBackup(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a dummy backup file
	backupPath := filepath.Join(dir, "test.bak")
	if err := os.WriteFile(backupPath, []byte("backup content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Record the backup
	m.RecordBackup(backupPath, ".zshrc")

	// Get state and verify
	s := m.GetState()

	backup, exists := s.BackupManifest[backupPath]
	if !exists {
		t.Fatal("expected backup to be recorded in state")
	}
	if backup.Original != ".zshrc" {
		t.Fatalf("expected original '.zshrc', got '%s'", backup.Original)
	}
	if !strings.HasPrefix(backup.Checksum, "sha256:") {
		t.Fatalf("expected sha256 checksum, got '%s'", backup.Checksum)
	}
	if backup.Created == "" {
		t.Fatal("expected created timestamp to be set")
	}
}

func TestRecordBackupPersistsAfterSave(t *testing.T) {
	dir := t.TempDir()

	// Create and save
	m1, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(dir, "persist.bak")
	if err := os.WriteFile(backupPath, []byte("persist content"), 0644); err != nil {
		t.Fatal(err)
	}

	m1.RecordBackup(backupPath, ".gitconfig")
	if err := m1.Save(); err != nil {
		t.Fatal(err)
	}

	// Create new manager (loads state from disk)
	m2, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	s := m2.GetState()
	backup, exists := s.BackupManifest[backupPath]
	if !exists {
		t.Fatal("expected backup to persist after reload")
	}
	if backup.Original != ".gitconfig" {
		t.Fatalf("expected original '.gitconfig', got '%s'", backup.Original)
	}
}

func TestRecordBackupWithoutFile(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Record a backup for a file that doesn't exist on disk
	nonexistentPath := filepath.Join(dir, "nonexistent.bak")
	m.RecordBackup(nonexistentPath, ".missing")

	s := m.GetState()
	backup, exists := s.BackupManifest[nonexistentPath]
	if !exists {
		t.Fatal("expected backup to be recorded even without file on disk")
	}
	if backup.Checksum != "" {
		t.Fatalf("expected empty checksum for nonexistent file, got '%s'", backup.Checksum)
	}
}

func TestVerifyBackupIntegrityAfterRecord(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a backup file and record it
	backupPath := filepath.Join(dir, "backups", "test.bak")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("original backup content"), 0644); err != nil {
		t.Fatal(err)
	}

	m.RecordBackup(backupPath, ".zshrc")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify backup integrity
	corrupted, err := m.VerifyBackupIntegrity()
	if err != nil {
		t.Fatalf("VerifyBackupIntegrity() failed: %v", err)
	}
	if len(corrupted) != 0 {
		t.Fatalf("expected no corrupted backups, got: %v", corrupted)
	}
}

func TestVerifyBackupIntegrityDetectsCorruption(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a backup file and record it
	backupPath := filepath.Join(dir, "backups", "test.bak")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}

	m.RecordBackup(backupPath, ".zshrc")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Corrupt the backup file
	if err := os.WriteFile(backupPath, []byte("corrupted content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify should detect corruption
	corrupted, err := m.VerifyBackupIntegrity()
	if err != nil {
		t.Fatalf("VerifyBackupIntegrity() failed: %v", err)
	}
	if len(corrupted) == 0 {
		t.Fatal("expected corruption to be detected")
	}
	found := false
	for _, c := range corrupted {
		if strings.Contains(c, "checksum mismatch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'checksum mismatch' in corruption report, got: %v", corrupted)
	}
}

func TestVerifyBackupIntegrityMissingFile(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Record a backup for a file that doesn't exist
	nonexistentPath := filepath.Join(dir, "backups", "missing.bak")
	m.RecordBackup(nonexistentPath, ".gitconfig")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify should report missing file
	corrupted, err := m.VerifyBackupIntegrity()
	if err != nil {
		t.Fatalf("VerifyBackupIntegrity() failed: %v", err)
	}
	if len(corrupted) == 0 {
		t.Fatal("expected missing file to be reported")
	}
	found := false
	for _, c := range corrupted {
		if strings.Contains(c, "file missing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'file missing' in corruption report, got: %v", corrupted)
	}
}

func TestRecordBackupMultiple(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Record multiple backups
	backups := []struct {
		path     string
		original string
	}{
		{filepath.Join(dir, "zsh.bak"), ".zshrc"},
		{filepath.Join(dir, "git.bak"), ".gitconfig"},
		{filepath.Join(dir, "nvim.bak"), ".config/nvim/init.lua"},
	}

	for _, b := range backups {
		if err := os.WriteFile(b.path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		m.RecordBackup(b.path, b.original)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Reload
	m2, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	s := m2.GetState()
	if len(s.BackupManifest) != 3 {
		t.Fatalf("expected 3 backup entries, got %d", len(s.BackupManifest))
	}

	for _, b := range backups {
		if _, exists := s.BackupManifest[b.path]; !exists {
			t.Fatalf("missing backup entry for %s", b.path)
		}
	}
}
