// Package lock provides cross-process locking for DOTF operations.
// Prevents concurrent installs, updates, restores, and migrations.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Lock represents an acquired file lock.
type Lock struct {
	path string
	file *os.File
}

// LockFile is the name of the lock file.
const LockFile = "dotf.lock"

// Acquire creates a lock for the given state directory.
// Blocks up to timeout trying to acquire the lock.
func Acquire(stateDir string, timeout time.Duration) (*Lock, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create state directory: %w", err)
	}

	lockPath := filepath.Join(stateDir, LockFile)

	deadline := time.Now().Add(timeout)
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
		if err == nil {
			// Write PID and hostname to lock file
			hostname, _ := os.Hostname()
			content := fmt.Sprintf("%d\n%s\n%s\n", os.Getpid(), hostname, time.Now().Format(time.RFC3339))
			if _, err := file.WriteString(content); err != nil {
				file.Close()
				os.Remove(lockPath)
				return nil, fmt.Errorf("cannot write lock file: %w", err)
			}
			if err := file.Sync(); err != nil {
				file.Close()
				os.Remove(lockPath)
				return nil, fmt.Errorf("cannot sync lock file: %w", err)
			}
			return &Lock{path: lockPath, file: file}, nil
		}

		if !os.IsExist(err) {
			return nil, fmt.Errorf("cannot acquire lock: %w", err)
		}

		// Lock exists — check if stale
		if isStaleLock(lockPath) {
			os.Remove(lockPath)
			continue
		}

		if time.Now().After(deadline) {
			// Read who holds the lock for the error message
			holder := readLockHolder(lockPath)
			return nil, fmt.Errorf("lock held by %s (timeout after %v)", holder, timeout)
		}

		time.Sleep(200 * time.Millisecond)
	}
}

// Release releases the lock.
func (l *Lock) Release() {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	if l.path != "" {
		os.Remove(l.path)
		l.path = ""
	}
}

// ForceRelease removes a stale lock file without validation.
func ForceRelease(stateDir string) error {
	lockPath := filepath.Join(stateDir, LockFile)
	return os.Remove(lockPath)
}

// IsLocked checks if a lock exists and is valid.
func IsLocked(stateDir string) bool {
	lockPath := filepath.Join(stateDir, LockFile)
	_, err := os.Stat(lockPath)
	if err != nil {
		return false
	}
	return !isStaleLock(lockPath)
}

// LockHolder returns information about who holds the lock.
func LockHolder(stateDir string) string {
	lockPath := filepath.Join(stateDir, LockFile)
	return readLockHolder(lockPath)
}

// isStaleLock checks if the lock file's PID is no longer running.
func isStaleLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return true // can't read, treat as stale
	}

	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) < 1 {
		return true
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return true // corrupted lock file
	}

	// Check if process exists by sending signal 0
	// On Unix, os.FindProcess is a no-op, so we use syscall.Kill
	if err := syscall.Kill(pid, syscall.Signal(0)); err != nil {
		// Process doesn't exist — stale lock
		return true
	}

	return false
}

// readLockHolder reads the PID from a lock file for error messages.
func readLockHolder(lockPath string) string {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}
