// Package backup manages versioned, checksummed backups of original files
// that are replaced by DOTF during installation.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxBackupVersions is the default number of backup versions to keep.
const MaxBackupVersions = 5

// Backup describes a single backup file.
type Backup struct {
	OriginalPath string // original file path (relative to home)
	BackupPath   string // absolute path to backup file
	Checksum     string // SHA-256 checksum
	Created      string // ISO 8601 timestamp
}

// Manager handles backup creation, restoration, and pruning.
type Manager struct {
	backupDir string
	maxKeep   int
}

// NewManager creates a new backup manager.
func NewManager(backupDir string) (*Manager, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create backup directory: %w", err)
	}
	return &Manager{
		backupDir: backupDir,
		maxKeep:   MaxBackupVersions,
	}, nil
}

// backupName generates a unique, timestamped backup filename.
func backupName(relativePath string) string {
	timestamp := time.Now().Format("20060102-150405")
	safeName := strings.ReplaceAll(relativePath, "/", "__")
	return fmt.Sprintf("%s.%s.bak", safeName, timestamp)
}

// Create creates a backup of a file before it's modified.
// The backup is written to the backup directory with a timestamp.
func (m *Manager) Create(relativePath, fullPath string) (*Backup, error) {
	// Verify the file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // nothing to back up
		}
		return nil, fmt.Errorf("cannot stat %s: %w", fullPath, err)
	}

	// Don't back up our own symlinks
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}

	backupFile := filepath.Join(m.backupDir, backupName(relativePath))

	// Read and checksum original
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", fullPath, err)
	}

	hash := sha256.Sum256(data)
	checksum := "sha256:" + hex.EncodeToString(hash[:])

	// Write backup
	if err := os.WriteFile(backupFile, data, 0644); err != nil {
		return nil, fmt.Errorf("cannot write backup %s: %w", backupFile, err)
	}

	b := &Backup{
		OriginalPath: relativePath,
		BackupPath:   backupFile,
		Checksum:     checksum,
		Created:      time.Now().Format(time.RFC3339),
	}

	// Prune old backups
	m.prune(relativePath)

	return b, nil
}

// List returns all backups for a given original path, sorted by date (newest first).
func (m *Manager) List(relativePath string) ([]Backup, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read backup directory: %w", err)
	}

	prefix := strings.ReplaceAll(relativePath, "/", "__") + "."
	var backups []Backup

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".bak") {
			continue
		}

		fullPath := filepath.Join(m.backupDir, entry.Name())
		checksum := computeChecksum(fullPath)
		// Extract timestamp from filename
		parts := strings.SplitN(entry.Name(), ".", 3)
		created := ""
		if len(parts) >= 3 {
			created = parts[1] // "20060102-150405"
		}

		backups = append(backups, Backup{
			OriginalPath: relativePath,
			BackupPath:   fullPath,
			Checksum:     checksum,
			Created:      created,
		})
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Created > backups[j].Created
	})

	return backups, nil
}

// ListAll returns all backup groups, organized by original path.
func (m *Manager) ListAll() (map[string][]Backup, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string][]Backup), nil
		}
		return nil, fmt.Errorf("cannot read backup directory: %w", err)
	}

	groups := make(map[string][]Backup)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bak") {
			continue
		}

		// Parse original path from filename: "path__to__file.TIMESTAMP.bak"
		name := strings.TrimSuffix(entry.Name(), ".bak")
		lastDot := strings.LastIndex(name, ".")
		if lastDot < 0 {
			continue
		}
		original := strings.ReplaceAll(name[:lastDot], "__", "/")
		timestamp := name[lastDot+1:]

		fullPath := filepath.Join(m.backupDir, entry.Name())
		checksum := computeChecksum(fullPath)

		groups[original] = append(groups[original], Backup{
			OriginalPath: original,
			BackupPath:   fullPath,
			Checksum:     checksum,
			Created:      timestamp,
		})
	}

	return groups, nil
}

// Verify checks the integrity of a backup by comparing checksums.
func (m *Manager) Verify(b Backup) (bool, error) {
	info, err := os.Stat(b.BackupPath)
	if err != nil {
		return false, fmt.Errorf("backup file missing: %s", b.BackupPath)
	}
	if info.IsDir() {
		return false, fmt.Errorf("backup path is a directory: %s", b.BackupPath)
	}

	currentChecksum := computeChecksum(b.BackupPath)
	return currentChecksum == b.Checksum, nil
}

// Restore restores a file from its backup.
func (m *Manager) Restore(b Backup, targetPath string) error {
	valid, err := m.Verify(b)
	if err != nil {
		return fmt.Errorf("cannot verify backup: %w", err)
	}
	if !valid {
		return fmt.Errorf("backup integrity check failed: %s", b.BackupPath)
	}

	data, err := os.ReadFile(b.BackupPath)
	if err != nil {
		return fmt.Errorf("cannot read backup: %w", err)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("cannot create target directory: %w", err)
	}

	// Write atomically via temp file
	tmpPath := targetPath + ".dotf-restore-tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("cannot write temp restore file: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot finalize restore: %w", err)
	}

	return nil
}

// prune removes old backups beyond the retention limit.
func (m *Manager) prune(relativePath string) {
	backups, err := m.List(relativePath)
	if err != nil || len(backups) <= m.maxKeep {
		return
	}

	// Remove oldest backups beyond the limit
	for i := m.maxKeep; i < len(backups); i++ {
		os.Remove(backups[i].BackupPath)
	}
}

// computeChecksum returns the SHA-256 checksum of a file.
func computeChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}
