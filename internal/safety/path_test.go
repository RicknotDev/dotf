package safety

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLayerFileValid(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ValidateLayerFile(dir, file)
	if !result.Safe {
		t.Fatalf("expected safe, got: %s", result.Reason)
	}
	if result.Normalized == "" {
		t.Fatal("expected normalized path")
	}
}

func TestValidateLayerFilePathEscape(t *testing.T) {
	dir := t.TempDir()
	// Try to access a file outside the repo root
	escapePath := filepath.Join(dir, "..", "..", "etc", "passwd")

	result := ValidateLayerFile(dir, escapePath)
	if result.Safe {
		t.Fatal("expected unsafe for path escape")
	}
}

func TestValidateLayerFileNonexistent(t *testing.T) {
	dir := t.TempDir()
	result := ValidateLayerFile(dir, filepath.Join(dir, "nonexistent.txt"))
	if result.Safe {
		t.Fatal("expected unsafe for nonexistent file")
	}
}

func TestValidateLayerFileSymlinkLoop(t *testing.T) {
	dir := t.TempDir()
	link1 := filepath.Join(dir, "link1")
	link2 := filepath.Join(dir, "link2")

	// Create a symlink loop
	if err := os.Symlink(link2, link1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(link1, link2); err != nil {
		t.Fatal(err)
	}

	result := ValidateLayerFile(dir, link1)
	if result.Safe {
		t.Fatal("expected unsafe for symlink loop")
	}
}

func TestValidateLayerFileSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "..", "outside.txt")
	// Create the outside file
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink inside the repo that points outside
	link := filepath.Join(dir, "escape_link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	result := ValidateLayerFile(dir, link)
	if result.Safe {
		t.Fatal("expected unsafe for symlink escaping repo")
	}
}

func TestValidateTargetPathValid(t *testing.T) {
	home := "/home/user"
	result := ValidateTargetPath(home, ".config/kitty/kitty.conf")
	if !result.Safe {
		t.Fatalf("expected safe, got: %s", result.Reason)
	}
	if result.Normalized != filepath.Join("/home/user", ".config/kitty/kitty.conf") {
		t.Fatalf("unexpected normalized path: %s", result.Normalized)
	}
}

func TestValidateTargetPathEscape(t *testing.T) {
	home := "/home/user"
	result := ValidateTargetPath(home, "../../etc/passwd")
	if result.Safe {
		t.Fatal("expected unsafe for path escape")
	}
}

func TestValidateTargetPathSensitive(t *testing.T) {
	home := "/home/user"
	result := ValidateTargetPath(home, "../root/.ssh/authorized_keys")
	if result.Safe {
		t.Fatal("expected unsafe for sensitive path")
	}
}

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path      string
		sensitive bool
	}{
		{"/etc/passwd", true},
		{"/etc/shadow", true},
		{"/etc/ssh/sshd_config", false},
		{"/home/user/.config/kitty/kitty.conf", false},
		{"/root", true},
		{"/root/.bashrc", true},
		{"/etc/sudoers.d/admin", true},
		{"/usr/local/bin/dotf", false},
	}

	for _, tt := range tests {
		got := IsSensitivePath(tt.path)
		if got != tt.sensitive {
			t.Errorf("IsSensitivePath(%q) = %v, want %v", tt.path, got, tt.sensitive)
		}
	}
}

func TestMaxFilePath(t *testing.T) {
	home := "/"
	longPath := string(make([]byte, MaxFilePath+1))
	result := ValidateTargetPath(home, longPath)
	if result.Safe {
		t.Fatal("expected unsafe for overlong path")
	}
}
