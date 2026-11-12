// Package hook runs user-defined hook scripts (layers/<layer>/hooks/*.sh)
// before and after installation phases. Hooks are opt-in (--allow-hooks or
// DOTF_ALLOW_HOOKS=1), run with /bin/sh from the repository root, are killed
// with their whole process group after a timeout, and are logged. They are
// not otherwise sandboxed: an allowed hook runs with your privileges.
package hook

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// DefaultTimeout is the maximum time a hook can run.
const DefaultTimeout = 60 * time.Second

// HookType represents when a hook runs.
type HookType string

const (
	PreInstall  HookType = "pre-install"
	PostInstall HookType = "post-install"
	PreUpdate   HookType = "pre-update"
	PostUpdate  HookType = "post-update"
	PreRestore  HookType = "pre-restore"
	PostRestore HookType = "post-restore"
	OnError     HookType = "error"
)

// Hook describes an executable hook.
type Hook struct {
	Type    HookType
	Path    string // absolute path to the hook script
	Layer   string // which layer provides this hook
	Timeout time.Duration
}

// ExecutionResult contains the result of a hook execution.
type ExecutionResult struct {
	Hook     Hook
	Success  bool
	Output   string
	Duration time.Duration
	Error    error
}

// DiscoverHooks finds all hook scripts in resolved layers.
func DiscoverHooks(layerPaths []string) []Hook {
	var hooks []Hook

	for _, layerDir := range layerPaths {
		hooksDir := filepath.Join(layerDir, "hooks")
		entries, err := os.ReadDir(hooksDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			hookType := hookTypeFromFilename(name)
			if hookType == "" {
				continue
			}

			// Derive layer name from path
			layerName := deriveLayerName(layerDir)
			hookPath := filepath.Join(hooksDir, name)

			hooks = append(hooks, Hook{
				Type:    hookType,
				Path:    hookPath,
				Layer:   layerName,
				Timeout: DefaultTimeout,
			})
		}
	}

	// Sort by type priority: pre hooks first, post hooks last, error hooks in between
	sort.SliceStable(hooks, func(i, j int) bool {
		return hookPriority(hooks[i].Type) < hookPriority(hooks[j].Type)
	})

	return hooks
}

// Options control how hooks run.
type Options struct {
	RepoRoot string // working directory of the hook and $DOTF_DIR
	HomeDir  string // $DOTF_HOME
	Profile  string // $DOTF_PROFILE
	LogFile  string // appended to, mode 0600 ("" disables logging)
}

// Execute runs a single hook with /bin/sh in its own process group, so the
// timeout kills the whole tree it started, not just the shell.
func Execute(h Hook, opts Options) *ExecutionResult {
	start := time.Now()
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", h.Path)
	cmd.Dir = opts.RepoRoot
	cmd.Stdin = nil
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// Grandchildren may keep the output pipe open; don't wait for them forever.
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(),
		"DOTF_HOOK=true",
		fmt.Sprintf("DOTF_HOOK_TYPE=%s", h.Type),
		fmt.Sprintf("DOTF_HOOK_LAYER=%s", h.Layer),
		fmt.Sprintf("DOTF_PROFILE=%s", opts.Profile),
		fmt.Sprintf("DOTF_DIR=%s", opts.RepoRoot),
		fmt.Sprintf("DOTF_HOME=%s", opts.HomeDir),
	)

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %v", timeout)
	}
	duration := time.Since(start)
	result := &ExecutionResult{
		Hook:     h,
		Success:  err == nil,
		Output:   output.String(),
		Duration: duration,
		Error:    err,
	}

	if opts.LogFile != "" {
		entry := fmt.Sprintf("[%s] %s/%s: success=%v duration=%v\n  %s\n",
			time.Now().Format(time.RFC3339), h.Layer, h.Type,
			result.Success, duration.Round(time.Millisecond),
			strings.TrimSpace(result.Output))
		if err := os.MkdirAll(filepath.Dir(opts.LogFile), 0o700); err == nil {
			if f, err := os.OpenFile(opts.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				f.WriteString(entry)
				f.Close()
			}
		}
	}
	return result
}

// Select returns the hooks of the given type.
func Select(hooks []Hook, hookType HookType) []Hook {
	var out []Hook
	for _, h := range hooks {
		if h.Type == hookType {
			out = append(out, h)
		}
	}
	return out
}

// ExecuteAll runs all hooks of a given type in order and stops at the first
// failure. The caller decides whether hooks are allowed at all.
func ExecuteAll(hooks []Hook, hookType HookType, opts Options) ([]*ExecutionResult, error) {
	var results []*ExecutionResult
	for _, h := range Select(hooks, hookType) {
		r := Execute(h, opts)
		results = append(results, r)
		if !r.Success {
			return results, fmt.Errorf("hook %s/%s failed: %v", h.Layer, h.Type, r.Error)
		}
	}
	return results, nil
}

// Allowed reports whether hooks may run: only an explicit --allow-hooks or
// DOTF_ALLOW_HOOKS=1 enables them. A repository cannot enable its own hooks.
func Allowed(flag bool) bool {
	return flag || os.Getenv("DOTF_ALLOW_HOOKS") == "1"
}

// hookTypeFromFilename determines the hook type from the filename.
func hookTypeFromFilename(name string) HookType {
	name = strings.ToLower(strings.TrimSuffix(name, ".sh"))
	name = strings.TrimSuffix(name, ".hook")
	switch name {
	case "pre-install":
		return PreInstall
	case "post-install":
		return PostInstall
	case "pre-update":
		return PreUpdate
	case "post-update":
		return PostUpdate
	case "pre-restore":
		return PreRestore
	case "post-restore":
		return PostRestore
	case "error":
		return OnError
	default:
		return ""
	}
}

// deriveLayerName extracts the layer name from a layer directory path.
func deriveLayerName(layerDir string) string {
	// Layer dirs are like /path/to/repo/layers/base or /path/to/repo/layers/distro/arch
	// Find the "layers/" in the path to extract the relative layer path
	idx := strings.Index(layerDir, "layers/")
	if idx < 0 {
		// Try without trailing slash (e.g., just "layers")
		idx = strings.Index(layerDir, "/layers")
		if idx >= 0 {
			idx++ // skip the / before layers
		} else {
			return filepath.Base(layerDir)
		}
	}
	rel := layerDir[idx+len("layers/"):] // e.g., "distro/arch" or "base"
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) >= 2 && parts[0] != "base" {
		return parts[0] + "/" + parts[1]
	}
	if len(parts) >= 1 {
		return parts[0]
	}
	return filepath.Base(layerDir)
}

// hookPriority returns the execution priority for a hook type.
func hookPriority(t HookType) int {
	switch t {
	case PreInstall:
		return 0
	case PreUpdate:
		return 1
	case PreRestore:
		return 2
	case OnError:
		return 3
	case PostInstall:
		return 4
	case PostUpdate:
		return 5
	case PostRestore:
		return 6
	default:
		return 10
	}
}
