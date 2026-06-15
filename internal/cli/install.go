package cli

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/codebuff/dotf/internal/backup"
	"github.com/codebuff/dotf/internal/detect"
	"github.com/codebuff/dotf/internal/hook"
	"github.com/codebuff/dotf/internal/layer"
	"github.com/codebuff/dotf/internal/lock"
	"github.com/codebuff/dotf/internal/pkg"
	"github.com/codebuff/dotf/internal/safety"
	"github.com/codebuff/dotf/internal/secret"
	"github.com/codebuff/dotf/internal/state"
	"github.com/codebuff/dotf/internal/transaction"
)

// Install runs the install command with full hardening.
func Install(args []string, stateDir string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "Print what would be done without doing it")
	copyMode := fs.Bool("copy", false, "Copy files instead of creating symlinks")
	allowHooks := fs.Bool("allow-hooks", false, "Allow hook execution")
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Fprint(os.Stderr, `Usage: dotf install [options]

Install configured dotfiles to your home directory.

Options:
  --dry-run       Print what would be done without installing anything
  --copy          Copy files instead of creating symlinks (useful for containers)
  --allow-hooks   Enable hook script execution (disabled by default)
  --no-merge      Disable file merging (winner takes all)
  --help          Show this help message

DOTF automatically detects your environment and resolves the matching
configuration layers. No manual profile selection needed.
`)
		return nil
	}

	// Determine repository root
	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	// Validate we're in a DOTF repository
	layersDir := filepath.Join(repoRoot, "layers")
	if _, err := os.Stat(layersDir); os.IsNotExist(err) {
		return fmt.Errorf("not a DOTF repository: no 'layers' directory found in %s\n\nMake sure you run 'dotf install' from the root of your dotfiles repository", repoRoot)
	}

	// --- SIGNAL HANDLER ---
	// Set up signal handling for graceful abort
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// --- LOCK ---
	// Acquire lock to prevent concurrent operations
	l, err := lock.Acquire(stateDir, 30*time.Second)
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w\n\n(Use 'dotf doctor --unlock' to release a stale lock)", err)
	}
	defer l.Release()

	// --- TRANSACTION ---
	// Start a transaction for atomicity
	tx, err := transaction.New(stateDir)
	if err != nil {
		return fmt.Errorf("cannot start transaction: %w", err)
	}

	// Ensure rollback on failure
	var installErr error
	defer func() {
		if installErr != nil {
			fmt.Fprintln(os.Stderr, "\nInstallation failed. Rolling back...")
			tx.Rollback()
		}
	}()

	// Goroutine to handle signals
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived %v. Aborting...\n", sig)
		installErr = fmt.Errorf("aborted by signal: %v", sig)
		tx.Rollback()
		os.Exit(1)
	}()

	// --- DETECT ---
	p := detect.Detect()
	homeDir, _ := os.UserHomeDir()

	// Print detection
	fmt.Fprintln(os.Stderr, "Detected:")
	fmt.Fprintf(os.Stderr, "  distro:   %s\n", nonEmpty(p.Distro, "-"))
	fmt.Fprintf(os.Stderr, "  session:  %s\n", nonEmpty(p.Session, "-"))
	fmt.Fprintf(os.Stderr, "  wm:       %s\n", nonEmpty(p.WM, "-"))
	fmt.Fprintf(os.Stderr, "  desktop:  %s\n", nonEmpty(p.DE, "-"))
	fmt.Fprintf(os.Stderr, "  shell:    %s\n", nonEmpty(p.Shell, "-"))
	fmt.Fprintf(os.Stderr, "  terminal: %s\n", nonEmpty(p.Terminal, "-"))
	fmt.Fprintf(os.Stderr, "  gpu:      %s\n", nonEmpty(p.GPU, "-"))
	fmt.Fprintf(os.Stderr, "  hostname: %s\n", nonEmpty(p.Hostname, "-"))
	fmt.Fprintf(os.Stderr, "  device:   %s\n", nonEmpty(p.DeviceType, "-"))

	// --- RESOLVE LAYERS ---
	result, err := layer.Resolve(repoRoot, p)
	if err != nil {
		installErr = err
		return fmt.Errorf("resolving layers: %w", err)
	}

	if len(result.Layers) == 0 {
		fmt.Fprintln(os.Stderr, "\nNo layers found for the detected environment.")
		fmt.Fprintln(os.Stderr, "Create layer directories under 'layers/' to add support.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "\nActive layers (%d):\n", len(result.Layers))
	for _, l := range result.Layers {
		fmt.Fprintf(os.Stderr, "  - %s\n", l.Path())
	}

	if len(result.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "\nUnavailable layers (not found in repo):\n")
		for _, l := range result.Missing {
			fmt.Fprintf(os.Stderr, "  - %s (layer directory not found)\n", l.Path())
		}
	}

	// --- PRE-INSTALL HOOKS ---
	layerDirs := make([]string, len(result.Layers))
	for i, l := range result.Layers {
		layerDirs[i] = l.DirPath
	}
	hooks := hook.DiscoverHooks(layerDirs)
	hookLogFile := filepath.Join(stateDir, "dotf", "hooks.log")
	hookResults := hook.ExecuteAll(hooks, hook.PreInstall, hookLogFile, *allowHooks)
	for _, hr := range hookResults {
		if !hr.Success {
			fmt.Fprintf(os.Stderr, "  hook %s/%s: %s\n", hr.Hook.Layer, hr.Hook.Type, hr.Output)
		}
	}

	fmt.Fprintln(os.Stderr)

	// --- STATE ---
	sm, err := state.NewManager(stateDir)
	if err != nil {
		installErr = err
		return fmt.Errorf("cannot initialize state: %w", err)
	}

	// --- BACKUP ---
	backupMgr, err := backup.NewManager(filepath.Join(stateDir, "dotf", "backups"))
	if err != nil {
		installErr = err
		return fmt.Errorf("cannot initialize backups: %w", err)
	}

	// --- PACKAGES ---
	pm := pkg.DetectManager()
	if pm != nil {
		pkgFiles := pkg.FindPackageFiles(repoRoot, getLayerPaths(result.Layers))
		if len(pkgFiles) > 0 {
			fmt.Fprintf(os.Stderr, "Package manager: %s\n", pm.Name())
			for _, pf := range pkgFiles {
				pkgs, err := pkg.LoadPackages(pf)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  cannot read %s: %v\n", pf, err)
					continue
				}
				if len(pkgs) > 0 {
					fmt.Fprintf(os.Stderr, "  packages from %s: %d\n", filepath.Base(pf), len(pkgs))
					if !*dryRun {
						fmt.Fprintf(os.Stderr, "  installing... ")
						if err := pm.Install(pkgs); err != nil {
							fmt.Fprintf(os.Stderr, "error: %v\n", err)
						} else {
							fmt.Fprintln(os.Stderr, "done")
						}
					}
				}
			}
			fmt.Fprintln(os.Stderr)
		}
	}

	// --- SECRETS ---
	secrets := secret.DiscoverSecrets(repoRoot, getLayerPaths(result.Layers))
	if len(secrets) > 0 {
		fmt.Fprintf(os.Stderr, "Secrets found: %d\n", len(secrets))
		for _, s := range secrets {
			_ = s
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", s.Name, s.Method)
		}
		if !*dryRun {
			for _, s := range secrets {
				if err := secret.Decrypt(&s); err != nil {
					fmt.Fprintf(os.Stderr, "  cannot decrypt %s: %v\n", s.Name, err)
					continue
				}
				fmt.Fprintf(os.Stderr, "  decrypted %s\n", s.Name)
				defer secret.Destroy(&s)
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	// --- INSTALL FILES ---
	installed := make(map[string]string) // relative path -> layer path
	stats := &installStats{}

	// Determine install mode
	mode := layer.InstallSymlink
	if *copyMode {
		mode = layer.InstallCopy
	}
	if *dryRun {
		mode = layer.InstallDryRun
	}

walkLayers:
	for _, l := range result.Layers {
		walkErr := filepath.WalkDir(l.DirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			rel, walkRelErr := filepath.Rel(l.DirPath, path)
			if walkRelErr != nil {
				return walkRelErr
			}

			// --- SAFETY CHECKS ---
			// Validate the source file within the repository
			sourceResult := safety.ValidateLayerFile(repoRoot, path)
			if !sourceResult.Safe {
				stats.Errors = append(stats.Errors, fmt.Sprintf("safety: %s: %s", rel, sourceResult.Reason))
				return nil // skip unsafe files
			}

			// Validate the target path within home directory
			targetResult := safety.ValidateTargetPath(homeDir, rel)
			if !targetResult.Safe {
				stats.Errors = append(stats.Errors, fmt.Sprintf("safety: %s: %s", rel, targetResult.Reason))
				return nil // skip unsafe targets
			}
			targetPath := targetResult.Normalized

			// Check if already installed by a lower-priority layer
			alreadyInstalled := false
			if _, exists := installed[rel]; exists {
				alreadyInstalled = true
				if mode == layer.InstallDryRun {
					fmt.Fprintf(os.Stderr, "  override %s <- %s (was %s)\n", rel, l.Path(), installed[rel])
				}
			}

			targetDir := filepath.Dir(targetPath)

			if mode == layer.InstallDryRun {
				if !alreadyInstalled {
					fmt.Fprintf(os.Stderr, "  create  %s <- %s\n", rel, l.Path())
				}
				installed[rel] = l.Path()
				return nil
			}

			// --- BACKUP ---
			// Check if target exists and needs backup
			if _, statErr := os.Lstat(targetPath); statErr == nil && !alreadyInstalled {
				// Only back up if it's not already our symlink
				if !isSymlink(targetPath) {
					b, bErr := backupMgr.Create(rel, targetPath)
					if bErr != nil {
						stats.Errors = append(stats.Errors, fmt.Sprintf("backup %s: %v", rel, bErr))
						return nil
					}
					if b != nil {
						stats.BackedUp++
						fmt.Fprintf(os.Stderr, "  backed up %s\n", rel)
					}
				}
			}

			// Ensure target directory exists
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("mkdir %s: %v", targetDir, err))
				return nil
			}

			// --- INSTALL ---
			var installError error
			if mode == layer.InstallSymlink {
				// Remove existing file/symlink if present
				if _, statErr := os.Lstat(targetPath); statErr == nil {
					if err := os.Remove(targetPath); err != nil {
						stats.Errors = append(stats.Errors, fmt.Sprintf("remove %s: %v", rel, err))
						return err // abort walk on error
					}
				}
				// Use transaction for atomic symlink creation
				installError = tx.Symlink(path, targetPath)
			} else {
				// Use transaction for atomic file copy
				installError = tx.Copy(path, targetPath)
			}

			if installError != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("install %s: %v", rel, installError))
				installErr = fmt.Errorf("install %s: %w", rel, installError) // signal rollback
				return filepath.SkipAll // abort the walk
			}

			stats.Created++
			installed[rel] = l.Path()

			// Record in state
			fileType := "symlink"
			if mode == layer.InstallCopy {
				fileType = "copy"
			}
			sm.RecordFile(rel, l.Path(), fileType, path)

			return nil
		})
		if walkErr != nil && installErr == nil {
			// Walk encountered an error but installErr wasn't set
			installErr = walkErr
		}
		if installErr != nil {
			break walkLayers // stop processing remaining layers
		}
	}

	if installErr != nil {
		// Rollback will happen via the deferred function
		// Return the error after printing what we did manage
		if stats.Created > 0 {
			fmt.Fprintf(os.Stderr, "  installed %d files before failure\n", stats.Created)
		}
		return installErr
	}

	// --- POST-INSTALL HOOKS ---
	hookResults = hook.ExecuteAll(hooks, hook.PostInstall, hookLogFile, *allowHooks)
	for _, hr := range hookResults {
		if !hr.Success {
			fmt.Fprintf(os.Stderr, "  hook %s/%s: %s\n", hr.Hook.Layer, hr.Hook.Type, hr.Output)
		}
	}

	// --- COMMIT ---
	// Record install in state
	layerNames := make([]string, len(result.Layers))
	for i, l := range result.Layers {
		layerNames[i] = l.Path()
	}
	sm.RecordInstall(layerNames)

	if err := sm.Save(); err != nil {
		installErr = err
		return fmt.Errorf("cannot save state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		installErr = err
		return fmt.Errorf("cannot commit transaction: %w", err)
	}

	if mode == layer.InstallDryRun {
		fmt.Fprintf(os.Stderr, "Dry run complete. Use 'dotf install' (without --dry-run) to apply.\n")
	} else {
		fmt.Fprintf(os.Stderr, "\nDone: %s\n", stats.String())
	}

	// Error summary
	if len(stats.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\nErrors (%d):\n", len(stats.Errors))
		for _, e := range stats.Errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
	}

	return nil
}

// getLayerPaths extracts layer path strings from resolved layers.
func getLayerPaths(layers []layer.ResolvedLayer) []string {
	paths := make([]string, len(layers))
	for i, l := range layers {
		paths[i] = l.Path()
	}
	return paths
}

// installStats contains counts from an install operation.
type installStats struct {
	Created  int
	BackedUp int
	Errors   []string
}

func (s *installStats) String() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%d created", s.Created))
	if s.BackedUp > 0 {
		parts = append(parts, fmt.Sprintf("%d backed up", s.BackedUp))
	}
	if len(s.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", len(s.Errors)))
	}
	return strings.Join(parts, ", ")
}

// nonEmpty returns the string value or a fallback if empty.
func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// isSymlink checks if a path is a symbolic link.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}
