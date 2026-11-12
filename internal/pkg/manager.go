// Package pkg installs package lists with the system package manager.
//
// Each layer may contain packages/<manager>.txt files, one package per line
// (# starts a comment). Only the files meant for the detected manager are
// used: pacman.txt for pacman, paru and yay (plus aur.txt for paru and yay),
// apt.txt, dnf.txt, zypper.txt and nix.txt. Other files are ignored.
package pkg

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager describes a package manager.
type Manager struct {
	name     string
	bin      string
	args     []string // arguments before the package names
	needRoot bool     // run through sudo when not root
	lists    []string // package list base names this manager reads
}

var managers = []Manager{
	// AUR helpers first: they handle repo packages too and call sudo themselves.
	{name: "paru", bin: "paru", args: []string{"-S", "--noconfirm", "--needed"}, lists: []string{"pacman", "aur"}},
	{name: "yay", bin: "yay", args: []string{"-S", "--noconfirm", "--needed"}, lists: []string{"pacman", "aur"}},
	{name: "pacman", bin: "pacman", args: []string{"-S", "--noconfirm", "--needed"}, needRoot: true, lists: []string{"pacman"}},
	{name: "apt", bin: "apt-get", args: []string{"install", "-y"}, needRoot: true, lists: []string{"apt"}},
	{name: "dnf", bin: "dnf", args: []string{"install", "-y"}, needRoot: true, lists: []string{"dnf"}},
	{name: "zypper", bin: "zypper", args: []string{"--non-interactive", "install"}, needRoot: true, lists: []string{"zypper"}},
	{name: "nix", bin: "nix", args: []string{"profile", "install"}, lists: []string{"nix"}},
}

// Hooks for tests.
var (
	lookPath = exec.LookPath
	geteuid  = os.Geteuid
	run      = func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
)

// DetectManager returns the first available package manager, or nil.
func DetectManager() *Manager {
	for i := range managers {
		if _, err := lookPath(managers[i].bin); err == nil {
			m := managers[i]
			return &m
		}
	}
	return nil
}

// Name returns the display name of the package manager.
func (m *Manager) Name() string { return m.name }

// Command returns the command line that installs pkgs.
func (m *Manager) Command(pkgs []string) []string {
	cmd := append([]string{m.bin}, m.args...)
	if m.needRoot && geteuid() != 0 {
		cmd = append([]string{"sudo"}, cmd...)
	}
	return append(cmd, pkgs...)
}

// Install installs pkgs.
func (m *Manager) Install(pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	if err := ValidateNames(pkgs); err != nil {
		return err
	}
	cmd := m.Command(pkgs)
	return run(cmd[0], cmd[1:]...)
}

// ListFiles returns the package list files in the given layer directories
// that this manager reads, and the ones it ignores.
func (m *Manager) ListFiles(layerDirs []string) (used, ignored []string) {
	wanted := map[string]bool{}
	for _, l := range m.lists {
		wanted[l] = true
	}
	for _, dir := range layerDirs {
		pkgDir := filepath.Join(dir, "packages")
		entries, err := os.ReadDir(pkgDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			base := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".txt"), ".list")
			path := filepath.Join(pkgDir, e.Name())
			if wanted[strings.ToLower(base)] {
				used = append(used, path)
			} else {
				ignored = append(ignored, path)
			}
		}
	}
	return used, ignored
}

// LoadPackages reads a package list file. Lines starting with # are
// comments; text after " #" is a trailing comment.
func LoadPackages(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var pkgs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// "#" starts a comment only at the start of a word, so nix
		// references like nixpkgs#ripgrep survive.
		for _, field := range strings.Fields(scanner.Text()) {
			if strings.HasPrefix(field, "#") {
				break
			}
			pkgs = append(pkgs, field)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := ValidateNames(pkgs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return pkgs, nil
}

// ValidateNames rejects names that the package manager would read as
// options.
func ValidateNames(pkgs []string) error {
	for _, p := range pkgs {
		if strings.HasPrefix(p, "-") {
			return fmt.Errorf("invalid package name %q (must not start with '-')", p)
		}
	}
	return nil
}
