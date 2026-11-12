package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManagerWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if _, err := NewManager(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("NewManager must not create the state directory")
	}
}

func TestSavePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	m, _ := NewManager(dir)
	m.SetRepository("/repo")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("state dir mode %o, want 700", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Join(dir, "state.json")); info.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode %o, want 600", info.Mode().Perm())
	}
	m2, _ := NewManager(dir)
	if m2.GetState().Repository != "/repo" {
		t.Fatal("repository not persisted")
	}
}

func TestCorruptStateIsKept(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Warning() == "" {
		t.Fatal("expected a warning for corrupt state")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(statePath + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatalf("expected corrupt copy, got %v", matches)
	}
	data, _ := os.ReadFile(matches[0])
	if !strings.Contains(string(data), "{not json") {
		t.Fatal("corrupt copy has wrong content")
	}
}

func TestRemoveFileAndFiles(t *testing.T) {
	m, _ := NewManager(t.TempDir())
	m.RecordFile("b", "base", TypeSymlink, "/r/b")
	m.RecordFile("a", "base", TypeSymlink, "/r/a")
	if got := strings.Join(m.Files(), ","); got != "a,b" {
		t.Fatalf("Files() = %s", got)
	}
	m.RemoveFile("a")
	if _, ok := m.File("a"); ok {
		t.Fatal("RemoveFile did not remove")
	}
}
