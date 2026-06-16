// Package reorganize detects flat dotfiles repository structures and reorganizes
// them into the layered structure that DOTF expects. It never modifies the
// original files — it creates the layer structure and copies/symlinks files into it.
package reorganize

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AppLayerMap maps flat application directory names to their corresponding
// layer type/name paths within the layers/ directory structure.
var AppLayerMap = map[string]string{
	"alacritty":  "terminal/alacritty",
	"awesome":    "wm/awesome",
	"cht-sh":     "base",
	"fish":       "shell/fish",
	"flameshot":  "base",
	"ghostty":    "terminal/ghostty",
	"neofetch":   "base",
	"nix":        "base",
	"picom":      "wm/picom",
	"presenterm": "base",
	"rofi":       "wm/rofi",
	"scripts":    "base",
	"starship":   "shell/starship",
	"tmux":       "base",
	"yazi":       "base",
	"zsh":        "shell/zsh",
}

// DetectionLayerDirs lists empty layer directories that should be created
// for automatic detection to work properly.
var DetectionLayerDirs = []string{
	"distro/arch",
	"distro/fedora",
	"distro/ubuntu",
	"distro/nixos",
	"wm/hyprland",
	"wm/awesome",
	"wm/qtile",
	"wm/sway",
	"shell/bash",
	"shell/zsh",
	"shell/fish",
	"terminal/alacritty",
	"terminal/ghostty",
	"terminal/kitty",
	"device/laptop",
	"device/desktop",
	"gpu/amd",
	"gpu/nvidia",
	"gpu/intel",
}

// Result describes what the reorganization did.
type Result struct {
	Moved   map[string]int // layer path -> file count
	Created []string       // created directories
	Skipped []string       // skipped app dirs with reasons
	Orphans []string       // flat dirs that couldn't be mapped
}

// Analyze scans the dotfiles directory and determines which flat directories
// can be mapped to layer paths. Returns the mapping without making changes.
func Analyze(dotfilesDir string) (*Result, error) {
	result := &Result{
		Moved:   make(map[string]int),
		Created: nil,
		Skipped: nil,
		Orphans: nil,
	}

	entries, err := os.ReadDir(dotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read dotfiles directory %s: %w", dotfilesDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories (like .git, .gitmodules) and layers/ itself
		if strings.HasPrefix(name, ".") || name == "layers" {
			continue
		}

		layerPath, ok := AppLayerMap[name]
		if !ok {
			// Count files to see if it's worth reporting
			fileCount := countFiles(filepath.Join(dotfilesDir, name))
			if fileCount > 0 {
				result.Orphans = append(result.Orphans, name)
			}
			continue
		}

		srcDir := filepath.Join(dotfilesDir, name)

		// Check for git submodule
		if isSubmodule(srcDir) {
			count := countFiles(srcDir)
			if count > 0 {
				result.Moved[layerPath] = count
			} else {
				result.Skipped = append(result.Skipped, name+" (empty)")
			}
			continue
		}

		files := collectFiles(srcDir)
		if len(files) == 0 {
			result.Skipped = append(result.Skipped, name+" (empty)")
			continue
		}

		result.Moved[layerPath] = len(files)
	}

	return result, nil
}

// Reorganize creates the layer structure and copies files from flat directories
// into the appropriate layer directories. Original files are never modified.
func Reorganize(dotfilesDir string, result *Result) error {
	if result == nil {
		var err error
		result, err = Analyze(dotfilesDir)
		if err != nil {
			return err
		}
	}

	layersDir := filepath.Join(dotfilesDir, "layers")

	for appName, layerPath := range AppLayerMap {
		srcDir := filepath.Join(dotfilesDir, appName)
		info, err := os.Stat(srcDir)
		if err != nil || !info.IsDir() {
			continue
		}
		// Skip hidden dirs or layers/ itself
		if strings.HasPrefix(appName, ".") || appName == "layers" {
			continue
		}

		// Check for git submodule — if it has a .git directory, preserve as-is by
		// copying the entire directory tree (including submodule contents)
		if isSubmodule(srcDir) {
			targetDir := filepath.Join(layersDir, layerPath)
			if err := copyDir(srcDir, targetDir); err != nil {
				return fmt.Errorf("cannot copy submodule %s: %w", appName, err)
			}
			result.Created = append(result.Created, layerPath)
			result.Moved[layerPath] = countFiles(srcDir)
			continue
		}

		files := collectFiles(srcDir)
		if len(files) == 0 {
			continue
		}

		// Create target layer directory
		targetDir := filepath.Join(layersDir, layerPath)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("cannot create layer directory %s: %w", targetDir, err)
		}
		result.Created = append(result.Created, layerPath)

		// Copy each file from flat dir to layer dir
		for _, relPath := range files {
			srcFile := filepath.Join(srcDir, relPath)
			dstFile := filepath.Join(targetDir, relPath)

			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
				return fmt.Errorf("cannot create directory %s: %w", filepath.Dir(dstFile), err)
			}

			// Read source and write to destination
			data, err := os.ReadFile(srcFile)
			if err != nil {
				return fmt.Errorf("cannot read %s: %w", srcFile, err)
			}
			if err := os.WriteFile(dstFile, data, 0644); err != nil {
				return fmt.Errorf("cannot write %s: %w", dstFile, err)
			}
		}
	}

	// Create detection layer directories (only if they don't exist)
	for _, layerPath := range DetectionLayerDirs {
		dir := filepath.Join(layersDir, layerPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("cannot create detection layer %s: %w", dir, err)
			}
			result.Created = append(result.Created, layerPath)
		}
	}

	// Move loose files (like README.md) to base/
	baseDir := filepath.Join(layersDir, "base")
	for _, name := range []string{"README.md", "README", "readme.md"} {
		srcFile := filepath.Join(dotfilesDir, name)
		dstFile := filepath.Join(baseDir, name)
		if _, err := os.Stat(srcFile); err == nil {
			if _, err := os.Stat(dstFile); os.IsNotExist(err) {
				data, err := os.ReadFile(srcFile)
				if err != nil {
					return fmt.Errorf("cannot read %s: %w", srcFile, err)
				}
				if err := os.MkdirAll(baseDir, 0755); err != nil {
					return fmt.Errorf("cannot create base dir: %w", err)
				}
				if err := os.WriteFile(dstFile, data, 0644); err != nil {
					return fmt.Errorf("cannot write %s: %w", dstFile, err)
				}
			}
		}
	}

	return nil
}

// FormatResult returns a human-readable summary of the reorganization.
func FormatResult(r *Result) string {
	var b strings.Builder

	b.WriteString("\nReorganization completed:\n")

	if len(r.Moved) > 0 {
		b.WriteString("\nFiles moved:\n")
		// Sort by layer path for deterministic output
		layers := make([]string, 0, len(r.Moved))
		for l := range r.Moved {
			layers = append(layers, l)
		}
		sort.Strings(layers)
		for _, l := range layers {
			fmt.Fprintf(&b, "  %s: %d files\n", l, r.Moved[l])
		}
	}

	if len(r.Created) > 0 {
		b.WriteString("\nLayer directories created:\n")
		for _, d := range r.Created {
			fmt.Fprintf(&b, "  layers/%s/\n", d)
		}
	}

	if len(r.Orphans) > 0 {
		b.WriteString("\nUnrecognized directories (could not map to a layer):\n")
		for _, d := range r.Orphans {
			fmt.Fprintf(&b, "  %s/ — no mapping found for this directory\n", d)
		}
		b.WriteString("  These directories were not placed into any layer. Create the corresponding layer\n")
		b.WriteString("  directories manually or extend the app-to-layer mapping.\n")
	}

	if len(r.Skipped) > 0 {
		b.WriteString("\nSkipped:\n")
		for _, s := range r.Skipped {
			fmt.Fprintf(&b, "  %s\n", s)
		}
	}

	return b.String()
}

// IsFlatStructure checks if a dotfiles directory uses a flat structure
// (application directories at root) rather than the layered format.
func IsFlatStructure(dotfilesDir string) bool {
	entries, err := os.ReadDir(dotfilesDir)
	if err != nil {
		return false
	}

	// If it has a layers/ directory at root, it's already organized
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == "layers" {
			// Check if layers/ has actual layer subdirectories
			layerEntries, err := os.ReadDir(filepath.Join(dotfilesDir, "layers"))
			if err == nil && len(layerEntries) > 0 {
				return false // already has layer structure
			}
			// layers/ exists but is empty — treat as flat
			return true
		}
	}

	// Check for known flat app directories
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if _, ok := AppLayerMap[entry.Name()]; ok {
			return true
		}
	}

	return false
}

// collectFiles recursively collects all file paths relative to root, skipping .git.
// Best-effort: errors during walk are ignored, partial results are returned.
func collectFiles(root string) []string {
	var files []string
	//nolint:errcheck // best-effort collection
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files
}

// countFiles counts non-hidden files in a directory tree, skipping .git.
// Best-effort: errors during walk are ignored, partial results are returned.
func countFiles(root string) int {
	count := 0
	//nolint:errcheck // best-effort counting
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		count++
		return nil
	})
	return count
}

// copyDir recursively copies a directory tree from src to dst, skipping .git.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)

		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	})
}

// isSubmodule checks if a directory is a git submodule (has a .git entry).
func isSubmodule(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}
