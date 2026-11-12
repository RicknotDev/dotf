package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RicknotDev/dotf/internal/backup"
	"github.com/RicknotDev/dotf/internal/safety"
	"github.com/RicknotDev/dotf/internal/state"
	"github.com/RicknotDev/dotf/internal/transaction"
)

const restoreHelp = `Usage: dotf restore [options] [path...]

Restore files from DOTF backups to their original locations (relative to
your home directory). The newest backup of each file is used.

Options:
  --all        Restore every backed-up file
  --preview    Show what is available without restoring anything
  --help       Show this help message

Examples:
  dotf restore                           # Same as --preview
  dotf restore --all                     # Restore everything
  dotf restore .config/kitty/kitty.conf  # Restore one file
`

type restorePreview struct {
	Path    string          `json:"path"`
	Backups []restoreBackup `json:"backups"`
}

type restoreBackup struct {
	File    string `json:"file"`
	Created string `json:"created"`
	Valid   bool   `json:"valid"`
	Symlink bool   `json:"symlink,omitempty"`
}

// Restore runs the restore command.
func Restore(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("restore")
	all := fs.Bool("all", false, "")
	preview := fs.Bool("preview", false, "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), restoreHelp)
		return nil
	}
	paths := fs.Args()
	if *all && len(paths) > 0 {
		return usageErr("use either --all or a list of paths")
	}

	bm, err := backup.NewManager(filepath.Join(stateDir, "backups"))
	if err != nil {
		return err
	}
	if *preview || (!*all && len(paths) == 0) {
		return restorePreviewCmd(cfg, bm, paths)
	}

	home, err := homeDir()
	if err != nil {
		return err
	}
	sess, err := startSession(stateDir, cfg)
	if err != nil {
		return err
	}
	defer sess.end()

	groups, err := bm.ListAll()
	if err != nil {
		return err
	}
	if *all {
		for p := range groups {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		if len(paths) == 0 {
			cfg.Print("No backups found. Restored 0 files.")
			return nil
		}
	}

	sm, err := state.NewManager(stateDir)
	if err != nil {
		return err
	}
	tx, err := transaction.New(stateDir)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w; rollback incomplete (run 'dotf doctor --fix'): %v", err, rbErr)
		}
		return fmt.Errorf("%w (nothing was restored)", err)
	}

	var restored []string
	for _, p := range paths {
		select {
		case <-sess.abort:
			return fail(fmt.Errorf("aborted by signal"))
		default:
		}
		rel := filepath.ToSlash(filepath.Clean(p))
		backups := groups[rel]
		if len(backups) == 0 {
			return fail(fmt.Errorf("no backup of %s", rel))
		}
		tgt := safety.ValidateTargetPath(home, rel)
		if !tgt.Safe {
			return fail(fmt.Errorf("%s: %s", rel, tgt.Reason))
		}
		content, err := bm.Open(backups[0])
		if err != nil {
			return fail(fmt.Errorf("%s: %w", rel, err))
		}
		// Read it now: backing up the current file below may prune old backups.
		var data []byte
		if content.LinkTarget == "" {
			r, err := content.Open()
			if err != nil {
				return fail(fmt.Errorf("%s: %w", rel, err))
			}
			data, err = io.ReadAll(r)
			r.Close()
			if err != nil {
				return fail(fmt.Errorf("%s: %w", rel, err))
			}
		}

		// Keep whatever is there now if dotf does not own it.
		ref, managed := sm.File(rel)
		if _, err := os.Lstat(tgt.Normalized); err == nil && !(managed && ownedBy(tgt.Normalized, ref)) {
			if b, err := bm.Create(rel, tgt.Normalized); err != nil {
				return fail(fmt.Errorf("cannot back up current %s: %w", rel, err))
			} else if b != nil {
				sm.RecordBackup(b.BackupPath, rel)
			}
		}

		if err := tx.MkdirAll(filepath.Dir(tgt.Normalized), 0o755); err != nil {
			return fail(err)
		}
		if content.LinkTarget != "" {
			err = tx.Symlink(content.LinkTarget, tgt.Normalized)
		} else {
			err = tx.WriteFile(tgt.Normalized, content.Mode, func(w io.Writer) error {
				_, err := w.Write(data)
				return err
			})
		}
		if err != nil {
			return fail(fmt.Errorf("restore %s: %w", rel, err))
		}
		sm.RemoveFile(rel) // restored files are no longer managed by dotf
		restored = append(restored, rel)
		cfg.Info("  restore %s (backup from %s)\n", rel, backups[0].Time.Format("2006-01-02 15:04:05"))
	}

	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	if err := sm.Save(); err != nil {
		return fmt.Errorf("files restored but the state could not be saved: %w", err)
	}
	if cfg.JSON {
		cfg.PrintJSON(map[string]interface{}{"restored": restored})
		return nil
	}
	cfg.Print(fmt.Sprintf("Restored %s.", plural(len(restored), "file")))
	return nil
}

func restorePreviewCmd(cfg OutputConfig, bm *backup.Manager, filter []string) error {
	groups, err := bm.ListAll()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		if len(filter) == 0 || matchAny(k, filter) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	if cfg.JSON {
		out := []restorePreview{}
		for _, k := range keys {
			pr := restorePreview{Path: k}
			for _, b := range groups[k] {
				ok, _ := bm.Verify(b)
				pr.Backups = append(pr.Backups, restoreBackup{
					File:    b.BackupPath,
					Created: b.Time.Format("2006-01-02T15:04:05Z07:00"),
					Valid:   ok,
					Symlink: b.Symlink,
				})
			}
			out = append(out, pr)
		}
		cfg.PrintJSON(out)
		return nil
	}

	cfg.Print("Available restores:")
	cfg.Print("")
	if len(keys) == 0 {
		cfg.Print("  (no backups found)")
		return nil
	}
	for _, k := range keys {
		cfg.Printf("  %s:\n", k)
		for i, b := range groups[k] {
			status := "ok"
			if ok, _ := bm.Verify(b); !ok {
				status = cfg.Red("CORRUPTED")
			}
			newest := ""
			if i == 0 {
				newest = " (newest, used by restore)"
			}
			cfg.Printf("    %s  %s  %s%s\n", b.Time.Format("2006-01-02 15:04:05"), status, filepath.Base(b.BackupPath), newest)
		}
	}
	return nil
}

// ownedBy reports whether target is still what dotf installed for ref.
func ownedBy(target string, ref state.FileRef) bool {
	ok, _ := removable(target, ref)
	return ok
}

// matchAny checks if a string contains any of the provided patterns.
func matchAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
