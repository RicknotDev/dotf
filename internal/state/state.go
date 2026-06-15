// Package state manages DOTF's persistent state with self-healing capabilities.
// State is never the sole source of truth — the filesystem is authoritative.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StateDir is the directory for DOTF state data.
const StateDir = "dotf"

// State represents the full DOTF state.
type State struct {
	Version        int                `json:"version"`
	Repository     string             `json:"repository"`
	LastInstall    string             `json:"last_install,omitempty"`
	InstalledLayers []string          `json:"installed_layers,omitempty"`
	InstalledFiles map[string]FileRef `json:"installed_files,omitempty"`
	BackupManifest map[string]Backup  `json:"backup_manifest,omitempty"`
}

// FileRef describes an installed file.
type FileRef struct {
	Layer    string `json:"layer"`
	Type     string `json:"type"` // "symlink" or "copy"
	Source   string `json:"source"`
	Checksum string `json:"checksum,omitempty"`
}

// Backup describes a backup file.
type Backup struct {
	Original string `json:"original"`
	Checksum string `json:"checksum"`
	Created  string `json:"created"`
}

// Manager handles state operations.
type Manager struct {
	stateDir  string
	statePath string
	backupDir string
	state     *State
	dirty     bool
}

// NewManager creates a new state manager.
func NewManager(baseDir string) (*Manager, error) {
	stateDir := filepath.Join(baseDir, StateDir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create state directory: %w", err)
	}

	backupDir := filepath.Join(baseDir, StateDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create backup directory: %w", err)
	}

	m := &Manager{
		stateDir:  stateDir,
		statePath: filepath.Join(stateDir, "state.json"),
		backupDir: backupDir,
		state: &State{
			Version:        1,
			InstalledFiles: make(map[string]FileRef),
			BackupManifest: make(map[string]Backup),
		},
	}

	// Load existing state if present
	m.load()

	return m, nil
}

// load reads state from disk. Missing/corrupt state is handled gracefully.
func (m *Manager) load() {
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		return // start fresh
	}

	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		return // corrupt state — start fresh
	}

	if loaded.InstalledFiles == nil {
		loaded.InstalledFiles = make(map[string]FileRef)
	}
	if loaded.BackupManifest == nil {
		loaded.BackupManifest = make(map[string]Backup)
	}

	m.state = &loaded
	m.dirty = false
}

// Save persists the state to disk.
func (m *Manager) Save() error {
	if !m.dirty {
		return nil
	}

	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal state: %w", err)
	}

	// Write atomically via temp file + rename
	tmpPath := m.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("cannot write state: %w", err)
	}
	if err := os.Rename(tmpPath, m.statePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot commit state: %w", err)
	}

	m.dirty = false
	return nil
}

// RecordInstall records a successful installation.
func (m *Manager) RecordInstall(layers []string) {
	m.dirty = true
	m.state.LastInstall = time.Now().Format(time.RFC3339)
	m.state.InstalledLayers = layers
}

// RecordFile records an installed file.
func (m *Manager) RecordFile(relativePath, layer, fileType, source string) {
	m.dirty = true
	checksum := ""
	if fileType == "copy" {
		checksum = computeChecksum(source)
	}
	m.state.InstalledFiles[relativePath] = FileRef{
		Layer:    layer,
		Type:     fileType,
		Source:   source,
		Checksum: checksum,
	}
}

// RemoveFile removes a file from the installed state.
func (m *Manager) RemoveFile(relativePath string) {
	m.dirty = true
	delete(m.state.InstalledFiles, relativePath)
}

// RecordBackup records a backup.
func (m *Manager) RecordBackup(backupName, originalPath string) {
	m.dirty = true
	checksum := computeChecksum(backupName)
	if _, err := os.Stat(backupName); err == nil {
		checksum = computeChecksum(backupName)
	}

	m.state.BackupManifest[backupName] = Backup{
		Original: originalPath,
		Checksum: checksum,
		Created:  time.Now().Format(time.RFC3339),
	}
}

// GetState returns a copy of the current state.
func (m *Manager) GetState() State {
	return *m.state
}

// RebuildFromFilesystem scans the home directory for DOTF-installed files.
// This is the self-healing mechanism — if state is corrupt, rebuild it.
func (m *Manager) RebuildFromFilesystem(repoRoot, homeDir string) error {
	newState := &State{
		Version:        1,
		Repository:     repoRoot,
		InstalledFiles: make(map[string]FileRef),
		BackupManifest: make(map[string]Backup),
	}

	// Start with existing backup manifest (backups are independent)
	newState.BackupManifest = m.state.BackupManifest

	// Walk home directory looking for symlinks pointing to the repo
	err := filepath.WalkDir(homeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if d.IsDir() {
			return nil
		}

		// Check if it's a symlink
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		target, err := os.Readlink(path)
		if err != nil {
			return nil
		}

		// Check if symlink points to our repository
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, absTarget)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil
		}

		// This is one of our symlinks
		homeRel, _ := filepath.Rel(homeDir, path)
		newState.InstalledFiles[homeRel] = FileRef{
			Type:   "symlink",
			Source: absTarget,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cannot walk filesystem: %w", err)
	}

	m.state = newState
	m.dirty = true
	return m.Save()
}

// VerifyBackupIntegrity checks all backup checksums.
func (m *Manager) VerifyBackupIntegrity() ([]string, error) {
	var corrupted []string

	for backupPath, backup := range m.state.BackupManifest {
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			corrupted = append(corrupted, fmt.Sprintf("%s: file missing", backupPath))
			continue
		}

		currentChecksum := computeChecksum(backupPath)
		if currentChecksum != backup.Checksum {
			corrupted = append(corrupted, fmt.Sprintf("%s: checksum mismatch", backupPath))
		}
	}

	return corrupted, nil
}

// StateDir returns the state directory path.
func (m *Manager) StateDir() string {
	return m.stateDir
}

// BackupDir returns the backup directory path.
func (m *Manager) BackupDir() string {
	return m.backupDir
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
