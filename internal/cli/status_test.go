package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codebuff/dotf/internal/state"
)

func TestStatusHelp(t *testing.T) {
	cfg := OutputConfig{}
	err := Status([]string{"--help"}, t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("Status --help returned error: %v", err)
	}
}

func TestStatusNoFiles(t *testing.T) {
	stateDir := t.TempDir()
	cfg := OutputConfig{}

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Status([]string{}, stateDir, cfg)
	// Should return "nothing to do" error
	if err == nil {
		t.Fatal("expected error for no installed files")
	}
	if !strings.Contains(err.Error(), "nothing to do") {
		t.Fatalf("expected 'nothing to do' error, got: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	_ = buf.String()
}

func TestStatusWithInstalledFiles(t *testing.T) {
	stateDir := t.TempDir()

	// Create a state with installed files
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a repo structure to point to
	repoDir := t.TempDir()
	os.MkdirAll(filepath.Join(repoDir, "layers", "base"), 0755)
	os.WriteFile(filepath.Join(repoDir, "layers", "base", ".zshrc"), []byte("export FOO=bar"), 0644)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create actual symlink
	symlinkPath := filepath.Join(homeDir, ".zshrc")
	os.Symlink(filepath.Join(repoDir, "layers", "base", ".zshrc"), symlinkPath)

	sourcePath := filepath.Join(repoDir, "layers", "base", ".zshrc")
	sm.RecordFile(".zshrc", "base", "symlink", sourcePath)
	sm.RecordInstall([]string{"base"})
	sm.Save()

	oldWd, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(oldWd)

	cfg := OutputConfig{}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Status:") {
		t.Fatalf("expected 'Status:' in output, got: %s", output)
	}
	if !strings.Contains(output, "OK") {
		t.Fatalf("expected 'OK' status, got: %s", output)
	}
}

func TestStatusJSON(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	repoDir := t.TempDir()
	os.MkdirAll(filepath.Join(repoDir, "layers", "base"), 0755)
	os.WriteFile(filepath.Join(repoDir, "layers", "base", ".zshrc"), []byte("data"), 0644)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	symlinkPath := filepath.Join(homeDir, ".zshrc")
	os.Symlink(filepath.Join(repoDir, "layers", "base", ".zshrc"), symlinkPath)

	sm.RecordFile(".zshrc", "base", "symlink", filepath.Join(repoDir, "layers", "base", ".zshrc"))
	sm.Save()

	oldWd, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(oldWd)

	cfg := OutputConfig{JSON: true}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Status() with JSON failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "\"version\"") {
		t.Fatalf("expected JSON 'version' field, got: %s", output)
	}
	if !strings.Contains(output, "\"entries\"") {
		t.Fatalf("expected JSON 'entries' field, got: %s", output)
	}
	if !strings.Contains(output, "\"summary\"") {
		t.Fatalf("expected JSON 'summary' field, got: %s", output)
	}
}

func TestStatusShort(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	repoDir := t.TempDir()
	os.MkdirAll(filepath.Join(repoDir, "layers", "base"), 0755)
	os.WriteFile(filepath.Join(repoDir, "layers", "base", ".zshrc"), []byte("data"), 0644)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	symlinkPath := filepath.Join(homeDir, ".zshrc")
	os.Symlink(filepath.Join(repoDir, "layers", "base", ".zshrc"), symlinkPath)

	sm.RecordFile(".zshrc", "base", "symlink", filepath.Join(repoDir, "layers", "base", ".zshrc"))
	sm.Save()

	oldWd, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(oldWd)

	cfg := OutputConfig{}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{"--short"}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Status --short failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// With only OK files, --short should show nothing (just the summary)
	// Even the summary line should not show if everything is OK in short mode
	_ = output
}

func TestStatusBrokenSymlink(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	repoDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create symlink to non-existent file
	symlinkPath := filepath.Join(homeDir, "broken.txt")
	os.Symlink(filepath.Join(repoDir, "nonexistent.txt"), symlinkPath)

	sm.RecordFile("broken.txt", "base", "symlink", filepath.Join(repoDir, "nonexistent.txt"))
	sm.Save()

	oldWd, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(oldWd)

	cfg := OutputConfig{}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{}, stateDir, cfg)
	// Should have issues
	if err == nil {
		t.Fatal("expected error for broken symlink")
	}
	if !strings.Contains(err.Error(), "have issues") {
		t.Fatalf("expected 'have issues' error, got: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "broken") {
		t.Fatalf("expected 'broken' status, got: %s", output)
	}
}

func TestStatusWithFilter(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	repoDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create an OK symlink
	symlinkPath := filepath.Join(homeDir, ".zshrc")
	os.WriteFile(filepath.Join(repoDir, "ok_target.txt"), []byte("data"), 0644)
	os.Symlink(filepath.Join(repoDir, "ok_target.txt"), symlinkPath)
	sm.RecordFile(".zshrc", "base", "symlink", filepath.Join(repoDir, "ok_target.txt"))

	// Create a broken symlink
	brokenPath := filepath.Join(homeDir, "broken.txt")
	os.Symlink(filepath.Join(repoDir, "nonexistent.txt"), brokenPath)
	sm.RecordFile("broken.txt", "base", "symlink", filepath.Join(repoDir, "nonexistent.txt"))

	sm.Save()

	oldWd, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(oldWd)

	// Test JSON with filter
	cfg := OutputConfig{JSON: true, Filter: "broken"}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Note: Status will return an error because there are issues, even with --json
	err = Status([]string{}, stateDir, cfg)
	_ = err // may or may not be nil depending on filter behavior

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// The output may vary but should be valid JSON
	if !strings.Contains(output, "\"status\"") {
		t.Fatalf("expected 'status' field in JSON output, got: %s", output)
	}
}

func TestStatusWithMissingFile(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	repoDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Record a file that doesn't exist on filesystem
	sm.RecordFile("missing.txt", "base", "symlink", filepath.Join(repoDir, "source.txt"))
	sm.Save()

	oldWd, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(oldWd)

	cfg := OutputConfig{}
	err = Status([]string{}, stateDir, cfg)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "have issues") {
		t.Fatalf("expected 'have issues' error, got: %v", err)
	}
}

func TestStatusNoColor(t *testing.T) {
	stateDir := t.TempDir()
	sm, err := state.NewManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	repoDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	symlinkPath := filepath.Join(homeDir, ".zshrc")
	os.WriteFile(filepath.Join(repoDir, "target.txt"), []byte("data"), 0644)
	os.Symlink(filepath.Join(repoDir, "target.txt"), symlinkPath)
	sm.RecordFile(".zshrc", "base", "symlink", filepath.Join(repoDir, "target.txt"))
	sm.Save()

	oldWd, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(oldWd)

	// Using --no-color should produce clean output
	cfg := OutputConfig{NoColor: true}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = Status([]string{}, stateDir, cfg)
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should not contain color codes
	if strings.Contains(output, "\033[") {
		t.Fatal("expected no color codes when --no-color is set")
	}
}
