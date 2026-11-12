// Package lock provides cross-process locking for DOTF operations.
//
// The lock is an advisory flock(2) on <stateDir>/dotf.lock. The kernel
// releases it when the holding process exits, so there is no "stale lock"
// guessing based on PIDs: if nobody holds the flock, the lock is free. The
// PID/hostname written into the file are informational only.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// LockFile is the name of the lock file.
const LockFile = "dotf.lock"

// Lock represents an acquired file lock.
type Lock struct {
	path string
	file *os.File
}

// Acquire takes the exclusive lock for the given state directory, waiting up
// to timeout for another process to release it.
func Acquire(stateDir string, timeout time.Duration) (*Lock, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create state directory: %w", err)
	}
	lockPath := filepath.Join(stateDir, LockFile)
	deadline := time.Now().Add(timeout)

	for {
		f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, fmt.Errorf("cannot open lock file: %w", err)
		}

		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// A previous holder may have unlinked the file between our open
			// and our flock. If the path no longer refers to the inode we
			// locked, start over with the new file.
			if sameFile(f, lockPath) {
				l := &Lock{path: lockPath, file: f}
				l.writeHolder()
				return l, nil
			}
			f.Close()
			continue
		}
		f.Close()

		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("cannot lock %s: %w", lockPath, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock held by %s (timeout after %v)", readLockHolder(lockPath), timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Release removes the lock file and drops the lock. The file is unlinked
// while still locked, so a waiter that opened the old inode notices the
// mismatch in Acquire and retries.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	os.Remove(l.path)
	syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	l.file.Close()
	l.file = nil
}

// IsLocked reports whether a live process currently holds the lock.
func IsLocked(stateDir string) bool {
	f, err := os.OpenFile(filepath.Join(stateDir, LockFile), os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return true
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}

// ForceRelease removes a leftover lock file. It refuses when a live process
// holds the lock, because removing the file would not stop that process and
// would let a second one start alongside it.
func ForceRelease(stateDir string) error {
	if IsLocked(stateDir) {
		return fmt.Errorf("lock is held by a running process (%s); stop it instead", LockHolder(stateDir))
	}
	err := os.Remove(filepath.Join(stateDir, LockFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LockHolder returns the informational holder line written into the lock
// file, or "unknown".
func LockHolder(stateDir string) string {
	return readLockHolder(filepath.Join(stateDir, LockFile))
}

func (l *Lock) writeHolder() {
	hostname, _ := os.Hostname()
	content := fmt.Sprintf("%d\n%s\n%s\n", os.Getpid(), hostname, time.Now().Format(time.RFC3339))
	if err := l.file.Truncate(0); err != nil {
		return
	}
	l.file.WriteAt([]byte(content), 0)
	l.file.Sync()
}

func sameFile(f *os.File, path string) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	pi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return os.SameFile(fi, pi)
}

func readLockHolder(lockPath string) string {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	switch {
	case len(lines) >= 2 && lines[0] != "":
		return fmt.Sprintf("PID %s on %s", strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1]))
	case len(lines) == 1 && lines[0] != "":
		return fmt.Sprintf("PID %s", strings.TrimSpace(lines[0]))
	}
	return "unknown"
}
