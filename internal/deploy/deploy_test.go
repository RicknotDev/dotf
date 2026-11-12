package deploy

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RicknotDev/dotf/internal/backup"
	"github.com/RicknotDev/dotf/internal/layer"
	"github.com/RicknotDev/dotf/internal/profile"
	"github.com/RicknotDev/dotf/internal/safety"
	"github.com/RicknotDev/dotf/internal/secret"
	"github.com/RicknotDev/dotf/internal/state"
	"github.com/RicknotDev/dotf/internal/transaction"
)

type env struct {
	repo, home, stateDir string
	layers               []layer.ResolvedLayer
}

func put(t *testing.T, path, content string, mode os.FileMode) {
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

func newEnv(t *testing.T, files map[string]string) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{
		repo:     filepath.Join(root, "repo"),
		home:     filepath.Join(root, "home"),
		stateDir: filepath.Join(root, "state"),
	}
	for p, c := range files {
		put(t, filepath.Join(e.repo, "layers", p), c, 0o644)
	}
	os.MkdirAll(e.home, 0o755)
	base := layer.ResolvedLayer{Type: profile.LayerBase, DirPath: filepath.Join(e.repo, "layers", "base")}
	e.layers = []layer.ResolvedLayer{base}
	if _, err := os.Stat(filepath.Join(e.repo, "layers", "shell", "zsh")); err == nil {
		e.layers = append(e.layers, layer.ResolvedLayer{Type: profile.LayerShell, Name: "zsh",
			DirPath: filepath.Join(e.repo, "layers", "shell", "zsh")})
	}
	return e
}

func (e *env) opts() Options {
	return Options{RepoRoot: e.repo, HomeDir: e.home, StateDir: e.stateDir, Layers: e.layers, Mode: Symlink}
}

func (e *env) run(t *testing.T, opts Options) (*Result, *state.Manager, error) {
	t.Helper()
	sm, err := state.NewManager(e.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	bm, _ := backup.NewManager(filepath.Join(e.stateDir, "backups"))
	plan, err := BuildPlan(opts, sm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Execute(opts, plan, sm, bm)
	if err == nil {
		if serr := sm.Save(); serr != nil {
			t.Fatal(serr)
		}
	}
	return res, sm, err
}

func TestReservedDirsAreNotDeployed(t *testing.T) {
	e := newEnv(t, map[string]string{
		"base/.zshrc":                  "zsh",
		"base/hooks/post-install.sh":   "echo hi",
		"base/packages/pacman.txt":     "git",
		"base/secrets/token.age":       "encrypted",
		"base/.git/config":             "[core]",
		"base/.config/app/hooks/x.ini": "nested hooks dir is a normal dir",
	})
	files, err := Collect(e.layers, false)
	if err != nil {
		t.Fatal(err)
	}
	for rel := range files {
		if strings.HasPrefix(rel, "hooks/") || strings.HasPrefix(rel, "packages/") ||
			strings.HasPrefix(rel, "secrets/") || strings.HasPrefix(rel, ".git/") {
			t.Fatalf("reserved path collected: %s", rel)
		}
	}
	if _, ok := files[".zshrc"]; !ok {
		t.Fatal(".zshrc missing")
	}
	if _, ok := files[".config/app/hooks/x.ini"]; !ok {
		t.Fatal("hooks/ below the layer root must be deployed")
	}
}

func TestHighestPriorityLayerWins(t *testing.T) {
	e := newEnv(t, map[string]string{
		"base/.zshrc":      "base",
		"shell/zsh/.zshrc": "zsh",
	})
	files, _ := Collect(e.layers, false)
	f := files[".zshrc"]
	if f.Layer != "shell/zsh" || len(f.Overridden) != 1 || f.Overridden[0] != "base" {
		t.Fatalf("wrong winner: %+v", f)
	}
}

// A failure in the middle of an install must undo the files already
// installed and restore what was there before.
func TestRollbackAfterMidInstallFailure(t *testing.T) {
	e := newEnv(t, map[string]string{
		"base/.a":                "a",
		"base/.b":                "b",
		"base/secrets/.zz.age":   "x",
		"base/.config/new/c.cfg": "c",
	})
	put(t, filepath.Join(e.home, ".a"), "user a", 0o600)

	old := secret.Decrypter
	secret.Decrypter = func(secret.Secret, io.Writer) error { return errors.New("boom") }
	t.Cleanup(func() { secret.Decrypter = old })

	_, _, err := e.run(t, e.opts())
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rolled-back error, got %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(e.home, ".a"))
	if string(data) != "user a" {
		t.Fatalf(".a not restored: %q", data)
	}
	if info, _ := os.Lstat(filepath.Join(e.home, ".a")); info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		t.Fatal(".a should be the user's 0600 file again")
	}
	for _, p := range []string{".b", ".config/new/c.cfg", ".config"} {
		if _, err := os.Lstat(filepath.Join(e.home, p)); !os.IsNotExist(err) {
			t.Fatalf("%s left behind after rollback", p)
		}
	}
	if n := len(transaction.Pending(e.stateDir)); n != 0 {
		t.Fatalf("journal left behind: %d", n)
	}
	// The backup made before the failure is kept.
	bm, _ := backup.NewManager(filepath.Join(e.stateDir, "backups"))
	if b, _ := bm.List(".a"); len(b) != 1 {
		t.Fatal("backup of .a missing")
	}
}

func TestInstallIsIdempotentAndPrunes(t *testing.T) {
	e := newEnv(t, map[string]string{"base/.a": "a", "base/.b": "b"})
	opts := e.opts()
	opts.Prune = true
	if _, _, err := e.run(t, opts); err != nil {
		t.Fatal(err)
	}
	res, _, err := e.run(t, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Unchanged != 2 || res.Created != 0 {
		t.Fatalf("second run should change nothing: %+v", res)
	}

	os.Remove(filepath.Join(e.repo, "layers", "base", ".b"))
	res, sm, err := e.run(t, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 {
		t.Fatalf("expected stale link removed: %+v", res)
	}
	if _, err := os.Lstat(filepath.Join(e.home, ".b")); !os.IsNotExist(err) {
		t.Fatal("stale link still present")
	}
	if _, ok := sm.File(".b"); ok {
		t.Fatal("stale file still in state")
	}
}

func TestForeignSymlinkIsBackedUp(t *testing.T) {
	e := newEnv(t, map[string]string{"base/.vimrc": "v"})
	os.Symlink("/elsewhere/vimrc", filepath.Join(e.home, ".vimrc"))
	res, _, err := e.run(t, e.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.BackedUp != 1 || res.Replaced != 1 {
		t.Fatalf("foreign symlink not backed up: %+v", res)
	}
}

func TestBackupFailureLeavesFileAlone(t *testing.T) {
	e := newEnv(t, map[string]string{"base/.a": "a"})
	put(t, filepath.Join(e.home, ".a"), "user", 0o644)
	// Make the backup directory impossible to create.
	put(t, filepath.Join(e.stateDir, "backups"), "not a dir", 0o644)

	res, _, err := e.run(t, e.opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "backup failed") {
		t.Fatalf("expected backup error, got %+v", res)
	}
	data, _ := os.ReadFile(filepath.Join(e.home, ".a"))
	if string(data) != "user" {
		t.Fatal("file replaced although its backup failed")
	}
}

func TestCopyModeKeepsPermissions(t *testing.T) {
	e := newEnv(t, nil)
	put(t, filepath.Join(e.repo, "layers", "base", ".ssh", "config"), "Host *", 0o600)
	put(t, filepath.Join(e.repo, "layers", "base", "bin", "tool"), "#!/bin/sh", 0o755)
	opts := e.opts()
	opts.Mode = Copy
	if _, _, err := e.run(t, opts); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]os.FileMode{".ssh/config": 0o600, "bin/tool": 0o755} {
		info, err := os.Lstat(filepath.Join(e.home, rel))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s: mode %o, want %o", rel, info.Mode().Perm(), want)
		}
	}
}

func TestSecretDeployedWith0600(t *testing.T) {
	e := newEnv(t, map[string]string{"base/secrets/.config/gh/hosts.yml.age": "enc"})
	old := secret.Decrypter
	secret.Decrypter = func(s secret.Secret, w io.Writer) error {
		_, err := w.Write([]byte("plaintext"))
		return err
	}
	t.Cleanup(func() { secret.Decrypter = old })

	_, sm, err := e.run(t, e.opts())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(e.home, ".config", "gh", "hosts.yml")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("secret mode %v, want regular 0600", info.Mode())
	}
	if ref, _ := sm.File(".config/gh/hosts.yml"); ref.Type != state.TypeSecret {
		t.Fatalf("secret recorded as %q", ref.Type)
	}
	// Unchanged on the second run.
	res, _, err := e.run(t, e.opts())
	if err != nil || res.Unchanged != 1 {
		t.Fatalf("second run: %+v %v", res, err)
	}
}

func TestPlanWritesNothing(t *testing.T) {
	e := newEnv(t, map[string]string{"base/.a": "a", "base/.config/x": "x"})
	sm, _ := state.NewManager(e.stateDir)
	plan, err := BuildPlan(e.opts(), sm)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Count(ActCreate) != 2 {
		t.Fatalf("expected 2 creates, got %+v", plan.Items)
	}
	if _, err := os.Stat(e.stateDir); !os.IsNotExist(err) {
		t.Fatal("planning created the state directory")
	}
	entries, _ := os.ReadDir(e.home)
	if len(entries) != 0 {
		t.Fatal("planning wrote to $HOME")
	}
}

// As root, $HOME is /root, which is in the sensitive list. Simulate that
// by marking the test home as sensitive: files must still be installed.
func TestInstallWhenHomeIsSensitive(t *testing.T) {
	e := newEnv(t, map[string]string{"base/.bashrc": "b"})
	old := safety.SensitivePaths
	safety.SensitivePaths = append([]string{e.home, filepath.Dir(e.home)}, old...)
	t.Cleanup(func() { safety.SensitivePaths = old })

	res, _, err := e.run(t, e.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("nothing installed with a sensitive home: %+v", res)
	}
}
