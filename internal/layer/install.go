package layer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// InstallMode controls how files are installed.
type InstallMode int

const (
	// InstallSymlink creates symlinks from $HOME to layer files.
	InstallSymlink InstallMode = iota
	// InstallCopy copies layer files to $HOME.
	InstallCopy
	// InstallDryRun prints what would be done without doing it.
	InstallDryRun
)

// InstallStats contains counts from an install operation.
type InstallStats struct {
	Created  int
	Skipped  int
	BackedUp int
	Errors   []string
}

// Install applies the resolved layers to the user's home directory.
// Layers are applied from lowest to highest priority, so higher-priority files
// overwrite lower-priority ones.
func Install(layers []ResolvedLayer, homeDir string, mode InstallMode) (*InstallStats, error) {
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
	}

	stats := &InstallStats{}

	// Track which files we've already installed (to handle overrides)
	// Maps relative file path -> source layer path
	installed := make(map[string]string)

	// Apply layers from lowest to highest priority
	for _, l := range layers {
		err := filepath.WalkDir(l.DirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			// Compute the relative path within the layer
			rel, err := filepath.Rel(l.DirPath, path)
			if err != nil {
				return err
			}

			targetPath := filepath.Join(homeDir, rel)
			targetDir := filepath.Dir(targetPath)

			// Check if this file was already installed by a lower-priority layer
			alreadyInstalled := false
			if _, exists := installed[rel]; exists {
				alreadyInstalled = true
				if mode == InstallDryRun {
					fmt.Fprintf(os.Stderr, "  override %s <- %s (was %s)\n", rel, l.Path(), installed[rel])
				}
			}

			switch mode {
			case InstallDryRun:
				if !alreadyInstalled {
					fmt.Fprintf(os.Stderr, "  create  %s <- %s\n", rel, l.Path())
				}
				installed[rel] = l.Path()
			case InstallSymlink:
				return installSymlink(path, targetPath, targetDir, l.Path(), rel, alreadyInstalled, installed, stats)

			case InstallCopy:
				return installCopy(path, targetPath, targetDir, l.Path(), rel, alreadyInstalled, installed, stats)
			}

			return nil
		})
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("layer %s: %v", l.Path(), err))
		}
	}

	return stats, nil
}

// installSymlink creates a symlink from targetPath pointing to sourcePath.
func installSymlink(sourcePath, targetPath, targetDir, layerPath, rel string, alreadyInstalled bool, installed map[string]string, stats *InstallStats) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("mkdir %s: %v", targetDir, err))
		return nil
	}

	// Check existing target
	if _, err := os.Lstat(targetPath); err == nil {
		if alreadyInstalled {
			// We already installed this from a lower-priority layer — just remove the old symlink
			if err := os.Remove(targetPath); err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("remove %s: %v", rel, err))
				return nil
			}
		} else if !isSymlink(targetPath) {
			// A real file exists — back it up
			backupPath := targetPath + ".bak"
			if err := os.Rename(targetPath, backupPath); err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("backup %s: %v", rel, err))
				return nil
			}
			stats.BackedUp++
			fmt.Fprintf(os.Stderr, "  backed up %s -> %s\n", rel, filepath.Base(backupPath))
		} else {
			// It's a symlink from a previous install — just remove it
			if err := os.Remove(targetPath); err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("remove %s: %v", rel, err))
				return nil
			}
		}
	}

	// Create the symlink
	if err := os.Symlink(sourcePath, targetPath); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("symlink %s: %v", rel, err))
		return nil
	}

	stats.Created++
	installed[rel] = layerPath
	return nil
}

// installCopy copies a file from sourcePath to targetPath.
func installCopy(sourcePath, targetPath, targetDir, layerPath, rel string, alreadyInstalled bool, installed map[string]string, stats *InstallStats) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("mkdir %s: %v", targetDir, err))
		return nil
	}

	// Check existing target
	if _, err := os.Lstat(targetPath); err == nil {
		if alreadyInstalled {
			// We already installed this from a lower-priority layer — remove it
			if err := os.Remove(targetPath); err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("remove %s: %v", rel, err))
				return nil
			}
		} else {
			// A real file exists — back it up
			backupPath := targetPath + ".bak"
			if err := os.Rename(targetPath, backupPath); err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("backup %s: %v", rel, err))
				return nil
			}
			stats.BackedUp++
			fmt.Fprintf(os.Stderr, "  backed up %s -> %s\n", rel, filepath.Base(backupPath))
		}
	}

	// Copy the file
	src, err := os.Open(sourcePath)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("open %s: %v", rel, err))
		return nil
	}
	defer src.Close()

	dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("create %s: %v", rel, err))
		return nil
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("copy %s: %v", rel, err))
		return nil
	}

	stats.Created++
	installed[rel] = layerPath
	return nil
}

// Uninstall removes all symlinks created by a previous install.
func Uninstall(layers []ResolvedLayer, homeDir string) (*InstallStats, error) {
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
	}

	stats := &InstallStats{}

	for _, l := range layers {
		err := filepath.WalkDir(l.DirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			rel, err := filepath.Rel(l.DirPath, path)
			if err != nil {
				return err
			}

			targetPath := filepath.Join(homeDir, rel)

			// Check if it's a symlink pointing to our layer
			target, err := os.Readlink(targetPath)
			if err != nil {
				return nil // not a symlink or doesn't exist
			}

			if target == path {
				if err := os.Remove(targetPath); err != nil {
					stats.Errors = append(stats.Errors, fmt.Sprintf("remove %s: %v", rel, err))
					return nil
				}
				stats.Created++
			}

			return nil
		})
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("layer %s: %v", l.Path(), err))
		}
	}

	return stats, nil
}

// isSymlink checks if a path is a symbolic link.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// String returns a human-readable summary of install stats.
func (s *InstallStats) String() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%d created", s.Created))
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}
	if s.BackedUp > 0 {
		parts = append(parts, fmt.Sprintf("%d backed up", s.BackedUp))
	}
	if len(s.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", len(s.Errors)))
	}
	return strings.Join(parts, ", ")
}
