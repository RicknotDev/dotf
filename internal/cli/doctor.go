package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RicknotDev/dotf/internal/backup"
	"github.com/RicknotDev/dotf/internal/hook"
	"github.com/RicknotDev/dotf/internal/lock"
	"github.com/RicknotDev/dotf/internal/pkg"
	"github.com/RicknotDev/dotf/internal/safety"
	"github.com/RicknotDev/dotf/internal/state"
	"github.com/RicknotDev/dotf/internal/transaction"
)

const doctorHelp = `Usage: dotf doctor [options]

Check the DOTF installation. Without --fix nothing is changed.

Options:
  --fix              Repair: roll back interrupted transactions, remove a
                     leftover lock file, recreate missing links and copies
                     whose source still exists
  --unlock           Only remove a leftover lock file (refused while a dotf
                     process is running)
  --emergency        --fix plus every optional check below
  --verify-backups   Verify backup checksums
  --list-hooks       List the hooks of the active layers
  --check-packages   Show the package manager and package lists in use
  --help             Show this help message
`

type doctorCheck struct {
	Name   string   `json:"name"`
	Status string   `json:"status"` // ok, issue, fixed, info
	Detail string   `json:"detail,omitempty"`
	Items  []string `json:"items,omitempty"`
}

type doctorReport struct {
	cfg    OutputConfig
	checks []doctorCheck
	issues int
}

// add records a check; text mode prints "<title>... <detail>".
func (r *doctorReport) add(c doctorCheck, title string) {
	if c.Status == "issue" {
		r.issues++
	}
	r.checks = append(r.checks, c)
	r.cfg.Printf("%s... %s\n", title, c.Detail)
	for _, it := range c.Items {
		r.cfg.Printf("  %s\n", it)
	}
}

// Doctor runs the doctor command.
func Doctor(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("doctor")
	fix := fs.Bool("fix", false, "")
	unlock := fs.Bool("unlock", false, "")
	emergency := fs.Bool("emergency", false, "")
	verifyBackups := fs.Bool("verify-backups", false, "")
	listHooks := fs.Bool("list-hooks", false, "")
	checkPackages := fs.Bool("check-packages", false, "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), doctorHelp)
		return nil
	}
	if *emergency {
		*fix, *verifyBackups, *listHooks, *checkPackages = true, true, true, true
	}

	r := &doctorReport{cfg: cfg}
	cfg.Print("DOTF Doctor")
	cfg.Print("===========")
	cfg.Print("")

	// 1. Lock
	lockPath := filepath.Join(stateDir, lock.LockFile)
	_, lockFileErr := os.Stat(lockPath)
	switch {
	case lock.IsLocked(stateDir):
		r.add(doctorCheck{Name: "lock", Status: "issue",
			Detail: "LOCKED by a running dotf process (" + lock.LockHolder(stateDir) + ")"}, "Checking lock status")
	case lockFileErr == nil && (*fix || *unlock):
		if err := lock.ForceRelease(stateDir); err != nil {
			r.add(doctorCheck{Name: "lock", Status: "issue", Detail: "cannot remove leftover lock file: " + err.Error()}, "Checking lock status")
		} else {
			r.add(doctorCheck{Name: "lock", Status: "fixed", Detail: "Stale lock released (no process held it)"}, "Checking lock status")
		}
	case lockFileErr == nil:
		r.add(doctorCheck{Name: "lock", Status: "ok", Detail: "OK (leftover lock file, not held by any process)"}, "Checking lock status")
	default:
		r.add(doctorCheck{Name: "lock", Status: "ok", Detail: "OK"}, "Checking lock status")
	}

	// 2. Repairs that need the lock
	var sess *session
	if *fix {
		s, err := startSession(stateDir, cfg)
		if err != nil {
			r.add(doctorCheck{Name: "transactions", Status: "issue", Detail: "cannot repair: " + err.Error()}, "Checking for incomplete transactions")
		} else {
			sess = s
			defer sess.end()
			r.add(doctorCheck{Name: "transactions", Status: "ok", Detail: "OK (any interrupted transaction was rolled back)"}, "Checking for incomplete transactions")
		}
	} else if pending := transaction.Pending(stateDir); len(pending) > 0 {
		r.add(doctorCheck{Name: "transactions", Status: "issue",
			Detail: fmt.Sprintf("%d interrupted (run 'dotf doctor --fix' to roll back)", len(pending)), Items: pending}, "Checking for incomplete transactions")
	} else {
		r.add(doctorCheck{Name: "transactions", Status: "ok", Detail: "OK"}, "Checking for incomplete transactions")
	}

	// 3. State
	sm, err := state.NewManager(stateDir)
	if err != nil {
		r.add(doctorCheck{Name: "state", Status: "issue", Detail: "ERROR: " + err.Error()}, "Checking state integrity")
	} else if w := sm.Warning(); w != "" {
		r.add(doctorCheck{Name: "state", Status: "issue", Detail: w}, "Checking state integrity")
	} else {
		st := sm.GetState()
		detail := "empty (nothing installed yet)"
		if len(st.InstalledFiles) > 0 {
			detail = fmt.Sprintf("OK (v%d, %s)", st.Version, plural(len(st.InstalledFiles), "file"))
		}
		r.add(doctorCheck{Name: "state", Status: "ok", Detail: detail}, "Checking state integrity")
	}

	// 4. Repository
	repoRoot, repoOK, _ := findRepo(stateDir)
	var layerDirs []string
	if !repoOK {
		r.add(doctorCheck{Name: "repository", Status: "info", Detail: "no DOTF repository here and none recorded"}, "Checking repository structure")
	} else if sel, err := selectLayers(repoRoot, ""); err != nil {
		r.add(doctorCheck{Name: "repository", Status: "issue", Detail: "ERROR: " + err.Error()}, "Checking repository structure")
	} else {
		for _, l := range sel.Result.Layers {
			layerDirs = append(layerDirs, l.DirPath)
		}
		r.add(doctorCheck{Name: "repository", Status: "ok",
			Detail: fmt.Sprintf("%s, found %s", repoRoot, plural(len(sel.Result.Layers), "active layer"))}, "Checking repository structure")
	}

	// 5. Backups
	if *verifyBackups {
		bm, err := backup.NewManager(filepath.Join(stateDir, "backups"))
		var groups map[string][]backup.Backup
		if err == nil {
			groups, err = bm.ListAll()
		}
		if err != nil {
			r.add(doctorCheck{Name: "backups", Status: "issue", Detail: "ERROR: " + err.Error()}, "Verifying backup integrity")
		} else {
			total, bad := 0, []string{}
			for _, list := range groups {
				for _, b := range list {
					total++
					if ok, _ := bm.Verify(b); !ok {
						bad = append(bad, "CORRUPTED: "+b.BackupPath)
					}
				}
			}
			c := doctorCheck{Name: "backups", Status: "ok", Detail: fmt.Sprintf("%d backups verified", total), Items: bad}
			if len(bad) > 0 {
				c.Status = "issue"
				c.Detail = fmt.Sprintf("%d backups verified, %d corrupted", total, len(bad))
			}
			r.add(c, "Verifying backup integrity")
		}
	}

	// 6. Installed files
	if sm != nil {
		home, herr := homeDir()
		if herr != nil {
			r.add(doctorCheck{Name: "files", Status: "issue", Detail: herr.Error()}, "Checking installed files")
		} else {
			r.add(checkInstalled(home, stateDir, sm, sess), "Checking installed files")
		}
	}

	// 7. Packages
	if *checkPackages {
		c := doctorCheck{Name: "packages", Status: "info", Detail: "no supported package manager found"}
		if pm := pkg.DetectManager(); pm != nil {
			used, ignored := pm.ListFiles(layerDirs)
			c.Detail = fmt.Sprintf("%s; %d list(s) used, %d ignored", pm.Name(), len(used), len(ignored))
			c.Items = used
			for _, f := range used {
				if _, err := pkg.LoadPackages(f); err != nil {
					c.Status = "issue"
					c.Items = append(c.Items, "ERROR: "+err.Error())
				}
			}
		}
		r.add(c, "Checking package manager")
	}

	// 8. Hooks
	if *listHooks {
		c := doctorCheck{Name: "hooks", Status: "info", Detail: "(none)"}
		hooks := hook.DiscoverHooks(layerDirs)
		if len(hooks) > 0 {
			c.Detail = plural(len(hooks), "hook")
		}
		for _, h := range hooks {
			c.Items = append(c.Items, fmt.Sprintf("[%s] %s", h.Type, h.Path))
		}
		r.add(c, "Available hooks")
	}

	if cfg.JSON {
		cfg.PrintJSON(map[string]interface{}{"checks": r.checks, "issues": r.issues})
	}
	cfg.Print("")
	if r.issues > 0 {
		hint := ""
		if !*fix {
			hint = " Use --fix to repair."
		}
		cfg.Printf("Found %s.%s\n", plural(r.issues, "issue"), hint)
		return fmt.Errorf("%s found", plural(r.issues, "issue"))
	}
	cfg.Print("All checks passed.")
	return nil
}

// checkInstalled checks the files recorded in the state; with a session
// (--fix) it recreates missing links and copies whose source still exists.
func checkInstalled(home, stateDir string, sm *state.Manager, sess *session) doctorCheck {
	files := sm.Files()
	c := doctorCheck{Name: "files", Status: "ok"}
	var tx *transaction.Transaction
	if sess != nil {
		tx, _ = transaction.New(stateDir)
	}
	fixed, problems := 0, 0
	for _, rel := range files {
		ref, _ := sm.File(rel)
		e := checkFile(home, rel, ref)
		if e.Status == StatusOK {
			continue
		}
		_, srcErr := os.Stat(ref.Source)
		canFix := e.Status == StatusMissing && srcErr == nil && ref.Type != state.TypeSecret
		if canFix && tx != nil {
			tgt := safety.ValidateTargetPath(home, rel)
			var err error
			if !tgt.Safe {
				err = fmt.Errorf("%s", tgt.Reason)
			} else if err = tx.MkdirAll(filepath.Dir(tgt.Normalized), 0o755); err == nil {
				if ref.Type == state.TypeCopy {
					err = tx.Copy(ref.Source, tgt.Normalized)
					if err == nil {
						ref.Checksum = state.ComputeChecksum(tgt.Normalized)
						sm.SetFile(rel, ref)
					}
				} else {
					err = tx.Symlink(ref.Source, tgt.Normalized)
				}
			}
			if err == nil {
				fixed++
				c.Items = append(c.Items, "RECREATED: "+rel)
				continue
			}
			c.Items = append(c.Items, fmt.Sprintf("CANNOT FIX: %s: %v", rel, err))
			problems++
			continue
		}
		hint := ""
		if canFix {
			hint = " (fixable with --fix)"
		}
		c.Items = append(c.Items, fmt.Sprintf("%s: %s (%s)%s", strings.ToUpper(string(e.Status)), rel, e.Detail, hint))
		problems++
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			c.Items = append(c.Items, "ERROR: "+err.Error())
			problems++
		} else if fixed > 0 {
			if err := sm.Save(); err != nil {
				c.Items = append(c.Items, "ERROR: "+err.Error())
			}
		}
	}
	c.Detail = fmt.Sprintf("%s, %d with problems", plural(len(files), "file"), problems)
	if fixed > 0 {
		c.Detail += fmt.Sprintf(", %d recreated", fixed)
		c.Status = "fixed"
	}
	if problems > 0 {
		c.Status = "issue"
	}
	return c
}
