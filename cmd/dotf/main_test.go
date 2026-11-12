package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestWelcomeBannerIsAligned(t *testing.T) {
	var out bytes.Buffer
	printWelcome(&out)
	lines := strings.Split(out.String(), "\n")
	for _, l := range lines[:4] {
		if n := len([]rune(l)); n != 46 {
			t.Fatalf("banner line %q is %d columns, want 46", l, n)
		}
	}
}

func TestVersionAndHelpGoToStdout(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--help"}, {}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("%v: stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
	}
}

func TestExitCodes(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := run([]string{"nonsense"}, &stdout, &stderr); code != 64 {
		t.Fatalf("unknown command: exit %d, want 64", code)
	}
	stderr.Reset()
	// Nothing installed: exit 3, and the error is printed even with --quiet.
	if code := run([]string{"unapply", "--quiet"}, &stdout, &stderr); code != 3 {
		t.Fatalf("unapply with nothing installed: exit %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "nothing to do") {
		t.Fatalf("error hidden by --quiet: %q", stderr.String())
	}
	stderr.Reset()
	// Global flags may come before the command; errors are JSON on stderr.
	if code := run([]string{"--json", "unapply"}, &stdout, &stderr); code != 3 {
		t.Fatalf("exit %d, want 3", code)
	}
	if !strings.HasPrefix(strings.TrimSpace(stderr.String()), "{") {
		t.Fatalf("expected JSON error, got %q", stderr.String())
	}
}
