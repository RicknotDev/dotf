// Package pkg provides a unified abstraction over Linux package managers.
// Supports pacman, paru, yay, apt, dnf, zypper, and nix.
package pkg

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager defines the interface for a package manager.
type Manager interface {
	// Name returns the display name of the package manager.
	Name() string

	// Available checks if this package manager is installed.
	Available() bool

	// Install installs the given packages.
	Install(pkgs []string) error

	// InstallOne installs a single package.
	InstallOne(pkg string) error
}

// DetectManager detects the available package manager.
func DetectManager() Manager {
	candidates := []Manager{
		&Pacman{},
		&Paru{},
		&Yay{},
		&Apt{},
		&Dnf{},
		&Zypper{},
		&Nix{},
	}

	for _, m := range candidates {
		if m.Available() {
			return m
		}
	}

	return nil
}

// LoadPackages reads a package list file.
// Lines starting with # are comments. Empty lines are ignored.
func LoadPackages(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var pkgs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pkgs = append(pkgs, line)
	}

	return pkgs, scanner.Err()
}

// FindPackageFiles finds all package list files in resolved layers.
func FindPackageFiles(layersDir string, layerPaths []string) []string {
	var files []string
	for _, layer := range layerPaths {
		// Search for packages/ directory in each layer
		pkgDir := filepath.Join(layersDir, layer, "packages")
		entries, err := os.ReadDir(pkgDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if ext == ".txt" || ext == ".list" || ext == "" {
					files = append(files, filepath.Join(pkgDir, entry.Name()))
				}
			}
		}
	}
	return files
}

// --- Pacman ---

// Pacman implements Manager for Arch Linux pacman.
type Pacman struct{}

func (p *Pacman) Name() string           { return "pacman" }
func (p *Pacman) Available() bool        { return which("pacman") }
func (p *Pacman) Install(pkgs []string) error {
	return runCmd("pacman", append([]string{"-S", "--noconfirm", "--needed"}, pkgs...)...)
}
func (p *Pacman) InstallOne(pkg string) error {
	return p.Install([]string{pkg})
}

// --- Paru ---

// Paru implements Manager for paru (AUR helper).
type Paru struct{}

func (p *Paru) Name() string           { return "paru" }
func (p *Paru) Available() bool        { return which("paru") }
func (p *Paru) Install(pkgs []string) error {
	return runCmd("paru", append([]string{"-S", "--noconfirm", "--needed"}, pkgs...)...)
}
func (p *Paru) InstallOne(pkg string) error {
	return p.Install([]string{pkg})
}

// --- Yay ---

// Yay implements Manager for yay (AUR helper).
type Yay struct{}

func (y *Yay) Name() string           { return "yay" }
func (y *Yay) Available() bool        { return which("yay") }
func (y *Yay) Install(pkgs []string) error {
	return runCmd("yay", append([]string{"-S", "--noconfirm", "--needed"}, pkgs...)...)
}
func (y *Yay) InstallOne(pkg string) error {
	return y.Install([]string{pkg})
}

// --- Apt ---

// Apt implements Manager for Debian/Ubuntu apt.
type Apt struct{}

func (a *Apt) Name() string           { return "apt" }
func (a *Apt) Available() bool        { return which("apt-get") }
func (a *Apt) Install(pkgs []string) error {
	return runCmd("apt-get", append([]string{"install", "-y"}, pkgs...)...)
}
func (a *Apt) InstallOne(pkg string) error {
	return a.Install([]string{pkg})
}

// --- Dnf ---

// Dnf implements Manager for Fedora dnf.
type Dnf struct{}

func (d *Dnf) Name() string           { return "dnf" }
func (d *Dnf) Available() bool        { return which("dnf") }
func (d *Dnf) Install(pkgs []string) error {
	return runCmd("dnf", append([]string{"install", "-y"}, pkgs...)...)
}
func (d *Dnf) InstallOne(pkg string) error {
	return d.Install([]string{pkg})
}

// --- Zypper ---

// Zypper implements Manager for openSUSE zypper.
type Zypper struct{}

func (z *Zypper) Name() string           { return "zypper" }
func (z *Zypper) Available() bool        { return which("zypper") }
func (z *Zypper) Install(pkgs []string) error {
	return runCmd("zypper", append([]string{"install", "-y"}, pkgs...)...)
}
func (z *Zypper) InstallOne(pkg string) error {
	return z.Install([]string{pkg})
}

// --- Nix ---

// Nix implements Manager for NixOS nix-env / nix profile.
type Nix struct{}

func (n *Nix) Name() string           { return "nix" }
func (n *Nix) Available() bool        { return which("nix-env") || which("nix") }
func (n *Nix) Install(pkgs []string) error {
	// Try nix profile install first, fall back to nix-env
	if which("nix") {
		return runCmd("nix", append([]string{"profile", "install"}, pkgs...)...)
	}
	return runCmd("nix-env", append([]string{"-iA"}, pkgs...)...)
}
func (n *Nix) InstallOne(pkg string) error {
	return n.Install([]string{pkg})
}

// --- Helpers ---

// which checks if a command is available in PATH.
func which(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// runCmd runs a command and streams output to stderr.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
