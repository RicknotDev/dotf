package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatalf("Acquire() failed: %v", err)
	}
	if l == nil {
		t.Fatal("Acquire() returned nil lock")
	}
	defer l.Release()

	// Lock file should exist
	lockPath := filepath.Join(dir, LockFile)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
}

func TestConcurrentAcquireFails(t *testing.T) {
	dir := t.TempDir()

	// Acquire first lock
	l1, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatalf("first Acquire() failed: %v", err)
	}
	defer l1.Release()

	// Second acquire should fail (no stale lock)
	l2, err := Acquire(dir, 100*time.Millisecond)
	if err == nil {
		l2.Release()
		t.Fatal("expected error for concurrent acquire")
	}
}

func TestReleaseRemovesLock(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	l.Release()

	lockPath := filepath.Join(dir, LockFile)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("lock file was not removed after release")
	}
}

func TestIsLocked(t *testing.T) {
	dir := t.TempDir()

	if IsLocked(dir) {
		t.Fatal("expected not locked initially")
	}

	l, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	if !IsLocked(dir) {
		t.Fatal("expected locked after acquire")
	}
}

func TestForceRelease(t *testing.T) {
	dir := t.TempDir()

	l, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	l.Release() // release without removing lock

	// Create stale lock file
	lockPath := filepath.Join(dir, LockFile)
	if err := os.WriteFile(lockPath, []byte("99999\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ForceRelease(dir); err != nil {
		t.Fatalf("ForceRelease() failed: %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("lock file was not removed by ForceRelease")
	}
}

func TestLockHolder(t *testing.T) {
	dir := t.TempDir()

	holder := LockHolder(dir)
	if holder != "unknown" {
		t.Fatalf("expected 'unknown', got %q", holder)
	}

	l, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	holder = LockHolder(dir)
	if holder == "unknown" {
		t.Fatal("expected lock holder info")
	}
}

func TestAcquireWithStaleLock(t *testing.T) {
	dir := t.TempDir()

	// Create a stale lock with a PID that doesn't exist
	lockPath := filepath.Join(dir, LockFile)
	if err := os.WriteFile(lockPath, []byte("99999\nstale\n2024-01-01T00:00:00Z\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should be able to acquire because the PID is stale
	l, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatalf("Acquire() with stale lock failed: %v", err)
	}
	l.Release()
}

func TestAcquireTimeout(t *testing.T) {
	dir := t.TempDir()

	// Create a lock that appears valid (we can't create a real one that stays)
	// Just verify the timeout path works
	l1, err := Acquire(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Release()

	// Try to acquire with very short timeout — this should fail
	_, err = Acquire(dir, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error for second acquire")
	}
}
