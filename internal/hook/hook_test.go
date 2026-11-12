package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeHook(t *testing.T, layerDir, name, body string) {
	t.Helper()
	p := filepath.Join(layerDir, "hooks", name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestHookRunsInRepoRoot(t *testing.T) {
	repo := t.TempDir()
	layer := filepath.Join(repo, "layers", "base")
	writeHook(t, layer, "post-install.sh", "pwd > \"$DOTF_HOME/pwd\"\n")
	home := t.TempDir()

	hooks := DiscoverHooks([]string{layer})
	if len(hooks) != 1 || hooks[0].Layer != "base" {
		t.Fatalf("unexpected hooks: %+v", hooks)
	}
	if _, err := ExecuteAll(hooks, PostInstall, Options{RepoRoot: repo, HomeDir: home}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(home, "pwd"))
	want, _ := filepath.EvalSymlinks(repo)
	if gotDir, _ := filepath.EvalSymlinks(strings.TrimSpace(string(got))); gotDir != want {
		t.Fatalf("hook ran in %q, want %q", got, want)
	}
}

// The timeout must kill everything the hook started, not just /bin/sh.
func TestHookTimeoutKillsProcessGroup(t *testing.T) {
	repo := t.TempDir()
	layer := filepath.Join(repo, "layers", "base")
	writeHook(t, layer, "pre-install.sh", "sleep 30 &\nsleep 30\n")
	hooks := DiscoverHooks([]string{layer})
	hooks[0].Timeout = 300 * time.Millisecond

	start := time.Now()
	_, err := ExecuteAll(hooks, PreInstall, Options{RepoRoot: repo})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("hook kept running for %v after its timeout", d)
	}
}

func TestExecuteAllStopsAtFirstFailure(t *testing.T) {
	repo := t.TempDir()
	a := filepath.Join(repo, "layers", "base")
	b := filepath.Join(repo, "layers", "shell", "zsh")
	writeHook(t, a, "pre-install.sh", "exit 3\n")
	writeHook(t, b, "pre-install.sh", "touch \"$DOTF_HOME/ran\"\n")
	home := t.TempDir()

	results, err := ExecuteAll(DiscoverHooks([]string{a, b}), PreInstall, Options{RepoRoot: repo, HomeDir: home})
	if err == nil {
		t.Fatal("expected failure")
	}
	if len(results) != 1 {
		t.Fatalf("expected to stop after the first hook, ran %d", len(results))
	}
	if _, err := os.Stat(filepath.Join(home, "ran")); !os.IsNotExist(err) {
		t.Fatal("second hook ran after the first failed")
	}
}

func TestAllowed(t *testing.T) {
	t.Setenv("DOTF_ALLOW_HOOKS", "")
	if Allowed(false) {
		t.Fatal("hooks allowed by default")
	}
	if !Allowed(true) {
		t.Fatal("--allow-hooks ignored")
	}
	t.Setenv("DOTF_ALLOW_HOOKS", "1")
	if !Allowed(false) {
		t.Fatal("DOTF_ALLOW_HOOKS=1 ignored")
	}
}
