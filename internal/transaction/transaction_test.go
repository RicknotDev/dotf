package transaction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewTransaction(t *testing.T) {
	dir := t.TempDir()
	tx, err := New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if tx == nil {
		t.Fatal("New() returned nil")
	}
	if tx.journalID == "" {
		t.Fatal("journalID is empty")
	}
}

func TestSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	tgt := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(src, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := tx.Symlink(src, tgt); err != nil {
		t.Fatalf("Symlink() failed: %v", err)
	}

	// Verify symlink exists and points correctly
	link, err := os.Readlink(tgt)
	if err != nil {
		t.Fatalf("Readlink() failed: %v", err)
	}
	if link != src {
		t.Fatalf("expected %s, got %s", src, link)
	}

	// Verify journal is cleaned up after Commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}
	_, err = os.Stat(tx.journalPath())
	if !os.IsNotExist(err) {
		t.Fatal("journal file was not cleaned up after commit")
	}
}

func TestSymlinkRollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	// Don't create source — symlink should fail
	target := filepath.Join(dir, "nonexistent/target.txt")

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	err = tx.Symlink("/nonexistent-source", target)
	if err == nil {
		t.Fatal("expected error for symlink to nonexistent source")
	}

	// Journal should be cleaned up after rollback
	_, err = os.Stat(tx.journalPath())
	if !os.IsNotExist(err) {
		t.Fatal("journal file was not cleaned up after rollback")
	}
}

func TestCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	tgt := filepath.Join(dir, "target.txt")
	content := []byte("test content")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := tx.Copy(src, tgt); err != nil {
		t.Fatalf("Copy() failed: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(tgt)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("expected %q, got %q", content, data)
	}
}

func TestCopyRollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	// Target in nonexistent directory
	target := filepath.Join(dir, "nonexistent/target.txt")

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := tx.Copy("/nonexistent-source", target); err == nil {
		t.Fatal("expected error for copy from nonexistent source")
	}
}

func TestCommit(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	tgt := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := tx.Symlink(src, tgt); err != nil {
		t.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	if !tx.completed {
		t.Fatal("transaction not marked completed")
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()

	// Test that uncommitted operations are rolled back
	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Manually log an operation without committing
	tx.logOperation(OpSymlink, "/tmp/fake-target", "/tmp/fake-source")

	tx.Rollback()

	// Journal should be cleaned up after rollback
	_, err = os.Stat(tx.journalPath())
	if !os.IsNotExist(err) {
		t.Fatal("journal file was not cleaned up after rollback")
	}
}

func TestCommittedOperationNotRolledBack(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	tgt := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Symlink creates and commits the operation
	if err := tx.Symlink(src, tgt); err != nil {
		t.Fatal(err)
	}

	// Rollback should NOT remove committed operations
	tx.Rollback()

	// Symlink should still exist (was committed before rollback)
	link, err := os.Readlink(tgt)
	if err != nil {
		t.Fatal("symlink was incorrectly removed during rollback of committed operation")
	}
	if link != src {
		t.Fatalf("expected %s, got %s", src, link)
	}
}

func TestDoubleRollback(t *testing.T) {
	dir := t.TempDir()
	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// First rollback should succeed
	tx.Rollback()
	// Second rollback should not panic
	tx.Rollback()
}

func TestRecoverIncomplete(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	tgt := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a symlink but don't commit
	_ = tx.Symlink(src, tgt)

	// Remove the journal manually to simulate crash after journal write but before commit
	journalPath := tx.journalPath()

	// Verify journal exists
	if _, err := os.Stat(journalPath); os.IsNotExist(err) {
		// The symlink was committed successfully, journal was cleaned up
		// Test with an uncommitted operation instead
		tx2, _ := New(dir)
		tx2.logOperation(OpSymlink, tgt, src)
		journalPath2 := tx2.journalPath()

		// Verify journal exists
		if _, err := os.Stat(journalPath2); err != nil {
			t.Fatalf("journal file not created: %v", err)
		}

		// Try recovery
		if err := RecoverIncomplete(dir); err != nil {
			t.Fatalf("RecoverIncomplete() failed: %v", err)
		}
	}
}

func TestRecoverIncompleteEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := RecoverIncomplete(dir); err != nil {
		t.Fatalf("RecoverIncomplete() on empty dir failed: %v", err)
	}
}

func TestRecoverIncompleteNonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	if err := RecoverIncomplete(dir); err != nil {
		t.Fatalf("RecoverIncomplete() on nonexistent dir failed: %v", err)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "remove_me.txt")
	if err := os.WriteFile(file, []byte("delete me"), 0644); err != nil {
		t.Fatal(err)
	}

	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := tx.Remove(file); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}

	_, err = os.Stat(file)
	if !os.IsNotExist(err) {
		t.Fatal("file was not removed")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := t.TempDir()
	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Removing nonexistent file should not error
	if err := tx.Remove(filepath.Join(dir, "nonexistent.txt")); err != nil {
		t.Fatalf("Remove() on nonexistent file failed: %v", err)
	}
}
