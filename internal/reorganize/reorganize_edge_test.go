package reorganize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReorganizeAlreadyFormatted(t *testing.T) {
	// Create a directory that already has layers/ structure
	dir := t.TempDir()

	// Add some flat dirs to test
	os.MkdirAll(filepath.Join(dir, "zsh"), 0755)
	os.WriteFile(filepath.Join(dir, "zsh", ".zshrc"), []byte("data"), 0644)

	// Also create layers/ structure already
	os.MkdirAll(filepath.Join(dir, "layers", "shell", "zsh"), 0755)
	os.WriteFile(filepath.Join(dir, "layers", "shell", "zsh", ".zshrc"), []byte("layered data"), 0644)

	// IsFlatStructure should return false (already has layers/)
	if IsFlatStructure(dir) {
		t.Fatal("expected IsFlatStructure to return false for already-formatted structure")
	}
}

func TestReorganizePreservesOriginals(t *testing.T) {
	dir := t.TempDir()

	// Create flat dirs
	os.MkdirAll(filepath.Join(dir, "zsh"), 0755)
	zshContent := []byte("export ZSH=$HOME/.oh-my-zsh")
	os.WriteFile(filepath.Join(dir, "zsh", ".zshrc"), zshContent, 0644)

	os.MkdirAll(filepath.Join(dir, "alacritty", ".config", "alacritty"), 0755)
	alacrittyContent := []byte("[window]\nopacity = 0.95\n")
	os.WriteFile(filepath.Join(dir, "alacritty", ".config", "alacritty", "alacritty.toml"), alacrittyContent, 0644)

	// Run reorganization
	result, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := Reorganize(dir, result); err != nil {
		t.Fatalf("Reorganize() failed: %v", err)
	}

	// Verify originals still exist with same content
	origContent, err := os.ReadFile(filepath.Join(dir, "zsh", ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(origContent) != string(zshContent) {
		t.Fatalf("original zsh/.zshrc was modified")
	}

	origAlacritty, err := os.ReadFile(filepath.Join(dir, "alacritty", ".config", "alacritty", "alacritty.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(origAlacritty) != string(alacrittyContent) {
		t.Fatalf("original alacritty config was modified")
	}

	// Verify copies exist in layers/
	copiedZsh, err := os.ReadFile(filepath.Join(dir, "layers", "shell", "zsh", ".zshrc"))
	if err != nil {
		t.Fatalf("layer copy not created: %v", err)
	}
	if string(copiedZsh) != string(zshContent) {
		t.Fatalf("layer copy content mismatch")
	}
}

func TestReorganizeAmbiguityReported(t *testing.T) {
	dir := t.TempDir()

	// Create known mapped dirs
	os.MkdirAll(filepath.Join(dir, "zsh"), 0755)
	os.WriteFile(filepath.Join(dir, "zsh", ".zshrc"), []byte("data"), 0644)

	// Create unknown dirs (orphans)
	os.MkdirAll(filepath.Join(dir, "unknown_app"), 0755)
	os.WriteFile(filepath.Join(dir, "unknown_app", "config.conf"), []byte("data"), 0644)

	os.MkdirAll(filepath.Join(dir, "custom_tool"), 0755)
	os.WriteFile(filepath.Join(dir, "custom_tool", "settings.json"), []byte("{}"), 0644)

	// Analyze should report orphans
	result, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Orphans) == 0 {
		t.Fatal("expected orphans to be reported for unknown directories")
	}

	orphanFound := false
	for _, o := range result.Orphans {
		if o == "unknown_app" || o == "custom_tool" {
			orphanFound = true
		}
	}
	if !orphanFound {
		t.Fatalf("expected unknown_app or custom_tool in orphans, got %v", result.Orphans)
	}

	// FormatResult should mention orphans
	output := FormatResult(result)
	if !strings.Contains(output, "unknown_app") {
		t.Fatal("expected FormatResult to include orphan names")
	}
}

func TestReorganizeEmptyDir(t *testing.T) {
	dir := t.TempDir()

	// Create an empty flat dir (no files)
	os.MkdirAll(filepath.Join(dir, "zsh"), 0755)
	// Note: no files in zsh/

	result, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Empty dirs should be skipped
	if len(result.Moved) > 0 {
		t.Fatalf("expected no moved dirs for empty dir, got %v", result.Moved)
	}
}

func TestReorganizeOrphanWithFiles(t *testing.T) {
	dir := t.TempDir()

	// Create unknown dir with files (real orphan)
	os.MkdirAll(filepath.Join(dir, "my_app"), 0755)
	os.WriteFile(filepath.Join(dir, "my_app", "config.yml"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dir, "my_app", "settings.yml"), []byte("more"), 0644)

	result, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Moved) > 0 {
		t.Fatal("expected no moved files for unknown app")
	}

	// Should be reported as orphan
	found := false
	for _, o := range result.Orphans {
		if o == "my_app" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected my_app in orphans, got %v", result.Orphans)
	}
}

func TestReorganizeFlatStructureWithGitSubmodule(t *testing.T) {
	dir := t.TempDir()

	// Create a flat dir that looks like a git submodule (has .git)
	os.MkdirAll(filepath.Join(dir, "nvim"), 0755)
	// .git is a file for submodules, not a directory
	os.WriteFile(filepath.Join(dir, "nvim", ".git"), []byte("gitdir: ../.git/modules/nvim"), 0644)
	os.WriteFile(filepath.Join(dir, "nvim", "init.lua"), []byte("vim.opt.number=true"), 0644)
	os.MkdirAll(filepath.Join(dir, "nvim", "lua", "plugins"), 0755)
	os.WriteFile(filepath.Join(dir, "nvim", "lua", "plugins", "treesitter.lua"), []byte("return {}"), 0644)

	result, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	// nvim is not in AppLayerMap, so it should be an orphan
	foundNvim := false
	for _, o := range result.Orphans {
		if o == "nvim" {
			foundNvim = true
		}
	}
	if !foundNvim {
		t.Fatalf("expected nvim in orphans, got %v", result.Orphans)
	}
}
