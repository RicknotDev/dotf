package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codebuff/dotf/internal/detect"
	"github.com/codebuff/dotf/internal/layer"
	"github.com/codebuff/dotf/internal/state"
)

// Inspect runs the inspect command.
func Inspect(args []string, stateDir string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Fprint(os.Stderr, `Usage: dotf inspect <subcommand> [options]

Inspect DOTF internals.

Subcommands:
  file <path>          Show which layer provides a specific file
  layer <name>         Show all files in a specific layer
  state                Show full state information
  overrides            Show all file overrides with details
  backup               Show backup manifest

Examples:
  dotf inspect file .config/kitty/kitty.conf
  dotf inspect layer distro/arch
  dotf inspect state
`)
		return nil
	}

	repoRoot, _ := os.Getwd()
	remaining := fs.Args()

	if len(remaining) < 1 {
		return fmt.Errorf("inspect requires a subcommand: file, layer, state, overrides, or backup")
	}

	subcmd := remaining[0]
	subargs := remaining[1:]

	switch subcmd {
	case "file":
		return inspectFile(repoRoot, subargs)
	case "layer":
		return inspectLayer(repoRoot, subargs)
	case "state":
		return inspectState(stateDir)
	case "overrides":
		return inspectOverrides(repoRoot)
	case "backup":
		return inspectBackup(stateDir)
	default:
		return fmt.Errorf("unknown inspect subcommand: %s (use: file, layer, state, overrides, backup)", subcmd)
	}
}

func inspectFile(repoRoot string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("file path required")
	}
	filePath := args[0]

	p := detect.Detect()
	result, err := layer.Resolve(repoRoot, p)
	if err != nil {
		return fmt.Errorf("resolving layers: %w", err)
	}

	fmt.Printf("Inspecting file: %s\n", filePath)
	fmt.Println()

	found := false
	for _, l := range result.Layers {
		fullPath := filepath.Join(l.DirPath, filePath)
		if _, err := os.Stat(fullPath); err == nil {
			fmt.Printf("  Layer:  %s\n", l.Path())
			fmt.Printf("  Path:   %s\n", fullPath)
			found = true
		}
	}

	if !found {
		fmt.Println("  (not found in any layer)")
	}

	// Check if installed
	homeDir, _ := os.UserHomeDir()
	targetPath := filepath.Join(homeDir, filePath)
	if info, err := os.Lstat(targetPath); err == nil {
		fmt.Println()
		fmt.Println("Installed status:")
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(targetPath)
			fmt.Printf("  Type:   symlink\n")
			fmt.Printf("  Target: %s\n", target)
		} else {
			fmt.Printf("  Type:   regular file\n")
		}
	} else {
		fmt.Println()
		fmt.Println("(not installed)")
	}

	return nil
}

func inspectLayer(repoRoot string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("layer name required (e.g., distro/arch)")
	}
	layerName := args[0]

	layerDir := filepath.Join(repoRoot, "layers", layerName)
	info, err := os.Stat(layerDir)
	if err != nil {
		return fmt.Errorf("layer not found: %s (%s)", layerName, layerDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", layerDir)
	}

	fmt.Printf("Layer: %s\n", layerName)
	fmt.Printf("Path:  %s\n", layerDir)
	fmt.Println()
	fmt.Println("Files:")

	count := 0
	filepath.WalkDir(layerDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(layerDir, path)
		fmt.Printf("  %s\n", rel)
		count++
		return nil
	})

	fmt.Println()
	fmt.Printf("Total: %d files\n", count)
	return nil
}

func inspectState(stateDir string) error {
	sm, err := state.NewManager(stateDir)
	if err != nil {
		return fmt.Errorf("cannot open state: %w", err)
	}

	s := sm.GetState()

	fmt.Println("DOTF State")
	fmt.Println("==========")
	fmt.Printf("  Version:        %d\n", s.Version)
	fmt.Printf("  Repository:     %s\n", s.Repository)
	fmt.Printf("  Last install:   %s\n", nonEmpty(s.LastInstall, "never"))
	fmt.Println()

	fmt.Printf("Installed layers (%d):\n", len(s.InstalledLayers))
	for _, l := range s.InstalledLayers {
		fmt.Printf("  %s\n", l)
	}
	fmt.Println()

	fmt.Printf("Installed files (%d):\n", len(s.InstalledFiles))
	for path, ref := range s.InstalledFiles {
		fmt.Printf("  %s [%s] from %s\n", path, ref.Type, ref.Layer)
	}
	fmt.Println()

	fmt.Printf("Backups (%d):\n", len(s.BackupManifest))
	for path, b := range s.BackupManifest {
		fmt.Printf("  %s -> %s (%s)\n", path, b.Original, b.Created)
	}

	return nil
}

func inspectOverrides(repoRoot string) error {
	p := detect.Detect()
	result, err := layer.Resolve(repoRoot, p)
	if err != nil {
		return fmt.Errorf("resolving layers: %w", err)
	}

	overrides := layer.ComputeOverrides(result.Layers)

	fmt.Println("File Overrides")
	fmt.Println("==============")
	fmt.Println()
	fmt.Print(layer.FormatOverrideGroups(overrides))

	// Also show non-overridden files for completeness
	fmt.Println()
	fmt.Println("All resolved files:")
	allFiles := make(map[string][]string)
	for _, l := range result.Layers {
		filepath.WalkDir(l.DirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(l.DirPath, path)
			allFiles[rel] = append(allFiles[rel], l.Path())
			return nil
		})
	}

	// Sort and display
	fileList := make([]string, 0, len(allFiles))
	for f := range allFiles {
		fileList = append(fileList, f)
	}
	sort.Strings(fileList)

	for _, f := range fileList {
		sources := allFiles[f]
		if len(sources) == 1 {
			fmt.Printf("  %s <- %s\n", f, sources[0])
		} else {
			fmt.Printf("  %s <- %s (overrides: %s)\n", f, sources[0], strings.Join(sources[1:], ", "))
		}
	}

	return nil
}

func inspectBackup(stateDir string) error {
	backupDir := filepath.Join(stateDir, "dotf", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No backups found.")
			return nil
		}
		return fmt.Errorf("cannot read backup directory: %w", err)
	}

	fmt.Println("Backup Manifest")
	fmt.Println("===============")
	fmt.Println()

	if len(entries) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bak") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fmt.Printf("  %s (%d bytes, %s)\n",
			entry.Name(),
			info.Size(),
			info.ModTime().Format("2006-01-02 15:04:05"))
	}

	return nil
}


