// Package hook provides sandboxed execution of user-defined hooks.
// Hooks are scripts that run before/after installation phases.
// They are opt-in, timed out, logged, and sandboxed by default.
package hook

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	Type     HookType
	Path     string // absolute path to the hook script
	Layer    string // which layer provides this hook
	Timeout  time.Duration
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
	sort.Slice(hooks, func(i, j int) bool {
		return hookPriority(hooks[i].Type) < hookPriority(hooks[j].Type)
	})

	return hooks
}

// Execute runs a hook with sandboxing.
func Execute(h Hook, logFile string, allowHooks bool) *ExecutionResult {
	if !allowHooks {
		return &ExecutionResult{
			Hook:    h,
			Success: false,
			Output:  "hooks are disabled (use --allow-hooks to enable)",
		}
	}

	start := time.Now()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), h.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", h.Path)

	// No stdin (prevent interactive hooks)
	cmd.Stdin = nil

	// Capture output
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output

	// Set environment with DOTF-specific variables
	cmd.Env = append(os.Environ(),
		"DOTF_HOOK=true",
		fmt.Sprintf("DOTF_HOOK_TYPE=%s", h.Type),
		fmt.Sprintf("DOTF_HOOK_LAYER=%s", h.Layer),
	)

	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecutionResult{
		Hook:     h,
		Success:  err == nil,
		Output:   output.String(),
		Duration: duration,
		Error:    err,
	}

	// Log to log file
	logEntry := fmt.Sprintf("[%s] %s/%s: success=%v duration=%v\n  %s\n",
		time.Now().Format(time.RFC3339),
		h.Layer, h.Type,
		result.Success, duration,
		strings.TrimSpace(result.Output))

	if logFile != "" {
		f, fErr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if fErr == nil {
			f.WriteString(logEntry)
			f.Close()
		}
	}

	return result
}

// ExecuteAll runs all hooks of a given type and returns results.
func ExecuteAll(hooks []Hook, hookType HookType, logFile string, allowHooks bool) []*ExecutionResult {
	var results []*ExecutionResult
	for _, h := range hooks {
		if h.Type == hookType {
			result := Execute(h, logFile, allowHooks)
			results = append(results, result)

			if !result.Success && hookType != OnError {
				if result.Error != nil {
					fmt.Fprintf(os.Stderr, "  hook %s/%s failed: %v\n", h.Layer, h.Type, result.Error)
				}
			}
		}
	}
	return results
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
