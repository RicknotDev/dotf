package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RicknotDev/dotf/internal/safety"
	"github.com/RicknotDev/dotf/internal/state"
	"github.com/RicknotDev/dotf/internal/transaction"
)

const unapplyHelp = `Usage: dotf unapply [options]

Remove the files dotf installed: symlinks that still point to their recorded
source, and copies and secrets that were not modified since. Anything you
changed is left in place. Use 'dotf restore' afterwards to bring back the
files dotf backed up when it replaced them.

Options:
  --dry-run    Show what would be removed without removing anything
  --json       Output in JSON format (global flag)
  --help       Show this help message
`

type unapplyEntry struct {
	Path   string `json:"path"`
	Action string `json:"action"` // removed, missing, kept
	Reason string `json:"reason,omitempty"`
}

// Unapply runs the unapply command.
func Unapply(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("unapply")
	dryRun := fs.Bool("dry-run", false, "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), unapplyHelp)
		return nil
	}

	home, err := homeDir()
	if err != nil {
		return err
	}

	var sess *session
	if !*dryRun {
		if sess, err = startSession(stateDir, cfg); err != nil {
			return err
		}
		defer sess.end()
	}
	sm, err := state.NewManager(stateDir)
	if err != nil {
		return err
	}
	files := sm.Files()
	if len(files) == 0 {
		return nothingToDo("nothing to do: no installed files")
	}

	tx, err := transaction.New(stateDir)
	if err != nil {
		return err
	}
	var entries []unapplyEntry
	removed, kept := 0, 0
	for _, rel := range files {
		if sess != nil {
			select {
			case <-sess.abort:
				tx.Rollback()
				return fmt.Errorf("aborted by signal (nothing was removed)")
			default:
			}
		}
		ref, _ := sm.File(rel)
		e := unapplyEntry{Path: rel}
		tgt := safety.ValidateTargetPath(home, rel)
		target := filepath.Join(home, rel)
		if tgt.Safe {
			target = tgt.Normalized
		}
		ok, reason := removable(target, ref)
		switch {
		case !tgt.Safe:
			e.Action, e.Reason = "kept", tgt.Reason
		case reason == "missing":
			e.Action = "missing"
		case !ok:
			e.Action, e.Reason = "kept", reason
		default:
			e.Action = "removed"
		}

		if !*dryRun {
			switch e.Action {
			case "removed":
				if err := tx.Remove(target); err != nil {
					if rbErr := tx.Rollback(); rbErr != nil {
						return fmt.Errorf("cannot remove %s: %w; rollback incomplete: %v", rel, err, rbErr)
					}
					return fmt.Errorf("cannot remove %s: %w (nothing was removed)", rel, err)
				}
			case "kept":
				cfg.Warn("%s left in place: %s", rel, e.Reason)
			}
		}
		switch e.Action {
		case "removed":
			removed++
			cfg.Info("  remove %s\n", rel)
		case "kept":
			kept++
		}
		entries = append(entries, e)
	}

	if !*dryRun {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("cannot commit transaction: %w", err)
		}
		// Everything is unmanaged now, including files left in place.
		sm.ClearFiles()
		sm.RecordInstall([]string{})
		if err := sm.Save(); err != nil {
			return fmt.Errorf("files removed but the state could not be saved: %w", err)
		}
	}

	if cfg.JSON {
		cfg.PrintJSON(map[string]interface{}{
			"dry_run": *dryRun,
			"removed": removed,
			"kept":    kept,
			"entries": entries,
		})
		return nil
	}
	verb := "removed"
	if *dryRun {
		verb = "would be removed"
	}
	msg := fmt.Sprintf("Unapplied: %s %s", plural(removed, "file"), verb)
	if kept > 0 {
		msg += fmt.Sprintf(", %d left in place", kept)
	}
	cfg.Print(cfg.Green(msg + "."))
	return nil
}

// removable reports whether target is still exactly what dotf installed.
func removable(target string, ref state.FileRef) (bool, string) {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return false, "missing"
	}
	if err != nil {
		return false, err.Error()
	}
	switch ref.Type {
	case state.TypeCopy, state.TypeSecret:
		if !info.Mode().IsRegular() {
			return false, "no longer the file dotf installed"
		}
		if ref.Checksum == "" || state.ComputeChecksum(target) != ref.Checksum {
			return false, "modified since it was installed"
		}
		return true, ""
	default:
		if info.Mode()&os.ModeSymlink == 0 {
			return false, "replaced by a regular file"
		}
		dest, err := os.Readlink(target)
		if err != nil || dest != ref.Source {
			return false, fmt.Sprintf("symlink points to %s, not to the installed source", dest)
		}
		return true, ""
	}
}
