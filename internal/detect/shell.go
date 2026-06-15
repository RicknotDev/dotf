package detect

import (
	"os"
	"path/filepath"
	"strings"
)

// detectShell reads the SHELL environment variable and returns the shell name.
// Uses the basename of the shell path (e.g., "/usr/bin/fish" -> "fish").
// Falls back to inspecting the parent process if SHELL is unset.
func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell != "" {
		return normalizeShell(shell)
	}

	// Fallback: check parent process
	ppid := os.Getenv("PPID")
	if ppid != "" {
		comm, err := os.ReadFile("/proc/" + ppid + "/comm")
		if err == nil {
			return strings.TrimSpace(string(comm))
		}
	}

	return ""
}

// normalizeShell extracts the basename and lowercases it.
func normalizeShell(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, ".exe")
	return strings.ToLower(name)
}
