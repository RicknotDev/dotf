// Package state stores what DOTF has installed (state.json in the state
// directory). The filesystem stays authoritative: commands compare the
// recorded state with what is actually on disk.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/RicknotDev/dotf/internal/backup"
)

// File types recorded in FileRef.Type.
const (
	TypeSymlink = "symlink"
	TypeCopy    = "copy"
	TypeSecret  = "secret"
)

// State represents the full DOTF state.
type State struct {
	Version         int                `json:"version"`
	Repository      string             `json:"repository"`
	LastInstall     string             `json:"last_install,omitempty"`
	InstalledLayers []string           `json:"installed_layers,omitempty"`
	InstalledFiles  map[string]FileRef `json:"installed_files,omitempty"`
	BackupManifest  map[string]Backup  `json:"backup_manifest,omitempty"`
}

// FileRef describes an installed file.
type FileRef struct {
	Layer string `json:"layer"`
	Type  string `json:"type"` // "symlink", "copy" or "secret"
	// Source is the repository file (for secrets: the encrypted file).
	Source string `json:"source"`
	// Checksum of the content placed in $HOME (copies and secrets).
	Checksum string `json:"checksum,omitempty"`
	// SourceChecksum of the encrypted file a secret was decrypted from.
	SourceChecksum string `json:"source_checksum,omitempty"`
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
	state     *State
	dirty     bool
	warning   string
}

// NewManager loads the state from baseDir (e.g. ~/.local/state/dotf).
// Nothing is written until Save, so read-only commands and dry runs leave
// the disk untouched. A corrupt state.json is moved aside to
// state.json.corrupt-<time> on the next Save and a warning is printed.
func NewManager(baseDir string) (*Manager, error) {
	m := &Manager{
		stateDir:  baseDir,
		statePath: filepath.Join(baseDir, "state.json"),
		state:     emptyState(),
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	if m.warning != "" {
		fmt.Fprintln(os.Stderr, "warning:", m.warning)
	}
	return m, nil
}

func emptyState() *State {
	return &State{
		Version:        1,
		InstalledFiles: make(map[string]FileRef),
		BackupManifest: make(map[string]Backup),
	}
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot read state: %w", err)
	}
	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		m.warning = fmt.Sprintf("state file %s is corrupt (%v); starting with empty state, the old file will be kept as %s.corrupt-*", m.statePath, err, m.statePath)
		m.dirty = true
		return nil
	}
	if loaded.InstalledFiles == nil {
		loaded.InstalledFiles = make(map[string]FileRef)
	}
	if loaded.BackupManifest == nil {
		loaded.BackupManifest = make(map[string]Backup)
	}
	if loaded.Version == 0 {
		loaded.Version = 1
	}
	m.state = &loaded
	return nil
}

// Warning returns the warning produced while loading ("" if none).
func (m *Manager) Warning() string { return m.warning }

// Save persists the state atomically (mode 0600 in a 0700 directory).
func (m *Manager) Save() error {
	if !m.dirty {
		return nil
	}
	if err := os.MkdirAll(m.stateDir, 0o700); err != nil {
		return fmt.Errorf("cannot create state directory: %w", err)
	}
	if m.warning != "" {
		if _, err := os.Stat(m.statePath); err == nil {
			corrupt := fmt.Sprintf("%s.corrupt-%s", m.statePath, time.Now().Format("20060102-150405"))
			if err := os.Rename(m.statePath, corrupt); err != nil {
				return fmt.Errorf("cannot keep corrupt state file: %w", err)
			}
		}
		m.warning = ""
	}
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal state: %w", err)
	}
	tmpPath := m.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("cannot write state: %w", err)
	}
	if err := os.Rename(tmpPath, m.statePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot commit state: %w", err)
	}
	m.dirty = false
	return nil
}

// RecordInstall records a successful installation of the given layers.
func (m *Manager) RecordInstall(layers []string) {
	m.dirty = true
	m.state.LastInstall = time.Now().Format(time.RFC3339)
	m.state.InstalledLayers = layers
}

// SetRepository records the repository the files were installed from.
func (m *Manager) SetRepository(repo string) {
	if m.state.Repository != repo {
		m.dirty = true
		m.state.Repository = repo
	}
}

// RecordFile records an installed symlink or copy.
func (m *Manager) RecordFile(relativePath, layer, fileType, source string) {
	ref := FileRef{Layer: layer, Type: fileType, Source: source}
	if fileType == TypeCopy {
		ref.Checksum = ComputeChecksum(source)
	}
	m.SetFile(relativePath, ref)
}

// SetFile records an installed file.
func (m *Manager) SetFile(relativePath string, ref FileRef) {
	m.dirty = true
	m.state.InstalledFiles[relativePath] = ref
}

// RemoveFile forgets an installed file.
func (m *Manager) RemoveFile(relativePath string) {
	if _, ok := m.state.InstalledFiles[relativePath]; ok {
		m.dirty = true
		delete(m.state.InstalledFiles, relativePath)
	}
}

// File returns the record for an installed file.
func (m *Manager) File(relativePath string) (FileRef, bool) {
	ref, ok := m.state.InstalledFiles[relativePath]
	return ref, ok
}

// Files returns the installed relative paths, sorted.
func (m *Manager) Files() []string {
	out := make([]string, 0, len(m.state.InstalledFiles))
	for k := range m.state.InstalledFiles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ClearFiles removes all installed files from state.
func (m *Manager) ClearFiles() {
	m.dirty = true
	m.state.InstalledFiles = make(map[string]FileRef)
}

// RecordBackup records a backup.
func (m *Manager) RecordBackup(backupPath, originalPath string) {
	m.dirty = true
	m.state.BackupManifest[backupPath] = Backup{
		Original: originalPath,
		Checksum: backupChecksum(backupPath),
		Created:  time.Now().Format(time.RFC3339),
	}
}

// GetState returns a copy of the current state.
func (m *Manager) GetState() State {
	s := *m.state
	s.InstalledFiles = make(map[string]FileRef, len(m.state.InstalledFiles))
	for k, v := range m.state.InstalledFiles {
		s.InstalledFiles[k] = v
	}
	s.BackupManifest = make(map[string]Backup, len(m.state.BackupManifest))
	for k, v := range m.state.BackupManifest {
		s.BackupManifest[k] = v
	}
	return s
}

// VerifyBackupIntegrity checks the checksums of all recorded backups.
func (m *Manager) VerifyBackupIntegrity() ([]string, error) {
	var corrupted []string
	keys := make([]string, 0, len(m.state.BackupManifest))
	for k := range m.state.BackupManifest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, backupPath := range keys {
		rec := m.state.BackupManifest[backupPath]
		if _, err := os.Lstat(backupPath); os.IsNotExist(err) {
			corrupted = append(corrupted, fmt.Sprintf("%s: file missing", backupPath))
			continue
		}
		if backupChecksum(backupPath) != rec.Checksum {
			corrupted = append(corrupted, fmt.Sprintf("%s: checksum mismatch", backupPath))
		}
	}
	return corrupted, nil
}

// backupChecksum matches backup.Checksum (symlink backups hash their target).
func backupChecksum(path string) string {
	sum, err := backup.Checksum(path)
	if err != nil {
		return ""
	}
	return sum
}

// ComputeChecksum returns "sha256:<hex>" of a file's content, or "" if it
// cannot be read.
func ComputeChecksum(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
