package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func mkfile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// .zshrc must not pick up (or prune) the backups of .zshrc.local.
func TestListExactMatchOnly(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(filepath.Join(dir, "backups"))
	m.maxKeep = 1
	home := filepath.Join(dir, "home")
	mkfile(t, filepath.Join(home, ".zshrc"), "main", 0o644)
	mkfile(t, filepath.Join(home, ".zshrc.local"), "local", 0o644)

	if _, err := m.Create(".zshrc.local", filepath.Join(home, ".zshrc.local")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(".zshrc", filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}

	main, _ := m.List(".zshrc")
	local, _ := m.List(".zshrc.local")
	if len(main) != 1 || len(local) != 1 {
		t.Fatalf("expected one backup each, got .zshrc=%d .zshrc.local=%d", len(main), len(local))
	}
	if main[0].OriginalPath != ".zshrc" || local[0].OriginalPath != ".zshrc.local" {
		t.Fatal("backups attributed to the wrong file")
	}
}

// A path containing "__" must round-trip and not collide with a/b.
func TestDoubleUnderscorePath(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(filepath.Join(dir, "backups"))
	home := filepath.Join(dir, "home")
	mkfile(t, filepath.Join(home, "a__b"), "underscore", 0o644)
	mkfile(t, filepath.Join(home, "a", "b"), "nested", 0o644)

	if _, err := m.Create("a__b", filepath.Join(home, "a__b")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("a/b", filepath.Join(home, "a", "b")); err != nil {
		t.Fatal(err)
	}
	all, _ := m.ListAll()
	if len(all["a__b"]) != 1 || len(all["a/b"]) != 1 {
		t.Fatalf("paths with __ are ambiguous: %v", all)
	}
}

func TestBackupAndRestoreKeepMode(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(filepath.Join(dir, "backups"))
	src := filepath.Join(dir, "home", ".ssh", "config")
	mkfile(t, src, "Host *", 0o600)

	b, err := m.Create(".ssh/config", src)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(b.BackupPath); info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode %o, want 600", info.Mode().Perm())
	}
	if info, _ := os.Stat(m.Dir()); info.Mode().Perm() != 0o700 {
		t.Fatalf("backup dir mode %o, want 700", info.Mode().Perm())
	}

	out := filepath.Join(dir, "restored")
	if err := m.Restore(*b, out); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(out); info.Mode().Perm() != 0o600 {
		t.Fatalf("restored mode %o, want 600", info.Mode().Perm())
	}
}

func TestChecksumRecordedAtCreation(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(filepath.Join(dir, "backups"))
	src := filepath.Join(dir, "f")
	mkfile(t, src, "content", 0o644)
	if _, err := m.Create("f", src); err != nil {
		t.Fatal(err)
	}
	listed, _ := m.List("f")
	if len(listed) != 1 || listed[0].Checksum == "" {
		t.Fatal("listed backup has no recorded checksum")
	}
	// Tamper with the backup; the listing must now fail verification.
	if err := os.WriteFile(listed[0].BackupPath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	listed, _ = m.List("f")
	if ok, _ := m.Verify(listed[0]); ok {
		t.Fatal("tampered backup passed verification")
	}
}

func TestLegacyBackupsAreListed(t *testing.T) {
	dir := t.TempDir()
	bdir := filepath.Join(dir, "backups")
	mkfile(t, filepath.Join(bdir, ".config__kitty__kitty.conf.1700000000000000000.bak"), "x", 0o644)
	m, _ := NewManager(bdir)
	b, err := m.List(".config/kitty/kitty.conf")
	if err != nil || len(b) != 1 {
		t.Fatalf("legacy backup not found: %v %v", b, err)
	}
	if ok, err := m.Verify(b[0]); err != nil || !ok {
		t.Fatal("legacy backup without checksum should be accepted")
	}
}

func TestBackupKeepEnv(t *testing.T) {
	t.Setenv("DOTF_BACKUP_KEEP", "2")
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if m.maxKeep != 2 {
		t.Fatalf("maxKeep = %d, want 2", m.maxKeep)
	}
	t.Setenv("DOTF_BACKUP_KEEP", "zero")
	if _, err := NewManager(t.TempDir()); err == nil {
		t.Fatal("invalid DOTF_BACKUP_KEEP accepted")
	}
}

func TestNewManagerDoesNotCreateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backups")
	if _, err := NewManager(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("NewManager must not write to disk")
	}
}
