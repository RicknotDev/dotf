package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codebuff/dotf/internal/state"
)

// Status runs the status command.
func Status(args []string, stateDir string, cfg OutputConfig) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	short := fs.Bool("short", false, "Show only non-OK entries")
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		cfg.Print(`Usage: dotf status [options]

Show the installation status of your dotfiles.

Options:
  --short     Show only entries that are not OK
  --json      Output in JSON format (global flag)
  --filter    Filter by profile/state (global flag)
  --quiet     Suppress non-error output (global flag)
  --help      Show this help message

Examples:
  dotf status              # Show all status
  dotf status --short      # Show only issues
  dotf status --json       # Machine-readable output
  dotf status --filter ok  # Show only OK entries
`)
		return nil
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	sm, err := state.NewManager(stateDir)
	if err != nil {
		return fmt.Errorf("cannot open state: %w", err)
	}

	s := sm.GetState()

	if len(s.InstalledFiles) == 0 {
		if cfg.JSON {
			cfg.PrintJSON(StatusResult{
				Version: s.Version,
				Entries: []StatusEntry{},
				Summary: StatusSummary{Total: 0, OK: 0, Issues: 0},
			})
			return nil
		}
		if !cfg.Quiet {
			cfg.Print("No files installed. Run 'dotf install' to install dotfiles.")
		}
		return fmt.Errorf("nothing to do: no installed files")
	}

	var entries []StatusEntry
	okCount := 0
	issueCount := 0

	for relPath, ref := range s.InstalledFiles {
		targetPath := filepath.Join(homeDir, relPath)

		entry := StatusEntry{
			Path:  relPath,
			Layer: ref.Layer,
		}

		info, lerr := os.Lstat(targetPath)
		if lerr != nil {
			if os.IsNotExist(lerr) {
				entry.Status = StatusMissing
				entry.Detail = "file does not exist"
			} else {
				entry.Status = StatusError
				entry.Detail = lerr.Error()
			}
			issueCount++
			entries = append(entries, entry)
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			entry.Type = "symlink"
			target, rerr := os.Readlink(targetPath)
			if rerr != nil {
				entry.Status = StatusError
				entry.Detail = fmt.Sprintf("cannot read symlink: %v", rerr)
				issueCount++
				entries = append(entries, entry)
				continue
			}
			entry.Target = target

			if target == ref.Source {
				// Check if source still exists
				if _, serr := os.Stat(ref.Source); serr != nil {
					entry.Status = StatusBroken
					entry.Detail = "symlink target missing"
					issueCount++
				} else {
					entry.Status = StatusOK
					okCount++
				}
			} else if strings.HasPrefix(target, repoRoot) {
				// Points into repo but different location
				if _, serr := os.Stat(target); serr != nil {
					entry.Status = StatusBroken
					entry.Detail = "symlink target missing"
					issueCount++
				} else {
					entry.Status = StatusConflict
					entry.Detail = "symlink points to different location"
					issueCount++
				}
			} else {
				entry.Status = StatusConflict
				entry.Detail = "symlink points outside repository"
				issueCount++
			}
		} else {
			entry.Type = "file"
			// Regular file — this is expected for copy mode, conflict for symlink mode
			if ref.Type == "copy" {
				entry.Status = StatusOK
				okCount++
			} else {
				entry.Status = StatusConflict
				entry.Detail = "expected symlink but found regular file"
				issueCount++
			}
		}

		entries = append(entries, entry)
	}

	// Apply filter
	cfg.Filter = strings.ToLower(cfg.Filter)
	var filtered []StatusEntry
	for _, e := range entries {
		if cfg.Filter != "" {
			if *short {
				// In short mode, filter only shows non-OK entries matching the filter
				if e.Status == StatusOK {
					continue
				}
				if !cfg.matchesFilter(string(e.Status)) && !cfg.matchesFilter(e.Path) {
					continue
				}
			} else {
				if !cfg.matchesFilter(string(e.Status)) && !cfg.matchesFilter(e.Path) && !cfg.matchesFilter(e.Layer) {
					continue
				}
			}
		} else if *short && e.Status == StatusOK {
			continue
		}
		filtered = append(filtered, e)
	}
	entries = filtered

	if cfg.JSON {
		cfg.PrintJSON(StatusResult{
			Version: s.Version,
			Entries: entries,
			Summary: StatusSummary{
				Total:   len(s.InstalledFiles),
				OK:      okCount,
				Issues:  issueCount,
				Broken:  countByStatus(entries, StatusBroken),
				Missing: countByStatus(entries, StatusMissing),
			},
		})
		return nil
	}

	// Print summary
	summary := fmt.Sprintf("Status: %d files, %d OK, %d issues",
		len(s.InstalledFiles), okCount, issueCount)

	if issueCount > 0 {
		cfg.Print(cfg.Red(summary))
	} else {
		cfg.Print(cfg.Green(summary))
	}

	// Print entries
	for _, e := range entries {
		var statusStr string
		switch e.Status {
		case StatusOK:
			statusStr = cfg.Green(string(e.Status))
		case StatusBroken:
			statusStr = cfg.Red(string(e.Status))
		case StatusMissing:
			statusStr = cfg.Yellow(string(e.Status))
		case StatusConflict:
			statusStr = cfg.Yellow(string(e.Status))
		case StatusError:
			statusStr = cfg.Red(string(e.Status))
		default:
			statusStr = string(e.Status)
		}

		line := fmt.Sprintf("  %s %s [%s]", statusStr, e.Path, e.Layer)
		if e.Detail != "" {
			line += " (" + e.Detail + ")"
		}
		cfg.Print(line)
	}

	// Return non-zero exit code if there are issues
	if issueCount > 0 {
		return fmt.Errorf("%d file(s) have issues", issueCount)
	}

	return nil
}

func countByStatus(entries []StatusEntry, status FilterStatus) int {
	count := 0
	for _, e := range entries {
		if e.Status == status {
			count++
		}
	}
	return count
}
