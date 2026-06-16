package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/codebuff/dotf/internal/lock"
	"github.com/codebuff/dotf/internal/state"
	"github.com/codebuff/dotf/internal/transaction"
)

// Unapply runs the unapply command.
func Unapply(args []string, stateDir string, cfg OutputConfig) error {
	fs := flag.NewFlagSet("unapply", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "Show what would be done without unapplying")
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		cfg.Print(`Usage: dotf unapply [options]

Revert applied configuration layers by removing symlinks.

Options:
  --dry-run    Show what would be done without unapplying
  --json       Output in JSON format (global flag)
  --help       Show this help message

Examples:
  dotf unapply          # Remove all applied symlinks
  dotf unapply --dry-run  # Preview what would be removed
`)
		return nil
	}

	sm, err := state.NewManager(stateDir)
	if err != nil {
		return fmt.Errorf("cannot open state: %w", err)
	}

	s := sm.GetState()
	if len(s.InstalledFiles) == 0 {
		return fmt.Errorf("nothing to do: no installed files")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
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

	var unapplyErr error
	defer func() {
		if unapplyErr != nil {
			cfg.PrintErr("\nUnapply failed. Rolling back...\n")
			tx.Rollback()
		}
	}()

	removed := 0

	for relPath := range s.InstalledFiles {
		targetPath := filepath.Join(homeDir, relPath)

		if *dryRun {
			cfg.Printf("  remove %s\n", relPath)
			removed++
			continue
		}

		// Check if it's our symlink
		info, lerr := os.Lstat(targetPath)
		if lerr != nil {
			if os.IsNotExist(lerr) {
				removed++
				continue
			}
			cfg.PrintErr("  cannot stat %s: %v\n", relPath, lerr)
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			if err := tx.Remove(targetPath); err != nil {
				cfg.PrintErr("  cannot remove %s: %v\n", relPath, err)
				unapplyErr = err
				continue
			}
			removed++
		}
	}

	if unapplyErr != nil {
		return unapplyErr
	}

	// Commit and clear state
	if !*dryRun {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("cannot commit transaction: %w", err)
		}

		// Clear installed files from state
		sm.RecordInstall([]string{})
		sm.ClearFiles()
		if err := sm.Save(); err != nil {
			return fmt.Errorf("cannot save state: %w", err)
		}
	}

	if cfg.JSON {
		cfg.PrintJSON(map[string]interface{}{
			"removed": removed,
			"dry_run": *dryRun,
		})
	} else {
		cfg.Print(cfg.Green(fmt.Sprintf("Unapplied: %d files removed.", removed)))
	}
	return nil
}
