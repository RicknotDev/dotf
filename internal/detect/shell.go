package detect

import (
	"fmt"
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

	// Fallback: the parent process is usually the shell
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", os.Getppid()))
	if err == nil {
		return normalizeShell(strings.TrimSpace(string(comm)))
	}
	return ""
}

// normalizeShell extracts the basename and lowercases it.
func normalizeShell(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, ".exe")
	return strings.ToLower(name)
}
