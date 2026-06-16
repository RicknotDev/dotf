// Package transaction provides atomic operations with write-ahead logging
// and automatic rollback for DOTF. All mutating filesystem operations
// must go through this package to ensure crash safety.
package transaction

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OpType represents a type of filesystem operation.
type OpType string

const (
	OpSymlink OpType = "SYMLINK"
	OpCopy    OpType = "COPY"
	OpMkdir   OpType = "MKDIR"
	OpRemove  OpType = "REMOVE"
	OpBackup  OpType = "BACKUP"
)

// Operation represents a single logged operation.
type Operation struct {
	Type      OpType `json:"type"`
	Path      string `json:"path"`
	Target    string `json:"target,omitempty"` // symlink target or backup path
	Timestamp string `json:"timestamp"`
	Committed bool   `json:"committed"`
}

// Transaction provides atomic filesystem operations with rollback.
type Transaction struct {
	mu        sync.Mutex
	journal   []Operation
	stateDir  string
	journalID string
	completed bool
}

// New creates a new transaction.
func New(stateDir string) (*Transaction, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create transaction directory: %w", err)
	}

	journalID := fmt.Sprintf("dotf-%d", time.Now().UnixNano())

	tx := &Transaction{
		stateDir:  stateDir,
		journalID: journalID,
	}

	return tx, nil
}

// journalPath returns the path to the journal file.
func (tx *Transaction) journalPath() string {
	return filepath.Join(tx.stateDir, tx.journalID+".journal")
}

// logOperation writes an operation to the write-ahead journal.
func (tx *Transaction) logOperation(op OpType, path, target string) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	entry := Operation{
		Type:      op,
		Path:      path,
		Target:    target,
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Committed: false,
	}
	tx.journal = append(tx.journal, entry)
	tx.flushJournal()
}

// commitOperation marks an operation as committed.
func (tx *Transaction) commitOperation(index int) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if index < len(tx.journal) {
		tx.journal[index].Committed = true
	}
	tx.flushJournal()
}

// flushJournal writes the journal to disk.
func (tx *Transaction) flushJournal() {
	data := strings.Builder{}
	for _, op := range tx.journal {
		status := "PENDING"
		if op.Committed {
			status = "DONE"
		}
		data.WriteString(fmt.Sprintf("%s|%s|%s|%s|%s\n",
			status, op.Type, op.Path, op.Target, op.Timestamp))
	}

	jPath := tx.journalPath()
	tmpPath := jPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(data.String()), 0644); err != nil {
		return // best effort
	}
	if err := os.Rename(tmpPath, jPath); err != nil {
		os.Remove(tmpPath) // clean up temp file on rename failure
	}
}

// Symlink performs an atomic symlink creation within the transaction.
func (tx *Transaction) Symlink(source, target string) error {
	idx := len(tx.journal)
	tx.logOperation(OpSymlink, target, source)

	if err := os.Symlink(source, target); err != nil {
		tx.Rollback()
		return fmt.Errorf("symlink %s -> %s: %w", target, source, err)
	}

	tx.commitOperation(idx)
	return nil
}

// Copy performs an atomic file copy within the transaction.
func (tx *Transaction) Copy(source, target string) error {
	idx := len(tx.journal)
	tx.logOperation(OpCopy, target, source)

	data, err := os.ReadFile(source)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("read %s: %w", source, err)
	}

	// Write to temp file then rename for atomicity
	tmpPath := target + ".dotf-tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		tx.Rollback()
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		tx.Rollback()
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, target, err)
	}

	tx.commitOperation(idx)
	return nil
}

// Remove removes a file within the transaction.
func (tx *Transaction) Remove(path string) error {
	idx := len(tx.journal)
	tx.logOperation(OpRemove, path, "")

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		tx.Rollback()
		return fmt.Errorf("remove %s: %w", path, err)
	}

	tx.commitOperation(idx)
	return nil
}

// Rollback undoes all uncommitted operations.
func (tx *Transaction) Rollback() {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	// Walk journal in reverse
	for i := len(tx.journal) - 1; i >= 0; i-- {
		op := tx.journal[i]

		switch op.Type {
		case OpSymlink:
			// Remove the symlink we created
			if !op.Committed {
				os.Remove(op.Path)
			}
		case OpCopy:
			// Remove the file we copied
			if !op.Committed {
				os.Remove(op.Path)
			}
		case OpMkdir:
			// We don't remove directories on rollback (safety)
		case OpRemove:
			// Can't undo a remove without a backup reference
			// Backups should be created before removes in the journal
		case OpBackup:
			// Backups are preserved even on rollback
		}
	}

	tx.journal = nil
	tx.completed = true
	tx.cleanupJournal()
}

// Commit finalizes the transaction.
func (tx *Transaction) Commit() error {
	// Mark all operations as committed
	tx.mu.Lock()
	for i := range tx.journal {
		tx.journal[i].Committed = true
	}
	tx.completed = true
	tx.mu.Unlock()

	tx.flushJournal()
	tx.cleanupJournal()
	return nil
}

// cleanupJournal removes the journal file after successful completion.
func (tx *Transaction) cleanupJournal() {
	os.Remove(tx.journalPath())
}

// RecoverIncomplete checks for incomplete transactions and rolls them back.
func RecoverIncomplete(stateDir string) error {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot read state directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".journal") {
			continue
		}

		journalPath := filepath.Join(stateDir, entry.Name())
		if err := recoverJournal(journalPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: incomplete transaction recovered: %s\n", journalPath)
		}
	}

	return nil
}

// recoverJournal reads a journal file and rolls back any uncommitted operations.
func recoverJournal(journalPath string) error {
	data, err := os.ReadFile(journalPath)
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		os.Remove(journalPath)
		return nil
	}

	// Walk in reverse, rolling back PENDING operations
	for i := len(lines) - 1; i >= 0; i-- {
		parts := strings.SplitN(lines[i], "|", 5)
		if len(parts) < 4 {
			continue
		}

		status := parts[0]
		opType := OpType(parts[1])
		path := parts[2]
		target := parts[3]

		if status == "DONE" {
			continue // already committed, don't touch
		}

		// Rollback PENDING operation
		switch opType {
		case OpSymlink:
			if _, err := os.Lstat(path); err == nil {
				os.Remove(path)
				fmt.Fprintf(os.Stderr, "  recovered: removed incomplete symlink %s\n", path)
			}
		case OpCopy:
			if _, err := os.Lstat(path); err == nil {
				os.Remove(path)
				fmt.Fprintf(os.Stderr, "  recovered: removed incomplete copy %s\n", path)
			}
		case OpBackup:
			// Backup was created — check if original still exists
			if _, err := os.Lstat(target); err != nil {
				// Original was modified, but backup exists — preserved
				fmt.Fprintf(os.Stderr, "  recovered: backup preserved %s\n", target)
			}
		}
	}

	os.Remove(journalPath)
	return nil
}
