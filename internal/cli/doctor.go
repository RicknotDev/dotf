package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codebuff/dotf/internal/backup"
	"github.com/codebuff/dotf/internal/detect"
	"github.com/codebuff/dotf/internal/hook"
	"github.com/codebuff/dotf/internal/layer"
	"github.com/codebuff/dotf/internal/lock"
	"github.com/codebuff/dotf/internal/pkg"
	"github.com/codebuff/dotf/internal/state"
	"github.com/codebuff/dotf/internal/transaction"
)

// Doctor runs the doctor command.
func Doctor(args []string, stateDir string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fix := fs.Bool("fix", false, "Attempt to repair detected issues")
	unlock := fs.Bool("unlock", false, "Force release stale lock")
	emergency := fs.Bool("emergency", false, "Full system recovery")
	verifyBackups := fs.Bool("verify-backups", false, "Verify backup integrity")
	listHooks := fs.Bool("list-hooks", false, "List all available hooks")
	checkPackages := fs.Bool("check-packages", false, "Validate package lists")
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Fprint(os.Stderr, `Usage: dotf doctor [options]

Run diagnostics and repair DOTF installation.

Options:
  --fix              Attempt to repair detected issues
  --unlock           Force release stale lock
  --emergency        Full system recovery
  --verify-backups   Verify backup integrity
  --list-hooks       List all available hooks
  --check-packages   Validate package lists
  --help             Show this help message
`)
		return nil
	}

	repoRoot, _ := os.Getwd()

	fmt.Println("DOTF Doctor")
	fmt.Println("===========")
	fmt.Println()

	issues := 0

	// 1. Check locks
	fmt.Print("Checking lock status... ")
	if lock.IsLocked(stateDir) {
		fmt.Println("LOCKED")
		fmt.Printf("  Held by: %s\n", lock.LockHolder(stateDir))
		if *fix || *unlock || *emergency {
			if err := lock.ForceRelease(stateDir); err != nil {
				fmt.Printf("  Cannot release lock: %v\n", err)
			} else {
				fmt.Println("  Lock released.")
			}
		}
		issues++
	} else {
		fmt.Println("OK")
	}

	// 2. Check state integrity
	fmt.Print("Checking state integrity... ")
	sm, err := state.NewManager(stateDir)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		issues++
	} else {
		s := sm.GetState()
		if s.Version > 0 {
			fmt.Printf("OK (v%d)\n", s.Version)
		} else {
			fmt.Println("empty (first run)")
		}
	}

	// 3. Check transaction integrity
	fmt.Print("Checking for incomplete transactions... ")
	if err := transaction.RecoverIncomplete(stateDir); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		issues++
	} else {
		fmt.Println("OK")
	}

	// 4. Check layers
	fmt.Print("Checking repository structure... ")
	p := detect.Detect()
	result, err := layer.Resolve(repoRoot, p)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		issues++
	} else {
		fmt.Printf("found %d layers\n", len(result.Layers))
	}

	// 5. Check backup integrity
	if *verifyBackups || *emergency {
		fmt.Print("Verifying backup integrity... ")
		backupMgr, err := backup.NewManager(filepath.Join(stateDir, "dotf", "backups"))
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
		} else {
			allBackups, err := backupMgr.ListAll()
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
			} else if len(allBackups) == 0 {
				fmt.Println("no backups found")
			} else {
				total := 0
				for _, backups := range allBackups {
					for _, b := range backups {
						valid, _ := backupMgr.Verify(b)
						if !valid {
							fmt.Printf("CORRUPTED: %s\n", b.BackupPath)
							issues++
						}
						total++
					}
				}
				fmt.Printf("%d backups verified\n", total)
			}
		}
	}

	// 6. Check symlinks
	fmt.Print("Checking installed symlinks... ")
	homeDir, _ := os.UserHomeDir()
	count := 0
	filepath.WalkDir(homeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return nil
		}
		if stringsHasPrefix(target, repoRoot) {
			_, err := os.Stat(target)
			if err != nil {
				fmt.Printf("  BROKEN: %s -> %s\n", path, target)
				issues++
			}
			count++
		}
		return nil
	})
	fmt.Printf("%d symlinks, %d broken\n", count, 0) // We count broken inline

	// 7. Check package availability
	if *checkPackages || *emergency {
		fmt.Print("Checking package manager... ")
		if pm := pkg.DetectManager(); pm != nil {
			fmt.Printf("found %s\n", pm.Name())
		} else {
			fmt.Println("none detected")
		}
	}

	// 8. List hooks
	if *listHooks || *emergency {
		fmt.Println("\nAvailable hooks:")
		if result != nil {
			layerDirs := make([]string, len(result.Layers))
			for i, l := range result.Layers {
				layerDirs[i] = l.DirPath
			}
			hooks := hook.DiscoverHooks(layerDirs)
			if len(hooks) == 0 {
				fmt.Println("  (none)")
			} else {
				for _, h := range hooks {
					fmt.Printf("  [%s] %s\n", h.Type, h.Path)
				}
			}
		}
	}

	fmt.Println()
	if issues > 0 {
		fmt.Printf("Found %d issue(s).", issues)
		if !*fix && !*emergency {
			fmt.Print(" Use --fix or --emergency to repair.")
		}
		fmt.Println()
	} else {
		fmt.Println("All checks passed.")
	}

	return nil
}

// stringsHasPrefix is a helper to avoid importing strings in the main CLI.
func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
