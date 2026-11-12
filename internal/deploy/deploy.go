// Package deploy is the single engine behind `dotf install` and `dotf apply`:
//
//  1. Collect: walk the active layers and pick, for every path relative to
//     $HOME, the file from the highest-priority layer. The special
//     directories hooks/, packages/ and secrets/ (and .git) at the root of a
//     layer are never deployed as files; secrets/ provides encrypted files.
//  2. Plan: compare each file with what is in $HOME and in the state to
//     decide whether to create, replace (with a backup), leave unchanged,
//     skip, or remove a file that is no longer in the repository.
//  3. Execute: apply the plan inside one transaction, so a failure rolls
//     every change back.
//
// Planning never writes to disk, which is what dry runs use.
package deploy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RicknotDev/dotf/internal/backup"
	"github.com/RicknotDev/dotf/internal/layer"
	"github.com/RicknotDev/dotf/internal/safety"
	"github.com/RicknotDev/dotf/internal/secret"
	"github.com/RicknotDev/dotf/internal/state"
	"github.com/RicknotDev/dotf/internal/transaction"
)

// Mode selects how regular files are placed.
type Mode int

const (
	Symlink Mode = iota
	Copy
)

// Kind of a deployed item.
type Kind string

const (
	KindFile   Kind = "file"
	KindSecret Kind = "secret"
)

// Entry is a file chosen from the layers.
type Entry struct {
	Rel        string   // path relative to $HOME
	Source     string   // absolute path in the repository
	Layer      string   // winning layer path
	Kind       Kind     // file or secret
	Overridden []string // lower-priority layers that also provide Rel
	secret     secret.Secret
}

// Action is what the plan does with an item.
type Action string

const (
	ActCreate    Action = "create"    // nothing at the target yet
	ActReplace   Action = "replace"   // target exists and is backed up first
	ActUpdate    Action = "update"    // target is ours (from a previous run) and is replaced without backup
	ActUnchanged Action = "unchanged" // target already is what we would install
	ActSkip      Action = "skip"      // cannot be installed (see Reason)
	ActRemove    Action = "remove"    // installed earlier, no longer in the repository
)

// Item is one step of a plan.
type Item struct {
	Entry
	Target string
	Action Action
	Reason string // for skips and removals
}

// Options configure planning and execution.
type Options struct {
	RepoRoot  string
	HomeDir   string
	StateDir  string
	Layers    []layer.ResolvedLayer
	Mode      Mode
	NoSecrets bool
	// Prune removes files that an earlier run installed and that are no
	// longer provided by any active layer.
	Prune bool
	// Abort, when closed, stops execution and rolls back.
	Abort <-chan struct{}
	// Log receives one line per change; may be nil.
	Log io.Writer
}

// Plan is the result of planning.
type Plan struct {
	Items []Item
}

// Count returns how many items have the given action.
func (p *Plan) Count(a Action) int {
	n := 0
	for _, it := range p.Items {
		if it.Action == a {
			n++
		}
	}
	return n
}

// Conflicts returns the items that would replace a file not managed by DOTF.
func (p *Plan) Conflicts() []Item {
	var out []Item
	for _, it := range p.Items {
		if it.Action == ActReplace {
			out = append(out, it)
		}
	}
	return out
}

// Collect returns the files provided by the layers (lowest priority first),
// keyed by path relative to $HOME, with the highest-priority layer winning.
func Collect(layers []layer.ResolvedLayer, includeSecrets bool) (map[string]*Entry, error) {
	files := make(map[string]*Entry)
	put := func(e *Entry) {
		if prev, ok := files[e.Rel]; ok {
			e.Overridden = append(append([]string{}, prev.Overridden...), prev.Layer)
		}
		files[e.Rel] = e
	}
	for _, l := range layers {
		root := l.DirPath
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if layer.Reserved(rel, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			put(&Entry{Rel: rel, Source: path, Layer: l.Path(), Kind: KindFile})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("reading layer %s: %w", l.Path(), err)
		}
		if includeSecrets {
			for _, s := range secret.Discover(l.DirPath, l.Path()) {
				put(&Entry{Rel: s.Name, Source: s.Encrypted, Layer: l.Path(), Kind: KindSecret, secret: s})
			}
		}
	}
	return files, nil
}

// BuildPlan decides what to do with every file. It does not write anything.
func BuildPlan(opts Options, sm *state.Manager) (*Plan, error) {
	files, err := Collect(opts.Layers, !opts.NoSecrets)
	if err != nil {
		return nil, err
	}
	rels := make([]string, 0, len(files))
	for r := range files {
		rels = append(rels, r)
	}
	sort.Strings(rels)

	plan := &Plan{}
	for _, rel := range rels {
		e := files[rel]
		it := Item{Entry: *e}

		if src := safety.ValidateLayerFile(opts.RepoRoot, e.Source); !src.Safe {
			it.Action, it.Reason = ActSkip, src.Reason
			plan.Items = append(plan.Items, it)
			continue
		}
		tgt := safety.ValidateTargetPath(opts.HomeDir, rel)
		if !tgt.Safe {
			it.Action, it.Reason = ActSkip, tgt.Reason
			plan.Items = append(plan.Items, it)
			continue
		}
		it.Target = tgt.Normalized
		if reason := parentProblem(opts.HomeDir, it.Target); reason != "" {
			it.Action, it.Reason = ActSkip, reason
			plan.Items = append(plan.Items, it)
			continue
		}
		it.Action, it.Reason = decide(it, opts.Mode, sm)
		plan.Items = append(plan.Items, it)
	}

	if opts.Prune && sm != nil {
		for _, rel := range sm.Files() {
			if _, ok := files[rel]; ok {
				continue
			}
			ref, _ := sm.File(rel)
			target := filepath.Join(opts.HomeDir, rel)
			it := Item{
				Entry:  Entry{Rel: rel, Source: ref.Source, Layer: ref.Layer, Kind: KindFile},
				Target: target,
			}
			if ref.Type == state.TypeSecret {
				it.Kind = KindSecret
			}
			if owned(target, ref) {
				it.Action, it.Reason = ActRemove, "no longer in the repository"
			} else {
				// Gone, or changed by the user: just forget it.
				it.Action, it.Reason = ActRemove, "no longer in the repository (left in place: not managed by dotf any more)"
				it.Target = ""
			}
			plan.Items = append(plan.Items, it)
		}
	}
	return plan, nil
}

// parentProblem reports a parent of target (below home) that is not a
// directory, e.g. a file where a directory is needed, or a symlink.
func parentProblem(home, target string) string {
	home = filepath.Clean(home)
	for d := filepath.Dir(target); d != home && strings.HasPrefix(d, home+string(filepath.Separator)); d = filepath.Dir(d) {
		info, err := os.Lstat(d)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err.Error()
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if st, err := os.Stat(d); err == nil && st.IsDir() {
				continue // symlinked directory (e.g. ~/.config -> elsewhere) is fine
			}
			return fmt.Sprintf("%s is a symlink to a non-directory", d)
		}
		if !info.IsDir() {
			return fmt.Sprintf("%s exists and is not a directory", d)
		}
	}
	return ""
}

func decide(it Item, mode Mode, sm *state.Manager) (Action, string) {
	info, err := os.Lstat(it.Target)
	if os.IsNotExist(err) {
		return ActCreate, ""
	}
	if err != nil {
		return ActSkip, err.Error()
	}
	if info.IsDir() {
		return ActSkip, "target is a directory"
	}
	var ref state.FileRef
	var known bool
	if sm != nil {
		ref, known = sm.File(it.Rel)
	}
	isLink := info.Mode()&os.ModeSymlink != 0

	switch {
	case it.Kind == KindSecret:
		if !isLink && known && ref.Type == state.TypeSecret &&
			state.ComputeChecksum(it.Target) == ref.Checksum {
			if ref.Source == it.Source && state.ComputeChecksum(it.Source) == ref.SourceChecksum {
				return ActUnchanged, ""
			}
			return ActUpdate, ""
		}
	case mode == Symlink:
		if isLink {
			if dest, _ := os.Readlink(it.Target); dest == it.Source {
				return ActUnchanged, ""
			}
		}
	case mode == Copy:
		if !isLink && state.ComputeChecksum(it.Target) == state.ComputeChecksum(it.Source) {
			return ActUnchanged, ""
		}
	}
	if known && owned(it.Target, ref) {
		return ActUpdate, ""
	}
	return ActReplace, ""
}

// owned reports whether target is still exactly what dotf installed.
func owned(target string, ref state.FileRef) bool {
	info, err := os.Lstat(target)
	if err != nil {
		return false
	}
	if ref.Type == state.TypeSymlink {
		if info.Mode()&os.ModeSymlink == 0 {
			return false
		}
		dest, err := os.Readlink(target)
		return err == nil && dest == ref.Source
	}
	if !info.Mode().IsRegular() || ref.Checksum == "" {
		return false
	}
	return state.ComputeChecksum(target) == ref.Checksum
}

// Result summarizes an execution.
type Result struct {
	Created   int
	Replaced  int
	Updated   int
	Unchanged int
	Removed   int
	BackedUp  int
	Skipped   []Item
	Errors    []string // per-file problems that did not abort the run
}

// ErrAborted is returned when the Abort channel is closed.
var ErrAborted = errors.New("aborted by signal")

// Execute applies the plan. Per-file problems that leave the target
// untouched (an unsafe path, a backup that cannot be made) are collected in
// Result.Errors; any failure while changing $HOME rolls back the whole
// transaction and is returned as an error. On success the state manager is
// updated (but not saved).
func Execute(opts Options, plan *Plan, sm *state.Manager, bm *backup.Manager) (*Result, error) {
	res := &Result{}
	tx, err := transaction.New(opts.StateDir)
	if err != nil {
		return res, err
	}
	logf := func(format string, args ...interface{}) {
		if opts.Log != nil {
			fmt.Fprintf(opts.Log, format, args...)
		}
	}
	fail := func(err error) (*Result, error) {
		if rbErr := tx.Rollback(); rbErr != nil {
			return res, fmt.Errorf("%w; rollback incomplete (run 'dotf doctor --fix'): %v", err, rbErr)
		}
		return res, fmt.Errorf("%w (all changes rolled back)", err)
	}

	type recorded struct {
		rel string
		ref *state.FileRef // nil: forget
	}
	var records []recorded

	for _, it := range plan.Items {
		select {
		case <-opts.Abort:
			return fail(ErrAborted)
		default:
		}

		switch it.Action {
		case ActSkip:
			res.Skipped = append(res.Skipped, it)
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", it.Rel, it.Reason))
			continue
		case ActUnchanged:
			res.Unchanged++
			records = append(records, recorded{it.Rel, refFor(it, opts.Mode, sm)})
			continue
		case ActRemove:
			if it.Target != "" {
				if err := tx.Remove(it.Target); err != nil {
					return fail(fmt.Errorf("remove %s: %w", it.Rel, err))
				}
				logf("  remove  %s (%s)\n", it.Rel, it.Reason)
				res.Removed++
			}
			records = append(records, recorded{it.Rel, nil})
			continue
		}

		if it.Action == ActReplace {
			b, err := bm.Create(it.Rel, it.Target)
			if err != nil {
				// Without a backup the existing file must not be touched.
				res.Errors = append(res.Errors, fmt.Sprintf("%s: backup failed, file left unchanged: %v", it.Rel, err))
				continue
			}
			if b != nil {
				res.BackedUp++
				sm.RecordBackup(b.BackupPath, it.Rel)
				logf("  backup  %s\n", it.Rel)
			}
		}

		if err := tx.MkdirAll(filepath.Dir(it.Target), 0o755); err != nil {
			return fail(fmt.Errorf("mkdir for %s: %w", it.Rel, err))
		}

		var ref state.FileRef
		switch {
		case it.Kind == KindSecret:
			if err := tx.WriteFile(it.Target, 0o600, func(w io.Writer) error {
				return secret.Decrypt(it.secret, w)
			}); err != nil {
				return fail(fmt.Errorf("secret %s: %w", it.Rel, err))
			}
			ref = secretRef(it)
		case opts.Mode == Copy:
			if err := tx.Copy(it.Source, it.Target); err != nil {
				return fail(fmt.Errorf("copy %s: %w", it.Rel, err))
			}
			ref = copyRef(it)
		default:
			if err := tx.Symlink(it.Source, it.Target); err != nil {
				return fail(fmt.Errorf("link %s: %w", it.Rel, err))
			}
			ref = state.FileRef{Layer: it.Layer, Type: state.TypeSymlink, Source: it.Source}
		}
		records = append(records, recorded{it.Rel, &ref})

		switch it.Action {
		case ActCreate:
			res.Created++
		case ActReplace:
			res.Replaced++
		case ActUpdate:
			res.Updated++
		}
		logf("  %-7s %s <- %s\n", it.Action, it.Rel, it.Layer)
	}

	if err := tx.Commit(); err != nil {
		return fail(fmt.Errorf("cannot commit transaction: %w", err))
	}
	for _, r := range records {
		if r.ref == nil {
			sm.RemoveFile(r.rel)
		} else {
			sm.SetFile(r.rel, *r.ref)
		}
	}
	return res, nil
}

// refFor builds the state record of an unchanged item (keeping what the
// state already knows).
func refFor(it Item, mode Mode, sm *state.Manager) *state.FileRef {
	if ref, ok := sm.File(it.Rel); ok && ref.Source == it.Source {
		ref.Layer = it.Layer
		return &ref
	}
	var ref state.FileRef
	switch {
	case it.Kind == KindSecret:
		ref = secretRef(it)
	case mode == Copy:
		ref = copyRef(it)
	default:
		ref = state.FileRef{Layer: it.Layer, Type: state.TypeSymlink, Source: it.Source}
	}
	return &ref
}

func secretRef(it Item) state.FileRef {
	return state.FileRef{
		Layer:          it.Layer,
		Type:           state.TypeSecret,
		Source:         it.Source,
		Checksum:       state.ComputeChecksum(it.Target),
		SourceChecksum: state.ComputeChecksum(it.Source),
	}
}

func copyRef(it Item) state.FileRef {
	return state.FileRef{
		Layer:    it.Layer,
		Type:     state.TypeCopy,
		Source:   it.Source,
		Checksum: state.ComputeChecksum(it.Target),
	}
}
