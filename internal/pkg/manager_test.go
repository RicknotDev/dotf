package pkg

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fakePath(t *testing.T, available ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, a := range available {
		set[a] = true
	}
	old := lookPath
	lookPath = func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = old })
}

func TestDetectPrefersAURHelper(t *testing.T) {
	fakePath(t, "pacman", "paru")
	if m := DetectManager(); m == nil || m.Name() != "paru" {
		t.Fatalf("expected paru, got %v", m)
	}
	fakePath(t, "pacman", "yay")
	if m := DetectManager(); m == nil || m.Name() != "yay" {
		t.Fatalf("expected yay, got %v", m)
	}
	fakePath(t)
	if m := DetectManager(); m != nil {
		t.Fatalf("expected none, got %s", m.Name())
	}
}

func TestCommandUsesSudoWhenNotRoot(t *testing.T) {
	old := geteuid
	t.Cleanup(func() { geteuid = old })
	fakePath(t, "apt-get")
	m := DetectManager()

	geteuid = func() int { return 1000 }
	if got := m.Command([]string{"git"}); !reflect.DeepEqual(got, []string{"sudo", "apt-get", "install", "-y", "git"}) {
		t.Fatalf("unexpected command %v", got)
	}
	geteuid = func() int { return 0 }
	if got := m.Command([]string{"git"}); got[0] != "apt-get" {
		t.Fatalf("root should not use sudo: %v", got)
	}

	fakePath(t, "paru")
	geteuid = func() int { return 1000 }
	if got := DetectManager().Command([]string{"x"}); got[0] != "paru" {
		t.Fatalf("paru must not be run through sudo: %v", got)
	}
}

func TestListFilesByManager(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "base")
	for _, f := range []string{"pacman.txt", "apt.txt", "aur.txt", "notes.md"} {
		p := filepath.Join(layer, "packages", f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x\n"), 0o644)
	}
	fakePath(t, "pacman")
	used, ignored := DetectManager().ListFiles([]string{layer})
	if len(used) != 1 || filepath.Base(used[0]) != "pacman.txt" {
		t.Fatalf("pacman should read only pacman.txt, got %v", used)
	}
	if len(ignored) != 3 {
		t.Fatalf("expected 3 ignored files, got %v", ignored)
	}

	fakePath(t, "yay")
	used, _ = DetectManager().ListFiles([]string{layer})
	if len(used) != 2 {
		t.Fatalf("yay should read pacman.txt and aur.txt, got %v", used)
	}
}

func TestLoadPackages(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nix.txt")
	os.WriteFile(p, []byte("# comment\nripgrep fd  # trailing\n\nnixpkgs#bat\n"), 0o644)
	pkgs, err := LoadPackages(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pkgs, []string{"ripgrep", "fd", "nixpkgs#bat"}) {
		t.Fatalf("got %v", pkgs)
	}
}

func TestRejectsOptionLikeNames(t *testing.T) {
	p := filepath.Join(t.TempDir(), "apt.txt")
	os.WriteFile(p, []byte("git\n--allow-unauthenticated\n"), 0o644)
	if _, err := LoadPackages(p); err == nil {
		t.Fatal("package name starting with - accepted")
	}
	fakePath(t, "apt-get")
	called := false
	oldRun := run
	run = func(string, ...string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { run = oldRun })
	if err := DetectManager().Install([]string{"-y"}); err == nil || called {
		t.Fatal("Install ran with an option-like package name")
	}
}
