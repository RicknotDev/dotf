// Package transaction makes a group of filesystem changes all-or-nothing.
//
// Every change is recorded in an append-only journal (one JSON object per
// line) before it is made. Each entry records what was at the path before:
// nothing, a symlink (and its target), or a regular file (copied into the
// transaction's stash directory together with its mode). Rollback undoes
// every entry of the transaction in reverse order and puts back what was
// there before. Commit appends a "commit" line and deletes the journal; a
// journal found on disk without a commit line belongs to an interrupted run
// and is rolled back by Recover.
//
// Files are replaced atomically: the new content is created under a
// temporary name in the target directory and renamed over the target.
package transaction

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OpType is the kind of a journaled operation.
type OpType string

const (
	OpBegin   OpType = "begin"
	OpSymlink OpType = "symlink"
	OpWrite   OpType = "write" // copy, secret, restore: a regular file is placed at Path
	OpRemove  OpType = "remove"
	OpMkdir   OpType = "mkdir"
	OpCommit  OpType = "commit"
)

// PriorKind describes what existed at a path before an operation.
type PriorKind string

const (
	PriorNone    PriorKind = "none"
	PriorSymlink PriorKind = "symlink"
	PriorFile    PriorKind = "file"
)

// Entry is one journal line.
type Entry struct {
	Op   OpType `json:"op"`
	Seq  int    `json:"seq,omitempty"`
	Path string `json:"path,omitempty"`
	Tmp  string `json:"tmp,omitempty"` // temporary name renamed onto Path

	Prior       PriorKind   `json:"prior,omitempty"`
	PriorTarget string      `json:"prior_target,omitempty"` // symlink target
	PriorStash  string      `json:"prior_stash,omitempty"`  // stashed copy of a regular file
	PriorMode   os.FileMode `json:"prior_mode,omitempty"`

	ID   string `json:"id,omitempty"`
	Time string `json:"time,omitempty"`
}

// Transaction groups filesystem operations so they can be undone together.
type Transaction struct {
	mu       sync.Mutex
	stateDir string
	id       string
	dir      string // <stateDir>/transactions/<id>
	journal  *os.File
	entries  []Entry
	done     bool
}

// TxDirName is the subdirectory of the state directory holding journals.
const TxDirName = "transactions"

// New creates a transaction. Nothing is written to disk until the first
// operation.
func New(stateDir string) (*Transaction, error) {
	if stateDir == "" {
		return nil, errors.New("transaction: empty state directory")
	}
	var rnd [4]byte
	rand.Read(rnd[:])
	id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(rnd[:]))
	return &Transaction{
		stateDir: stateDir,
		id:       id,
		dir:      filepath.Join(stateDir, TxDirName, id),
	}, nil
}

// ID returns the transaction identifier.
func (tx *Transaction) ID() string { return tx.id }

// JournalPath returns the path of the journal file.
func (tx *Transaction) JournalPath() string { return filepath.Join(tx.dir, "journal") }

// Len returns the number of journaled operations.
func (tx *Transaction) Len() int {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	return len(tx.entries)
}

func (tx *Transaction) open() error {
	if tx.done {
		return errors.New("transaction already finished")
	}
	if tx.journal != nil {
		return nil
	}
	if err := os.MkdirAll(tx.dir, 0o700); err != nil {
		return fmt.Errorf("cannot create transaction directory: %w", err)
	}
	f, err := os.OpenFile(tx.JournalPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("cannot create journal: %w", err)
	}
	tx.journal = f
	return tx.appendLine(Entry{Op: OpBegin, ID: tx.id, Time: time.Now().Format(time.RFC3339Nano)})
}

func (tx *Transaction) appendLine(e Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := tx.journal.Write(b); err != nil {
		return fmt.Errorf("cannot write journal: %w", err)
	}
	if err := tx.journal.Sync(); err != nil {
		return fmt.Errorf("cannot sync journal: %w", err)
	}
	return nil
}

// record captures what currently exists at path and journals the entry.
func (tx *Transaction) record(e Entry) (Entry, error) {
	if err := tx.open(); err != nil {
		return e, err
	}
	e.Seq = len(tx.entries) + 1
	if e.Op != OpMkdir {
		info, err := os.Lstat(e.Path)
		switch {
		case os.IsNotExist(err):
			e.Prior = PriorNone
		case err != nil:
			return e, fmt.Errorf("cannot stat %s: %w", e.Path, err)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(e.Path)
			if err != nil {
				return e, fmt.Errorf("cannot read symlink %s: %w", e.Path, err)
			}
			e.Prior, e.PriorTarget = PriorSymlink, target
		case info.Mode().IsRegular():
			stash := filepath.Join(tx.dir, fmt.Sprintf("%d.prior", e.Seq))
			if err := copyFile(e.Path, stash, info.Mode().Perm()); err != nil {
				return e, fmt.Errorf("cannot save previous %s: %w", e.Path, err)
			}
			e.Prior, e.PriorStash, e.PriorMode = PriorFile, stash, info.Mode().Perm()
		default:
			return e, fmt.Errorf("refusing to replace %s: not a regular file or symlink", e.Path)
		}
	}
	if err := tx.appendLine(e); err != nil {
		return e, err
	}
	tx.entries = append(tx.entries, e)
	return e, nil
}

func (tx *Transaction) tmpName(target string) string {
	var rnd [4]byte
	rand.Read(rnd[:])
	return filepath.Join(filepath.Dir(target),
		fmt.Sprintf(".%s.dotf-tmp-%s", filepath.Base(target), hex.EncodeToString(rnd[:])))
}

// Symlink atomically makes target a symlink to source.
func (tx *Transaction) Symlink(source, target string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	e, err := tx.record(Entry{Op: OpSymlink, Path: target, Tmp: tx.tmpName(target)})
	if err != nil {
		return err
	}
	if err := os.Symlink(source, e.Tmp); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", target, source, err)
	}
	if err := os.Rename(e.Tmp, target); err != nil {
		os.Remove(e.Tmp)
		return fmt.Errorf("rename %s: %w", target, err)
	}
	return nil
}

// Copy atomically places a copy of source at target, keeping source's mode.
func (tx *Transaction) Copy(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	return tx.WriteFile(target, info.Mode().Perm(), func(w io.Writer) error {
		f, err := os.Open(source)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

// WriteFile atomically places a regular file with the given mode at target.
// write is called with the temporary file, which is already created with
// mode, so secrets are never readable by others, not even briefly.
func (tx *Transaction) WriteFile(target string, mode os.FileMode, write func(io.Writer) error) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	e, err := tx.record(Entry{Op: OpWrite, Path: target, Tmp: tx.tmpName(target)})
	if err != nil {
		return err
	}
	// Created owner-only; the final mode is applied after the content is written.
	f, err := os.OpenFile(e.Tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", e.Tmp, err)
	}
	werr := write(f)
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(e.Tmp, mode.Perm())
	}
	if werr != nil {
		os.Remove(e.Tmp)
		return fmt.Errorf("write %s: %w", target, werr)
	}
	if err := os.Rename(e.Tmp, target); err != nil {
		os.Remove(e.Tmp)
		return fmt.Errorf("rename %s: %w", target, err)
	}
	return nil
}

// Remove removes path (a file or symlink). A missing path is not an error.
func (tx *Transaction) Remove(path string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	if _, err := tx.record(Entry{Op: OpRemove, Path: path}); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// MkdirAll creates dir and any missing parents, journaling each directory it
// creates so rollback can remove them again (if they are still empty).
func (tx *Transaction) MkdirAll(dir string, perm os.FileMode) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	var missing []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		info, err := os.Stat(d)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s exists and is not a directory", d)
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if _, err := tx.record(Entry{Op: OpMkdir, Path: missing[i]}); err != nil {
			return err
		}
		if err := os.Mkdir(missing[i], perm); err != nil && !os.IsExist(err) {
			return fmt.Errorf("mkdir %s: %w", missing[i], err)
		}
	}
	return nil
}

// Commit finalizes the transaction and deletes its journal and stash.
func (tx *Transaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return nil
	}
	tx.done = true
	if tx.journal == nil {
		return nil // nothing was done
	}
	err := tx.appendLine(Entry{Op: OpCommit, Time: time.Now().Format(time.RFC3339Nano)})
	tx.journal.Close()
	tx.journal = nil
	if err != nil {
		return err
	}
	return os.RemoveAll(tx.dir)
}

// Rollback undoes every operation of the transaction, newest first, and
// restores what existed before. If something cannot be restored the journal
// is kept so `dotf doctor --fix` can retry.
func (tx *Transaction) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return nil
	}
	tx.done = true
	if tx.journal == nil {
		return nil
	}
	tx.journal.Close()
	tx.journal = nil
	if err := undo(tx.entries, nil); err != nil {
		return err
	}
	return os.RemoveAll(tx.dir)
}

// undo reverts entries in reverse order.
func undo(entries []Entry, log io.Writer) error {
	var errs []error
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Tmp != "" {
			os.Remove(e.Tmp)
		}
		var err error
		switch e.Op {
		case OpMkdir:
			os.Remove(e.Path) // only succeeds if empty
			continue
		case OpSymlink, OpWrite, OpRemove:
			err = restorePrior(e)
		default:
			continue
		}
		if err != nil {
			errs = append(errs, err)
		} else if log != nil {
			fmt.Fprintf(log, "  restored %s\n", e.Path)
		}
	}
	return errors.Join(errs...)
}

func restorePrior(e Entry) error {
	switch e.Prior {
	case PriorNone:
		if err := os.Remove(e.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove %s: %w", e.Path, err)
		}
	case PriorSymlink:
		if cur, err := os.Readlink(e.Path); err == nil && cur == e.PriorTarget {
			return nil
		}
		tmp := e.Path + ".dotf-rollback"
		os.Remove(tmp)
		if err := os.Symlink(e.PriorTarget, tmp); err != nil {
			return fmt.Errorf("cannot restore symlink %s: %w", e.Path, err)
		}
		if err := os.Rename(tmp, e.Path); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("cannot restore symlink %s: %w", e.Path, err)
		}
	case PriorFile:
		tmp := e.Path + ".dotf-rollback"
		os.Remove(tmp)
		if err := copyFile(e.PriorStash, tmp, e.PriorMode); err != nil {
			return fmt.Errorf("cannot restore %s: %w", e.Path, err)
		}
		if err := os.Rename(tmp, e.Path); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("cannot restore %s: %w", e.Path, err)
		}
	}
	return nil
}

// Pending returns the journal paths of interrupted transactions.
func Pending(stateDir string) []string {
	var out []string
	dirs, _ := os.ReadDir(filepath.Join(stateDir, TxDirName))
	for _, d := range dirs {
		if d.IsDir() {
			p := filepath.Join(stateDir, TxDirName, d.Name(), "journal")
			if _, err := os.Stat(p); err == nil {
				out = append(out, p)
			}
		}
	}
	legacy, _ := filepath.Glob(filepath.Join(stateDir, "*.journal"))
	return append(out, legacy...)
}

// RecoverIncomplete rolls back every interrupted transaction in stateDir,
// writing progress to stderr. The caller must hold the DOTF lock.
func RecoverIncomplete(stateDir string) error {
	_, err := Recover(stateDir, os.Stderr)
	return err
}

// Recover rolls back interrupted transactions and returns how many were
// found. Journals that end in a commit line are simply removed.
func Recover(stateDir string, log io.Writer) (int, error) {
	var errs []error
	n := 0
	for _, jp := range Pending(stateDir) {
		n++
		if strings.HasSuffix(jp, ".journal") {
			recoverLegacy(jp, log)
			continue
		}
		entries, committed, err := readJournal(jp)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", jp, err))
			continue
		}
		dir := filepath.Dir(jp)
		if committed {
			os.RemoveAll(dir)
			continue
		}
		if log != nil {
			fmt.Fprintf(log, "Rolling back interrupted transaction %s\n", filepath.Base(dir))
		}
		if err := undo(entries, log); err != nil {
			errs = append(errs, err)
			continue
		}
		os.RemoveAll(dir)
	}
	return n, errors.Join(errs...)
}

func readJournal(path string) ([]Entry, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	var entries []Entry
	committed := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			// A torn last line means the process died while writing it;
			// the operation it describes never started.
			break
		}
		switch e.Op {
		case OpCommit:
			committed = true
		case OpBegin:
		default:
			entries = append(entries, e)
		}
	}
	return entries, committed, sc.Err()
}

// recoverLegacy handles journals written by dotf <= 0.6 ("STATUS|TYPE|path|target|time").
// Those journals cannot restore previous content; incomplete symlinks and
// copies are removed as before.
func recoverLegacy(path string, log io.Writer) {
	data, err := os.ReadFile(path)
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			parts := strings.SplitN(line, "|", 5)
			if len(parts) < 4 || parts[0] == "DONE" {
				continue
			}
			if parts[1] == "SYMLINK" || parts[1] == "COPY" {
				if _, err := os.Lstat(parts[2]); err == nil {
					os.Remove(parts[2])
					if log != nil {
						fmt.Fprintf(log, "  removed incomplete %s\n", parts[2])
					}
				}
			}
		}
	}
	os.Remove(path)
}

// copyFile copies src to dst (which must not exist) and sets mode.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(dst, mode.Perm())
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}
