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
	"time"
)

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
// baseDir should be the DOTF state directory (e.g., ~/.local/state/dotf).
func NewManager(baseDir string) (*Manager, error) {
	stateDir := baseDir
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create state directory: %w", err)
	}

	backupDir := filepath.Join(baseDir, "backups")
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

// ClearFiles removes all installed files from state.
func (m *Manager) ClearFiles() {
	m.dirty = true
	m.state.InstalledFiles = make(map[string]FileRef)
}

// RecordBackup records a backup.
func (m *Manager) RecordBackup(backupName, originalPath string) {
	m.dirty = true
	checksum := ""
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

// computeChecksum returns the SHA-256 checksum of a file.
func computeChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}
