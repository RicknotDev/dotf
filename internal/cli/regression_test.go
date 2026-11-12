package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// snapshot lists every path under dir (with symlink targets) so tests can
// check that nothing changed.
func snapshot(t *testing.T, dirs ...string) string {
	t.Helper()
	var out []string
	for _, dir := range dirs {
		filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			entry := p
			if info.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(p)
				entry += " -> " + target
			}
			out = append(out, entry)
			return nil
		})
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

// --dry-run must not write anything: not the state directory, not $HOME,
// not dotf.yaml in the repository, and it must not run hooks even with
// --allow-hooks.
func TestDryRunWritesNothing(t *testing.T) {
	repo := setupTestRepo(t)
	hooks := filepath.Join(repo, "layers", "base", "hooks")
	os.MkdirAll(hooks, 0o755)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	os.WriteFile(filepath.Join(hooks, "pre-install.sh"), []byte("touch "+marker+"\n"), 0o755)

	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(t.TempDir(), "state")
	chdir(t, repo)

	before := snapshot(t, repo, home)
	if err := Install([]string{"--dry-run", "--allow-hooks"}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if err := Apply([]string{"--dry-run"}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, repo, home); after != before {
		t.Fatalf("dry run changed files:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatal("dry run created the state directory")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("dry run executed a hook")
	}
	if _, err := os.Stat(filepath.Join(repo, "dotf.yaml")); !os.IsNotExist(err) {
		t.Fatal("dotf.yaml must not be generated")
	}
}

// Files under hooks/, packages/ and secrets/ are never linked into $HOME.
func TestInstallExcludesReservedDirs(t *testing.T) {
	repo := setupTestRepo(t)
	base := filepath.Join(repo, "layers", "base")
	for _, p := range []string{"hooks/post-install.sh", "packages/pacman.txt", "secrets/notes.txt"} {
		os.MkdirAll(filepath.Dir(filepath.Join(base, p)), 0o755)
		os.WriteFile(filepath.Join(base, p), []byte("x\n"), 0o644)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := t.TempDir()

	if err := Install([]string{"--no-packages", repo}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"hooks", "packages", "secrets"} {
		if _, err := os.Lstat(filepath.Join(home, d)); !os.IsNotExist(err) {
			t.Fatalf("~/%s was created", d)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(".zshrc not installed")
	}
}

// Hooks are only enabled by the command line, never by the repository.
func TestRepoCannotEnableHooks(t *testing.T) {
	t.Setenv("DOTF_ALLOW_HOOKS", "")
	repo := setupTestRepo(t)
	os.WriteFile(filepath.Join(repo, "dotf.yaml"), []byte("hooks:\n  enabled: true\n"), 0o644)
	hooks := filepath.Join(repo, "layers", "base", "hooks")
	os.MkdirAll(hooks, 0o755)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	os.WriteFile(filepath.Join(hooks, "post-install.sh"), []byte("touch "+marker+"\n"), 0o755)
	t.Setenv("HOME", t.TempDir())

	if err := Install([]string{"--no-packages", repo}, t.TempDir(), OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("hooks.enabled in dotf.yaml enabled hooks")
	}
}

func TestInstallRecordsRepositoryAndStatusWorksElsewhere(t *testing.T) {
	repo := setupTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	stateDir := t.TempDir()
	if err := Install([]string{"--no-packages", repo}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	chdir(t, t.TempDir()) // not a repository
	if err := Status(nil, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatalf("status outside the repo: %v", err)
	}
	if err := Explain(nil, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatalf("explain outside the repo: %v", err)
	}
	if err := Inspect([]string{"overrides"}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatalf("inspect outside the repo: %v", err)
	}
}

func TestStatusJSONExitCodeWithIssues(t *testing.T) {
	repo := setupTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := t.TempDir()
	if err := Install([]string{"--no-packages", repo}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(home, ".zshrc"))

	var out bytes.Buffer
	err := Status(nil, stateDir, OutputConfig{JSON: true, Stdout: &out})
	if ExitCode(err) != ExitFailure {
		t.Fatalf("status --json with a missing file: exit %d, want 1", ExitCode(err))
	}
	var res StatusResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if res.Summary.Missing != 1 {
		t.Fatalf("expected 1 missing, got %+v", res.Summary)
	}
	// Entries are sorted.
	for i := 1; i < len(res.Entries); i++ {
		if res.Entries[i-1].Path > res.Entries[i].Path {
			t.Fatal("status entries not sorted")
		}
	}
}

func TestStatusFilterByKey(t *testing.T) {
	repo := setupTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := t.TempDir()
	if err := Install([]string{"--no-packages", repo}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// "path:zshrc" must look at the path only.
	if err := Status(nil, stateDir, OutputConfig{JSON: true, Stdout: &out, Filter: "path:zshrc"}); err != nil {
		t.Fatal(err)
	}
	var res StatusResult
	json.Unmarshal(out.Bytes(), &res)
	if len(res.Entries) != 1 || res.Entries[0].Path != ".zshrc" {
		t.Fatalf("filter path:zshrc returned %+v", res.Entries)
	}
	if err := Status(nil, stateDir, OutputConfig{Quiet: true, Filter: "colour:red"}); ExitCode(err) != ExitUsage {
		t.Fatalf("unknown filter key should be a usage error, got %v", err)
	}
}

func TestParseGlobalFlagsEqualsAndPosition(t *testing.T) {
	cfg, rest := ParseGlobalFlags([]string{"--json", "status", "--filter=layer:base", "--short"})
	if !cfg.JSON || cfg.Filter != "layer:base" {
		t.Fatalf("got %+v", cfg)
	}
	if strings.Join(rest, " ") != "status --short" {
		t.Fatalf("remaining args %v", rest)
	}
}

func TestQuietStillShowsErrors(t *testing.T) {
	var errOut bytes.Buffer
	cfg := OutputConfig{Quiet: true, Stderr: &errOut}
	cfg.Info("progress\n")
	cfg.PrintErr("real error\n")
	if got := errOut.String(); got != "real error\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestUnapplyKeepsForeignAndModifiedFiles(t *testing.T) {
	repo := setupTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := t.TempDir()
	if err := Install([]string{"--no-packages", "--copy", repo}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	// The user edits one copied file.
	edited := filepath.Join(home, ".zshrc")
	os.WriteFile(edited, []byte("my edits\n"), 0o644)

	if err := Unapply(nil, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(edited); string(data) != "my edits\n" {
		t.Fatal("unapply removed a file the user modified")
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "alacritty", "alacritty.toml")); !os.IsNotExist(err) {
		t.Fatal("unapply did not remove an unmodified copy")
	}
}

// install -> status -> unapply -> restore puts the user's original back.
func TestInstallUnapplyRestoreRoundTrip(t *testing.T) {
	repo := setupTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := t.TempDir()
	orig := filepath.Join(home, ".zshrc")
	os.WriteFile(orig, []byte("original\n"), 0o600)
	os.Chmod(orig, 0o600)

	quiet := OutputConfig{Quiet: true}
	if err := Install([]string{"--no-packages", repo}, stateDir, quiet); err != nil {
		t.Fatal(err)
	}
	if !isSymlink(orig) {
		t.Fatal(".zshrc not linked")
	}
	if err := Status(nil, stateDir, quiet); err != nil {
		t.Fatalf("status after install: %v", err)
	}
	if err := Unapply(nil, stateDir, quiet); err != nil {
		t.Fatal(err)
	}
	if err := Restore([]string{".zshrc"}, stateDir, quiet); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(orig)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(orig); string(data) != "original\n" || info.Mode().Perm() != 0o600 {
		t.Fatalf("restored %q with mode %o", data, info.Mode().Perm())
	}
	if err := Restore([]string{".nope"}, stateDir, quiet); err == nil {
		t.Fatal("restoring a path without backups must fail")
	}
}

func TestDoctorFixRecreatesMissingLinks(t *testing.T) {
	repo := setupTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := t.TempDir()
	if err := Install([]string{"--no-packages", repo}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(home, ".zshrc"))

	if err := Doctor(nil, stateDir, OutputConfig{Quiet: true}); err == nil {
		t.Fatal("doctor should report the missing link")
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal("doctor without --fix changed something")
	}
	if err := Doctor([]string{"--fix"}, stateDir, OutputConfig{Quiet: true}); err != nil {
		t.Fatalf("doctor --fix: %v", err)
	}
	if !isSymlink(filepath.Join(home, ".zshrc")) {
		t.Fatal("doctor --fix did not recreate the link")
	}
}
