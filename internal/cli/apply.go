package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/codebuff/dotf/internal/backup"
	"github.com/codebuff/dotf/internal/detect"
	"github.com/codebuff/dotf/internal/layer"
	"github.com/codebuff/dotf/internal/lock"
	"github.com/codebuff/dotf/internal/safety"
	"github.com/codebuff/dotf/internal/state"
	"github.com/codebuff/dotf/internal/transaction"
)

// Apply runs the apply command.
func Apply(args []string, stateDir string, cfg OutputConfig) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	profile := fs.String("profile", "", "Profile to apply (overrides auto-detection)")
	dryRun := fs.Bool("dry-run", false, "Print what would be done without doing it")
	diff := fs.Bool("diff", false, "Show what would change compared to current state")
	noInteractive := fs.Bool("no-interactive", false, "Non-interactive mode (abort on conflicts)")
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		cfg.Print(`Usage: dotf apply [options]

Apply the resolved configuration layers to your home directory.

Options:
  --profile <name>     Override auto-detected profile
  --dry-run            Show what would be done without applying
  --diff               Show what would change compared to current state
  --no-interactive     Abort on conflicts instead of prompting
  --json               Output in JSON format (global flag)
  --help               Show this help message

Examples:
  dotf apply                    # Apply auto-detected profile
  dotf apply --profile laptop   # Apply specific profile
  dotf apply --dry-run          # Preview changes
  dotf apply --diff             # Show changes vs current state
`)
		return nil
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	layersDir := filepath.Join(repoRoot, "layers")
	if _, err := os.Stat(layersDir); os.IsNotExist(err) {
		return fmt.Errorf("not a DOTF repository: no 'layers' directory found in %s", repoRoot)
	}

	// Load dotf.yaml if present
	dotfCfg, err := LoadDotfConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("cannot load dotf config: %w", err)
	}

	// Detect environment or use specified profile
	p := detect.Detect()
	if *profile != "" {
		// TODO: support named profiles
		cfg.PrintErr("Profile override: %s (auto-detection disabled)\n", *profile)
	}

	// Resolve layers
	result, err := layer.Resolve(repoRoot, p)
	if err != nil {
		return fmt.Errorf("resolving layers: %w", err)
	}

	if len(result.Layers) == 0 {
		return fmt.Errorf("nothing to do: no layers resolved for current environment")
	}

	// Diff/dry-run mode: show what would change
	if *diff || *dryRun {
		if cfg.JSON {
			dryRunResult := map[string]interface{}{
				"mode":       "preview",
				"layers":     getLayerPaths(result.Layers),
				"layer_count": len(result.Layers),
				"has_config":  dotfCfg != nil,
				"dry_run":    *dryRun,
			}
			if dotfCfg != nil {
				dryRunResult["profile"] = dotfCfg.Profile
			}
			cfg.PrintJSON(dryRunResult)
		} else {
			cfg.Print("Would apply the following layers:")
			for _, l := range result.Layers {
				cfg.Printf("  - %s", l.Path())
			}
			cfg.Printf("Total: %d layers", len(result.Layers))
			if dotfCfg != nil {
				cfg.Printf("Config: dotf.yaml found (profile: %s)", dotfCfg.Profile)
			}
			if *dryRun {
				cfg.Print("Use 'dotf apply' (without --dry-run) to apply.")
			}
		}
		return nil
	}

	// Acquire lock
	l, err := lock.Acquire(stateDir, 30*time.Second)
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer l.Release()

	// Start transaction
	tx, err := transaction.New(stateDir)
	if err != nil {
		return fmt.Errorf("cannot start transaction: %w", err)
	}

	var applyErr error
	defer func() {
		if applyErr != nil {
			cfg.PrintErr("\nApply failed. Rolling back...\n")
			tx.Rollback()
		}
	}()

	// Initialize state and backup
	sm, err := state.NewManager(stateDir)
	if err != nil {
		applyErr = err
		return fmt.Errorf("cannot initialize state: %w", err)
	}

	backupMgr, err := backup.NewManager(filepath.Join(stateDir, "backups"))
	if err != nil {
		applyErr = err
		return fmt.Errorf("cannot initialize backups: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		applyErr = err
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Apply each layer
	applied := 0
	backedUp := 0
	conflicts := 0

	for _, l := range result.Layers {
		walkErr := filepath.WalkDir(l.DirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			rel, relErr := filepath.Rel(l.DirPath, path)
			if relErr != nil {
				return relErr
			}

			// Safety check: validate source and target paths
			sourceResult := safety.ValidateLayerFile(repoRoot, path)
			if !sourceResult.Safe {
				cfg.PrintErr("  safety: %s: %s\n", rel, sourceResult.Reason)
				return nil // skip unsafe files
			}

			targetResult := safety.ValidateTargetPath(homeDir, rel)
			if !targetResult.Safe {
				cfg.PrintErr("  safety: %s: %s\n", rel, targetResult.Reason)
				return nil
			}
			targetPath := targetResult.Normalized

			// Check for existing file (conflict)
			if _, statErr := os.Lstat(targetPath); statErr == nil {
				if !isSymlink(targetPath) {
					if *noInteractive {
						conflicts++
						return fmt.Errorf("conflict: %s already exists (aborting due to --no-interactive)", rel)
					}
					// Create backup before overwriting
					b, bErr := backupMgr.Create(rel, targetPath)
					if bErr != nil {
						cfg.PrintErr("  backup %s failed: %v\n", rel, bErr)
					} else if b != nil {
						backedUp++
						sm.RecordBackup(b.BackupPath, rel)
					}
				}
			}

			// Ensure target directory exists
			if dirErr := os.MkdirAll(filepath.Dir(targetPath), 0755); dirErr != nil {
				return fmt.Errorf("cannot create directory for %s: %w", rel, dirErr)
			}

			// Remove existing file if present (it was backed up above)
			if _, statErr := os.Lstat(targetPath); statErr == nil {
				if err := os.Remove(targetPath); err != nil {
					return fmt.Errorf("cannot remove %s: %w", rel, err)
				}
			}

			// Create symlink
			if err := tx.Symlink(path, targetPath); err != nil {
				return fmt.Errorf("cannot create symlink for %s: %w", rel, err)
			}

			applied++
			sm.RecordFile(rel, l.Path(), "symlink", path)
			return nil
		})

		if walkErr != nil {
			applyErr = walkErr
			if *noInteractive && conflicts > 0 {
				return fmt.Errorf("conflict: %d file(s) already exist (aborting due to --no-interactive)", conflicts)
			}
			return walkErr
		}
	}

	if applyErr != nil {
		return applyErr
	}

	// Commit
	layerNames := make([]string, len(result.Layers))
	for i, l := range result.Layers {
		layerNames[i] = l.Path()
	}
	sm.RecordInstall(layerNames)

	if err := sm.Save(); err != nil {
		return fmt.Errorf("cannot save state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit transaction: %w", err)
	}

	cfg.Print(cfg.Green(fmt.Sprintf("Applied %d files from %d layers.\n", applied, len(result.Layers))))
	return nil
}
