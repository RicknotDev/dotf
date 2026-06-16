package reorganize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupFlatStructure creates a temporary flat dotfiles directory for testing.
func setupFlatStructure(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create flat app directories
	apps := map[string]map[string]string{
		"zsh": {
			".zshrc": "export ZSH=$HOME/.oh-my-zsh\n",
		},
		"alacritty": {
			".config/alacritty/alacritty.toml": "[window]\nopacity = 0.95\n",
		},
		"nvim": {
			".config/nvim/init.lua": "vim.opt.number = true\n",
		},
	}

	for app, files := range apps {
		for path, content := range files {
			fullPath := filepath.Join(dir, app, path)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	return dir
}

func TestIsFlatStructureTrue(t *testing.T) {
	dir := setupFlatStructure(t)
	if !IsFlatStructure(dir) {
		t.Fatal("expected IsFlatStructure to return true for flat structure")
	}
}

func TestIsFlatStructureWithLayersDir(t *testing.T) {
	dir := setupFlatStructure(t)
	// Create a properly structured layers/ directory
	layersDir := filepath.Join(dir, "layers")
	if err := os.MkdirAll(filepath.Join(layersDir, "shell", "zsh"), 0755); err != nil {
		t.Fatal(err)
	}
	// Write a file to make layers/ non-empty
	_ = os.WriteFile(filepath.Join(layersDir, "shell", "zsh", ".zshrc"), []byte("data"), 0644)

	if IsFlatStructure(dir) {
		t.Fatal("expected IsFlatStructure to return false when layers/ has content")
	}
}

func TestIsFlatStructureEmpty(t *testing.T) {
	dir := t.TempDir()
	if IsFlatStructure(dir) {
		t.Fatal("expected IsFlatStructure to return false for empty directory")
	}
}

func TestIsFlatStructureOnlyHidden(t *testing.T) {
	dir := t.TempDir()
	// Create hidden directories (should be ignored)
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, ".config"), 0755)

	if IsFlatStructure(dir) {
		t.Fatal("expected IsFlatStructure to return false with only hidden dirs")
	}
}

func TestAnalyzeFlatStructure(t *testing.T) {
	dir := setupFlatStructure(t)
	result, err := Analyze(dir)
	if err != nil {
		t.Fatalf("Analyze() failed: %v", err)
	}
	if result == nil {
		t.Fatal("Analyze() returned nil")
	}
	// We should have at least zsh mapped to shell/zsh
	if _, ok := result.Moved["shell/zsh"]; !ok {
		t.Fatalf("expected shell/zsh in result.Moved, got %v", result.Moved)
	}
	// alacritty -> terminal/alacritty
	if _, ok := result.Moved["terminal/alacritty"]; !ok {
		t.Fatalf("expected terminal/alacritty in result.Moved, got %v", result.Moved)
	}
}

func TestAnalyzeNoAppDirs(t *testing.T) {
	dir := t.TempDir()
	result, err := Analyze(dir)
	if err != nil {
		t.Fatalf("Analyze() on empty dir failed: %v", err)
	}
	if len(result.Moved) > 0 {
		t.Fatalf("expected no moved dirs, got %v", result.Moved)
	}
}

func TestReorganizeCreatesLayerStructure(t *testing.T) {
	dir := setupFlatStructure(t)

	result, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := Reorganize(dir, result); err != nil {
		t.Fatalf("Reorganize() failed: %v", err)
	}

	// Check that layer directories were created
	checks := []string{
		"layers/shell/zsh/.zshrc",
		"layers/terminal/alacritty/.config/alacritty/alacritty.toml",
	}

	for _, path := range checks {
		fullPath := filepath.Join(dir, path)
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("expected %s to exist after reorganize: %v", path, err)
		}
	}

	// Verify original files were NOT modified
	origFiles := []string{
		"zsh/.zshrc",
		"alacritty/.config/alacritty/alacritty.toml",
	}
	for _, path := range origFiles {
		fullPath := filepath.Join(dir, path)
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("original file %s was removed: %v", path, err)
		}
	}
}

func TestReorganizeIdempotent(t *testing.T) {
	dir := setupFlatStructure(t)

	// Run reorganization twice
	result1, _ := Analyze(dir)
	if err := Reorganize(dir, result1); err != nil {
		t.Fatal(err)
	}

	result2, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Reorganize(dir, result2); err != nil {
		t.Fatalf("second Reorganize() failed: %v", err)
	}

	// Files should still exist
	check := filepath.Join(dir, "layers", "shell", "zsh", ".zshrc")
	if _, err := os.Stat(check); err != nil {
		t.Fatalf("file missing after second reorganize: %v", err)
	}
}

func TestDetectLayerDirsCreated(t *testing.T) {
	dir := setupFlatStructure(t)
	result, _ := Analyze(dir)
	_ = Reorganize(dir, result)

	// Check that detection layer dirs were created
	for _, layer := range []string{"distro/arch", "shell/bash", "device/desktop"} {
		d := filepath.Join(dir, "layers", layer)
		if _, err := os.Stat(d); os.IsNotExist(err) {
			t.Fatalf("detection layer %s was not created", layer)
		}
	}
}

func TestFormatResult(t *testing.T) {
	r := &Result{
		Moved:   map[string]int{"shell/zsh": 3, "base": 5},
		Created: []string{"shell/zsh", "base"},
		Orphans: []string{"unknown_app"},
		Skipped: []string{"empty_app (empty)"},
	}

	output := FormatResult(r)
	if !strings.Contains(output, "shell/zsh") {
		t.Fatal("expected FormatResult to contain layer name")
	}
	if !strings.Contains(output, "unknown_app") {
		t.Fatal("expected FormatResult to contain orphan name")
	}
	if !strings.Contains(output, "empty_app") {
		t.Fatal("expected FormatResult to contain skipped name")
	}
}

func TestCollectFiles(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	files := map[string]string{
		"a.txt":         "content a",
		"sub/b.txt":     "content b",
		".hidden":       "hidden",     // hidden files should be collected
		".git/config":   "git config", // .git should be skipped
		"sub/.git/HEAD": "ref",
	}

	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
		_ = os.WriteFile(fullPath, []byte(content), 0644)
	}

	collected := collectFiles(dir)

	// Should NOT include .git files
	for _, f := range collected {
		if strings.HasPrefix(f, ".git") || strings.Contains(f, "/.git/") {
			t.Fatalf("collectFiles should not include .git files, got %s", f)
		}
	}

	// Should include a.txt and sub/b.txt
	hasA := false
	hasB := false
	for _, f := range collected {
		if f == "a.txt" {
			hasA = true
		}
		if f == "sub/b.txt" {
			hasB = true
		}
	}
	if !hasA {
		t.Fatal("expected a.txt in collected files")
	}
	if !hasB {
		t.Fatal("expected sub/b.txt in collected files")
	}
}

func TestCountFiles(t *testing.T) {
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	_ = os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref"), 0644)

	count := countFiles(dir)
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestIsSubmodule(t *testing.T) {
	dir := t.TempDir()

	// Regular directory is not a submodule
	if isSubmodule(dir) {
		t.Fatal("expected regular dir to not be a submodule")
	}

	// Directory with .git directory is a submodule
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	if !isSubmodule(dir) {
		t.Fatal("expected dir with .git/ to be a submodule")
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copied")

	// Create source structure
	_ = os.MkdirAll(filepath.Join(src, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	_ = os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0644)
	_ = os.MkdirAll(filepath.Join(src, ".git"), 0755)
	_ = os.WriteFile(filepath.Join(src, ".git", "HEAD"), []byte("ref"), 0644)

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir() failed: %v", err)
	}

	// Verify files copied
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Fatal("a.txt not copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "sub", "b.txt")); err != nil {
		t.Fatal("sub/b.txt not copied")
	}
	// .git should NOT be copied
	if _, err := os.Stat(filepath.Join(dst, ".git", "HEAD")); err == nil {
		t.Fatal(".git should not have been copied")
	}
}
