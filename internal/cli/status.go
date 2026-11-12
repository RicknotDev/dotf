package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RicknotDev/dotf/internal/state"
)

const statusHelp = `Usage: dotf status [options]

Show the state of every file dotf installed. Exits with 1 when any file has
a problem (also with --json), so it can be used in scripts.

Options:
  --short     Show only entries that are not OK
  --json      Output in JSON format (global flag)
  --filter    Filter entries: a value matched against every field, or
              key:value with key one of status, path, layer, type
              (global flag, e.g. --filter status:broken)
  --quiet     Print nothing; only the exit code (global flag)
  --help      Show this help message
`

// Status runs the status command.
func Status(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("status")
	short := fs.Bool("short", false, "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), statusHelp)
		return nil
	}

	home, err := homeDir()
	if err != nil {
		return err
	}
	sm, err := state.NewManager(stateDir)
	if err != nil {
		return err
	}
	s := sm.GetState()

	if len(s.InstalledFiles) == 0 {
		if cfg.JSON {
			cfg.PrintJSON(StatusResult{Version: s.Version, Entries: []StatusEntry{}})
			return nil
		}
		cfg.Print("No files installed. Run 'dotf install' to install dotfiles.")
		return nothingToDo("nothing to do: no installed files")
	}

	all := make([]StatusEntry, 0, len(s.InstalledFiles))
	for _, rel := range sm.Files() {
		ref, _ := sm.File(rel)
		all = append(all, checkFile(home, rel, ref))
	}

	summary := StatusSummary{Total: len(all)}
	for _, e := range all {
		switch e.Status {
		case StatusOK:
			summary.OK++
		case StatusBroken:
			summary.Broken++
		case StatusMissing:
			summary.Missing++
		}
	}
	summary.Issues = summary.Total - summary.OK

	entries := []StatusEntry{}
	for _, e := range all {
		if *short && e.Status == StatusOK {
			continue
		}
		ok, err := cfg.matchFields(map[string]string{
			"status": string(e.Status),
			"path":   e.Path,
			"layer":  e.Layer,
			"type":   e.Type,
		})
		if err != nil {
			return err
		}
		if ok {
			entries = append(entries, e)
		}
	}

	var issuesErr error
	if summary.Issues > 0 {
		issuesErr = fmt.Errorf("%d file(s) have issues", summary.Issues)
	}

	if cfg.JSON {
		cfg.PrintJSON(StatusResult{Version: s.Version, Entries: entries, Summary: summary})
		return issuesErr
	}

	line := fmt.Sprintf("Status: %s, %d OK, %d with issues", plural(summary.Total, "file"), summary.OK, summary.Issues)
	if summary.Issues > 0 {
		cfg.Print(cfg.Red(line))
	} else {
		cfg.Print(cfg.Green(line))
	}
	for _, e := range entries {
		var st string
		switch e.Status {
		case StatusOK:
			st = cfg.Green(string(e.Status))
		case StatusBroken, StatusError:
			st = cfg.Red(string(e.Status))
		default:
			st = cfg.Yellow(string(e.Status))
		}
		out := fmt.Sprintf("  %s %s [%s]", st, e.Path, e.Layer)
		if e.Detail != "" {
			out += " (" + e.Detail + ")"
		}
		cfg.Print(out)
	}
	return issuesErr
}

// checkFile compares an installed file with its state record.
func checkFile(home, rel string, ref state.FileRef) StatusEntry {
	e := StatusEntry{Path: rel, Layer: ref.Layer, Type: ref.Type}
	target := filepath.Join(home, rel)
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			e.Status, e.Detail = StatusMissing, "file does not exist"
		} else {
			e.Status, e.Detail = StatusError, err.Error()
		}
		return e
	}

	isLink := info.Mode()&os.ModeSymlink != 0
	if isLink {
		e.Target, _ = os.Readlink(target)
	}
	switch ref.Type {
	case state.TypeCopy, state.TypeSecret:
		switch {
		case isLink || !info.Mode().IsRegular():
			e.Status, e.Detail = StatusConflict, "expected a regular file"
		case ref.Checksum != "" && state.ComputeChecksum(target) != ref.Checksum:
			e.Status, e.Detail = StatusConflict, "modified since it was installed"
		default:
			e.Status = StatusOK
		}
	default:
		switch {
		case !isLink:
			e.Status, e.Detail = StatusConflict, "expected symlink but found regular file"
		case e.Target != ref.Source:
			e.Status, e.Detail = StatusConflict, "symlink points to "+e.Target
		default:
			if _, err := os.Stat(target); err != nil {
				e.Status, e.Detail = StatusBroken, "symlink target missing"
			} else {
				e.Status = StatusOK
			}
		}
	}
	return e
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
