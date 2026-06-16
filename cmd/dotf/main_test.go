package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestXdgStateDirDefault(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	dir := xdgStateDir()

	// Should fall back to $HOME/.local/state/dotf
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(home, ".local", "state", "dotf")
	if dir != expected {
		t.Fatalf("expected '%s', got '%s'", expected, dir)
	}
}

func TestXdgStateDirCustom(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")

	dir := xdgStateDir()

	expected := filepath.Join("/custom/state", "dotf")
	if dir != expected {
		t.Fatalf("expected '%s', got '%s'", expected, dir)
	}
}

func TestXdgStateDirWithTrailingSlash(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state/")

	dir := xdgStateDir()

	// filepath.Join cleans trailing slashes
	expected := filepath.Join("/custom/state", "dotf")
	if dir != expected {
		t.Fatalf("expected '%s', got '%s'", expected, dir)
	}
}

func TestXdgStateDirTmpFallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")

	dir := xdgStateDir()

	expected := filepath.Join("/tmp", "dotf-state")
	if dir != expected {
		t.Fatalf("expected '%s', got '%s'", expected, dir)
	}
}

func TestXdgStateDirIsDirectory(t *testing.T) {
	// Verify the returned path is a valid directory path we could create
	os.Unsetenv("XDG_STATE_HOME")

	dir := xdgStateDir()

	// The path should not be empty
	if dir == "" {
		t.Fatal("expected non-empty path")
	}

	// The path should be absolute
	if !filepath.IsAbs(dir) {
		t.Fatalf("expected absolute path, got '%s'", dir)
	}
}
