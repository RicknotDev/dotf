package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codebuff/dotf/internal/backup"
	"github.com/codebuff/dotf/internal/lock"
	"github.com/codebuff/dotf/internal/transaction"
)

// Restore runs the restore command.
func Restore(args []string, stateDir string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	all := fs.Bool("all", false, "Restore all backed-up files")
	preview := fs.Bool("preview", false, "Show what would be restored")
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Fprint(os.Stderr, `Usage: dotf restore [options] [path...]

Restore files from DOTF backups to their original locations.

Options:
  --all        Restore all backed-up files
  --preview    Show what would be restored without doing it
  --help       Show this help message

Examples:
  dotf restore --preview              # Show all available restores
  dotf restore --all                  # Restore everything
  dotf restore .config/kitty/kitty.conf  # Restore a specific file
`)
		return nil
	}

	// Acquire lock for safety
	l, err := lock.Acquire(stateDir, 30*time.Second)
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer l.Release()

	// Start transaction for atomicity
	tx, err := transaction.New(stateDir)
	if err != nil {
		return fmt.Errorf("cannot start transaction: %w", err)
	}

	// Ensure rollback on failure
	var restoreErr error
	defer func() {
		if restoreErr != nil {
			fmt.Fprintln(os.Stderr, "Restore failed. Rolling back...")
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	backupMgr, err := backup.NewManager(filepath.Join(stateDir, "dotf", "backups"))
	if err != nil {
		return fmt.Errorf("cannot open backups: %w", err)
	}

	homeDir, _ := os.UserHomeDir()
	paths := fs.Args()

	if len(paths) == 0 && !*all {
		// Preview mode: show all available backups
		*preview = true
	}

	if *preview {
		fmt.Println("Available restores:")
		fmt.Println()

		allBackups, err := backupMgr.ListAll()
		if err != nil {
			return fmt.Errorf("cannot list backups: %w", err)
		}

		if len(allBackups) == 0 {
			fmt.Println("  (no backups found)")
			return nil
		}

		for original, backups := range allBackups {
			if len(paths) > 0 && !matchAny(original, paths) {
				continue
			}
			fmt.Printf("  %s:\n", original)
			for _, b := range backups {
				valid, _ := backupMgr.Verify(b)
				status := "✓"
				if !valid {
					status = "✗ CORRUPTED"
				}
				fmt.Printf("    [%s] %s %s\n", b.Created, status, filepath.Base(b.BackupPath))
			}
		}
		return nil
	}

	if *all {
		// Restore all
		allBackups, err := backupMgr.ListAll()
		if err != nil {
			return fmt.Errorf("cannot list backups: %w", err)
		}

		restored := 0
		for original, backups := range allBackups {
			if len(backups) == 0 {
				continue
			}
			// Restore from the most recent backup
			b := backups[0]
			targetPath := filepath.Join(homeDir, original)

			fmt.Fprintf(os.Stderr, "  restore %s\n", original)
			if err := backupMgr.Restore(b, targetPath); err != nil {
				restoreErr = err
				fmt.Fprintf(os.Stderr, "  error: %v\n", err)
				continue
			}
			restored++
		}

		fmt.Fprintf(os.Stderr, "\nRestored %d files.\n", restored)
		if restoreErr != nil {
			return fmt.Errorf("some restores failed: %w", restoreErr)
		}
		return nil
	}

	// Restore specific files
	restored := 0
	for _, p := range paths {
		backups, err := backupMgr.List(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  no backups for %s\n", p)
			continue
		}
		if len(backups) == 0 {
			fmt.Fprintf(os.Stderr, "  no backups for %s\n", p)
			continue
		}

		b := backups[0]
		targetPath := filepath.Join(homeDir, p)
		fmt.Fprintf(os.Stderr, "  restore %s\n", p)
		if err := backupMgr.Restore(b, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			restoreErr = err
			continue
		}
		restored++
	}

	fmt.Fprintf(os.Stderr, "Restored %d files.\n", restored)
	return nil
}

// matchAny checks if a string matches any of the provided patterns.
func matchAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
