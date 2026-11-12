package transaction

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewTransactionWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	tx, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tx.ID() == "" {
		t.Fatal("empty id")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("an empty transaction must not create the state directory")
	}
}

func TestSymlinkAndCommit(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	tgt := filepath.Join(dir, "target.txt")
	write(t, src, "hello", 0o644)

	tx, _ := New(filepath.Join(dir, "state"))
	if err := tx.Symlink(src, tgt); err != nil {
		t.Fatal(err)
	}
	if link, _ := os.Readlink(tgt); link != src {
		t.Fatalf("expected link to %s, got %q", src, link)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tx.JournalPath()); !os.IsNotExist(err) {
		t.Fatal("journal not removed after commit")
	}
	if n := len(Pending(filepath.Join(dir, "state"))); n != 0 {
		t.Fatalf("expected no pending transactions, got %d", n)
	}
}

func TestSymlinkReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	tgt := filepath.Join(dir, "target.txt")
	write(t, src, "new", 0o644)
	write(t, tgt, "old", 0o600)

	tx, _ := New(filepath.Join(dir, "state"))
	if err := tx.Symlink(src, tgt); err != nil {
		t.Fatal(err)
	}
	if link, _ := os.Readlink(tgt); link != src {
		t.Fatalf("target not replaced by symlink")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "dotf-tmp") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}

// Rollback must undo every operation, including ones that completed, and
// put back what was there before.
func TestRollbackRestoresEverything(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	src := filepath.Join(dir, "repo", "a")
	write(t, src, "repo", 0o644)

	fileTgt := filepath.Join(home, ".zshrc")
	write(t, fileTgt, "user zshrc", 0o600)
	linkTgt := filepath.Join(home, ".vimrc")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/somewhere/else", linkTgt); err != nil {
		t.Fatal(err)
	}
	newTgt := filepath.Join(home, ".config", "kitty", "kitty.conf")
	removed := filepath.Join(home, ".bashrc")
	write(t, removed, "bash", 0o640)

	tx, _ := New(filepath.Join(dir, "state"))
	if err := tx.Symlink(src, fileTgt); err != nil {
		t.Fatal(err)
	}
	if err := tx.Symlink(src, linkTgt); err != nil {
		t.Fatal(err)
	}
	if err := tx.MkdirAll(filepath.Dir(newTgt), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := tx.Copy(src, newTgt); err != nil {
		t.Fatal(err)
	}
	if err := tx.Remove(removed); err != nil {
		t.Fatal(err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if got := read(t, fileTgt); got != "user zshrc" {
		t.Fatalf(".zshrc not restored: %q", got)
	}
	if info, _ := os.Lstat(fileTgt); info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf(".zshrc should be a 0600 regular file again, got %v", info.Mode())
	}
	if link, _ := os.Readlink(linkTgt); link != "/somewhere/else" {
		t.Fatalf(".vimrc symlink not restored: %q", link)
	}
	if _, err := os.Lstat(newTgt); !os.IsNotExist(err) {
		t.Fatal("new file not removed by rollback")
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Fatal("directories created by the transaction should be removed")
	}
	if got := read(t, removed); got != "bash" {
		t.Fatalf("removed file not restored: %q", got)
	}
	if _, err := os.Stat(tx.JournalPath()); !os.IsNotExist(err) {
		t.Fatal("journal kept after successful rollback")
	}
}

func TestCopyKeepsMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "script.sh")
	tgt := filepath.Join(dir, "out.sh")
	write(t, src, "#!/bin/sh\n", 0o750)

	tx, _ := New(filepath.Join(dir, "state"))
	if err := tx.Copy(src, tgt); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(tgt)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("expected 0750, got %o", info.Mode().Perm())
	}
}

func TestWriteFileMode(t *testing.T) {
	dir := t.TempDir()
	tgt := filepath.Join(dir, "secret")
	tx, _ := New(filepath.Join(dir, "state"))
	err := tx.WriteFile(tgt, 0o600, func(w io.Writer) error {
		_, err := io.Copy(w, bytes.NewBufferString("token"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(tgt)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
	if read(t, tgt) != "token" {
		t.Fatal("wrong content")
	}
}

func TestCopyMissingSourceFails(t *testing.T) {
	dir := t.TempDir()
	tx, _ := New(filepath.Join(dir, "state"))
	if err := tx.Copy(filepath.Join(dir, "missing"), filepath.Join(dir, "t")); err == nil {
		t.Fatal("expected error")
	}
}

func TestRefusesToReplaceDirectory(t *testing.T) {
	dir := t.TempDir()
	tgt := filepath.Join(dir, "adir")
	if err := os.Mkdir(tgt, 0o755); err != nil {
		t.Fatal(err)
	}
	tx, _ := New(filepath.Join(dir, "state"))
	if err := tx.Symlink("/x", tgt); err == nil {
		t.Fatal("expected refusal to replace a directory")
	}
}

func TestDoubleRollbackAndCommit(t *testing.T) {
	tx, _ := New(t.TempDir())
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// A journal without a commit line (process killed mid-transaction) is
// rolled back by Recover.
func TestRecoverInterruptedTransaction(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	src := filepath.Join(dir, "src")
	tgt := filepath.Join(dir, ".zshrc")
	write(t, src, "repo", 0o644)
	write(t, tgt, "original", 0o644)

	tx, _ := New(stateDir)
	if err := tx.Symlink(src, tgt); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash: neither Commit nor Rollback runs.
	tx.journal.Close()

	if n := len(Pending(stateDir)); n != 1 {
		t.Fatalf("expected 1 pending transaction, got %d", n)
	}
	var log bytes.Buffer
	n, err := Recover(stateDir, &log)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 recovered, got %d", n)
	}
	if got := read(t, tgt); got != "original" {
		t.Fatalf("original not restored: %q", got)
	}
	if len(Pending(stateDir)) != 0 {
		t.Fatal("journal not removed after recovery")
	}
}

func TestRecoverTornLastLine(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	tgt := filepath.Join(dir, "new")
	tx, _ := New(stateDir)
	if err := tx.Symlink("/x", tgt); err != nil {
		t.Fatal(err)
	}
	tx.journal.Write([]byte(`{"op":"symlink","seq":2,"pa`))
	tx.journal.Close()

	if _, err := Recover(stateDir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(tgt); !os.IsNotExist(err) {
		t.Fatal("symlink from interrupted transaction not removed")
	}
}

func TestRecoverLegacyJournal(t *testing.T) {
	dir := t.TempDir()
	tgt := filepath.Join(dir, "link")
	if err := os.Symlink("/x", tgt); err != nil {
		t.Fatal(err)
	}
	legacy := "PENDING|SYMLINK|" + tgt + "|/x|2024-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "dotf-1.journal"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RecoverIncomplete(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(tgt); !os.IsNotExist(err) {
		t.Fatal("legacy pending symlink not removed")
	}
	if len(Pending(dir)) != 0 {
		t.Fatal("legacy journal not removed")
	}
}

func TestRecoverEmptyAndMissingDir(t *testing.T) {
	if err := RecoverIncomplete(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := RecoverIncomplete(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := t.TempDir()
	tx, _ := New(dir)
	if err := tx.Remove(filepath.Join(dir, "nonexistent.txt")); err != nil {
		t.Fatal(err)
	}
	if tx.Len() != 0 {
		t.Fatal("removing a missing file should not be journaled")
	}
}
