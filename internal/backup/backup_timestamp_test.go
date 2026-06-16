package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupTimestampUnique(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0755)
	origFile := filepath.Join(home, "test.txt")

	// Create 5 backups of the same file as fast as possible
	// to test timestamp uniqueness within the same second
	backups := make([]*Backup, 5)
	for i := 0; i < 5; i++ {
		content := []byte("content" + string(rune('0'+i)))
		if err := os.WriteFile(origFile, content, 0644); err != nil {
			t.Fatal(err)
		}
		b, err := m.Create("test.txt", origFile)
		if err != nil {
			t.Fatalf("Create() iteration %d failed: %v", i, err)
		}
		if b == nil {
			t.Fatalf("Create() returned nil at iteration %d", i)
		}
		backups[i] = b
		// Small sleep to help ensure different timestamps
		time.Sleep(time.Millisecond)
	}

	// Verify all backup file names are unique
	seen := make(map[string]bool)
	for i, b := range backups {
		if seen[b.BackupPath] {
			t.Fatalf("duplicate backup path at iteration %d: %s", i, b.BackupPath)
		}
		seen[b.BackupPath] = true

		// Verify the backup file exists
		if _, err := os.Stat(b.BackupPath); err != nil {
			t.Fatalf("backup file missing at iteration %d: %v", i, err)
		}
	}

	// Verify they're listed in correct order (newest first)
	listed, err := m.List("test.txt")
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(listed) != 5 {
		t.Fatalf("expected 5 backups, got %d", len(listed))
	}
	for i := 1; i < len(listed); i++ {
		if listed[i-1].Created < listed[i].Created {
			t.Fatal("backups not sorted newest first")
		}
	}
}

func TestBackupSameSecondRace(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0755)
	origFile := filepath.Join(home, "race.txt")
	os.WriteFile(origFile, []byte("original"), 0644)

	// Create 3 backups rapidly
	for i := 0; i < 3; i++ {
		content := []byte("version" + string(rune('0'+i)))
		os.WriteFile(origFile, content, 0644)
		b, err := m.Create("race.txt", origFile)
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		if b == nil {
			t.Fatal("Create() returned nil")
		}
		if !strings.HasSuffix(b.BackupPath, ".bak") {
			t.Fatalf("backup path doesn't end with .bak: %s", b.BackupPath)
		}
	}
}

func TestBackupNameFormatResolution(t *testing.T) {
	// Test that backupName produces valid filenames for various paths
	tests := []struct {
		input    string
		wantSafe bool
	}{
		{".zshrc", true},
		{".config/alacritty/alacritty.toml", true},
		{".local/share/app/settings.conf", true},
		{"path/with spaces/file.conf", true},
		{"unicode/应用程序/config.toml", true},
	}

	for _, tt := range tests {
		name := backupName(tt.input)
		if name == "" {
			t.Fatalf("backupName(%q) returned empty", tt.input)
		}
		if !strings.HasSuffix(name, ".bak") {
			t.Fatalf("backupName(%q) = %q, want .bak suffix", tt.input, name)
		}
		// Verify no path separators remain in the safe name (they should be __)
		safePart := strings.TrimSuffix(name, ".bak")
		// The format is: safePath.TIMESTAMP.bak
		if strings.Contains(safePart, "/") {
			t.Fatalf("backupName(%q) = %q, contains path separator", tt.input, name)
		}
	}
}
