package safety

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTargetPathSymlinkToOtherLocation(t *testing.T) {
	home := t.TempDir()
	outsideTarget := filepath.Join(home, "..", "outside.txt")
	os.WriteFile(outsideTarget, []byte("data"), 0644)
	linkPath := filepath.Join(home, ".zshrc")
	os.Symlink(outsideTarget, linkPath)

	result := ValidateTargetPath(home, ".zshrc")
	if !result.Safe {
		t.Fatalf("expected safe for target within home, got: %s", result.Reason)
	}
}

func TestValidateTargetPathWithSpecialChars(t *testing.T) {
	home := t.TempDir()
	result := ValidateTargetPath(home, ".config/app with spaces/config.toml")
	if !result.Safe {
		t.Fatalf("expected safe for paths with spaces, got: %s", result.Reason)
	}
	if result.Normalized == "" {
		t.Fatal("expected normalized path")
	}
}

func TestValidateTargetPathUnicode(t *testing.T) {
	home := t.TempDir()
	result := ValidateTargetPath(home, ".config/应用程序/settings.toml")
	if !result.Safe {
		t.Fatalf("expected safe for unicode paths, got: %s", result.Reason)
	}
}

func TestValidateLayerFileNonExistent(t *testing.T) {
	dir := t.TempDir()
	result := ValidateLayerFile(dir, filepath.Join(dir, "nonexistent.txt"))
	if result.Safe {
		t.Fatal("expected unsafe for nonexistent file")
	}
}

func TestValidateLayerFileSymlinkWithinRepo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	os.WriteFile(target, []byte("data"), 0644)
	link := filepath.Join(dir, "link.txt")
	os.Symlink(target, link)

	result := ValidateLayerFile(dir, link)
	if !result.Safe {
		t.Fatalf("expected safe for symlink within repo, got: %s", result.Reason)
	}
}

func TestIsSensitivePathEdgeCases(t *testing.T) {
	tests := []struct {
		path      string
		sensitive bool
	}{
		{"/etc/passwd", true},
		{"/etc/PASSWD", false},
		{"/etc/ssh/sshd_config", false},
		{"/home/user/.ssh/authorized_keys", false},
		{"/root/.bashrc", true},
		{"/root", true},
		{"/root/subdir/file", true},
	}

	for _, tt := range tests {
		got := IsSensitivePath(tt.path)
		if got != tt.sensitive {
			t.Errorf("IsSensitivePath(%q) = %v, want %v", tt.path, got, tt.sensitive)
		}
	}
}
