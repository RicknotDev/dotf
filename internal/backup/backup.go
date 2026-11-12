// Package backup keeps versioned, checksummed copies of files that DOTF
// replaces in $HOME.
//
// A backup of <rel> taken at Unix time <ns> (nanoseconds) is stored as
//
//	<backupDir>/<url.PathEscape(rel)>@<ns>.bak      the content (same mode as the original)
//	<backupDir>/<url.PathEscape(rel)>@<ns>.bak.sha256  "sha256:<hex>" recorded at creation
//
// Symlinks are backed up as symlinks with the same target. Files written by
// dotf <= 0.6 ("path__to__file.<ns>.bak") are still listed and restorable.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MaxBackupVersions is the default number of versions kept per file.
// Override with DOTF_BACKUP_KEEP.
const MaxBackupVersions = 5

const (
	suffix         = ".bak"
	checksumSuffix = ".bak.sha256"
)

// Backup describes a single backup.
type Backup struct {
	OriginalPath string    // path relative to $HOME
	BackupPath   string    // absolute path of the backup
	Checksum     string    // checksum recorded when the backup was made ("" for legacy backups)
	Created      string    // creation time in Unix nanoseconds, as a string
	Time         time.Time // creation time
	Symlink      bool      // the original was a symlink
}

// Manager handles backup creation, restoration, and pruning.
type Manager struct {
	backupDir string
	maxKeep   int
}

// NewManager returns a manager for backupDir. The directory is created
// (mode 0700) on the first backup, not here, so read-only commands and
// dry runs never write to disk.
func NewManager(backupDir string) (*Manager, error) {
	keep := MaxBackupVersions
	if v := os.Getenv("DOTF_BACKUP_KEEP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid DOTF_BACKUP_KEEP=%q (want a number >= 1)", v)
		}
		keep = n
	}
	return &Manager{backupDir: backupDir, maxKeep: keep}, nil
}

// Dir returns the backup directory.
func (m *Manager) Dir() string { return m.backupDir }

// backupName returns the file name for a backup of relativePath taken at ns.
func backupName(relativePath string, ns int64) string {
	return fmt.Sprintf("%s@%d%s", url.PathEscape(filepath.ToSlash(relativePath)), ns, suffix)
}

// parseName returns the original path and timestamp encoded in a backup
// file name, accepting both the current and the legacy format.
func parseName(name string) (original string, ns int64, ok bool) {
	if !strings.HasSuffix(name, suffix) {
		return "", 0, false
	}
	base := strings.TrimSuffix(name, suffix)
	if at := strings.LastIndex(base, "@"); at > 0 {
		if n, err := strconv.ParseInt(base[at+1:], 10, 64); err == nil {
			if orig, err := url.PathUnescape(base[:at]); err == nil {
				return orig, n, true
			}
		}
	}
	// Legacy: "path__to__file.<ns>.bak"
	dot := strings.LastIndex(base, ".")
	if dot <= 0 {
		return "", 0, false
	}
	n, err := strconv.ParseInt(base[dot+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return strings.ReplaceAll(base[:dot], "__", "/"), n, true
}

// Create backs up fullPath (a regular file or symlink). It returns nil and
// no error when fullPath does not exist. Any error means no backup was made
// and the caller must not modify fullPath.
func (m *Manager) Create(relativePath, fullPath string) (*Backup, error) {
	info, err := os.Lstat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot stat %s: %w", fullPath, err)
	}
	if err := os.MkdirAll(m.backupDir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create backup directory: %w", err)
	}

	now := time.Now()
	ns := now.UnixNano()
	var backupFile string
	for {
		backupFile = filepath.Join(m.backupDir, backupName(relativePath, ns))
		if _, err := os.Lstat(backupFile); os.IsNotExist(err) {
			break
		}
		ns++
	}

	var checksum string
	isLink := info.Mode()&os.ModeSymlink != 0
	switch {
	case isLink:
		target, err := os.Readlink(fullPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read symlink %s: %w", fullPath, err)
		}
		if err := os.Symlink(target, backupFile); err != nil {
			return nil, fmt.Errorf("cannot write backup %s: %w", backupFile, err)
		}
		checksum = linkChecksum(target)
	case info.Mode().IsRegular():
		checksum, err = copyWithChecksum(fullPath, backupFile, info.Mode().Perm())
		if err != nil {
			os.Remove(backupFile)
			return nil, fmt.Errorf("cannot write backup %s: %w", backupFile, err)
		}
	default:
		return nil, fmt.Errorf("cannot back up %s: not a regular file or symlink", fullPath)
	}

	if err := os.WriteFile(backupFile+".sha256", []byte(checksum+"\n"), 0o600); err != nil {
		os.Remove(backupFile)
		return nil, fmt.Errorf("cannot write backup checksum: %w", err)
	}

	b := &Backup{
		OriginalPath: relativePath,
		BackupPath:   backupFile,
		Checksum:     checksum,
		Created:      strconv.FormatInt(ns, 10),
		Time:         time.Unix(0, ns),
		Symlink:      isLink,
	}
	m.prune(relativePath)
	return b, nil
}

// List returns the backups of exactly relativePath, newest first.
func (m *Manager) List(relativePath string) ([]Backup, error) {
	all, err := m.ListAll()
	if err != nil {
		return nil, err
	}
	return all[filepath.ToSlash(filepath.Clean(relativePath))], nil
}

// ListAll returns all backups grouped by original path, each group sorted
// newest first.
func (m *Manager) ListAll() (map[string][]Backup, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]Backup{}, nil
		}
		return nil, fmt.Errorf("cannot read backup directory: %w", err)
	}
	groups := make(map[string][]Backup)
	for _, e := range entries {
		original, ns, ok := parseName(e.Name())
		if !ok || e.IsDir() {
			continue
		}
		full := filepath.Join(m.backupDir, e.Name())
		groups[original] = append(groups[original], Backup{
			OriginalPath: original,
			BackupPath:   full,
			Checksum:     readChecksum(full),
			Created:      strconv.FormatInt(ns, 10),
			Time:         time.Unix(0, ns),
			Symlink:      e.Type()&os.ModeSymlink != 0,
		})
	}
	for k := range groups {
		g := groups[k]
		sort.Slice(g, func(i, j int) bool { return g[i].Time.After(g[j].Time) })
	}
	return groups, nil
}

// Verify compares the backup with the checksum recorded when it was made.
// Legacy backups have no recorded checksum and are reported as valid.
func (m *Manager) Verify(b Backup) (bool, error) {
	info, err := os.Lstat(b.BackupPath)
	if err != nil {
		return false, fmt.Errorf("backup file missing: %s", b.BackupPath)
	}
	if info.IsDir() {
		return false, fmt.Errorf("backup path is a directory: %s", b.BackupPath)
	}
	if b.Checksum == "" {
		b.Checksum = readChecksum(b.BackupPath)
		if b.Checksum == "" {
			return true, nil
		}
	}
	current, err := Checksum(b.BackupPath)
	if err != nil {
		return false, err
	}
	return current == b.Checksum, nil
}

// Restore writes the backup to targetPath (verifying it first), keeping the
// original mode. Prefer restoring through a transaction; see Open.
func (m *Manager) Restore(b Backup, targetPath string) error {
	if err := m.check(b); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("cannot create target directory: %w", err)
	}
	tmp := targetPath + ".dotf-restore-tmp"
	os.Remove(tmp)
	if b.Symlink {
		target, err := os.Readlink(b.BackupPath)
		if err != nil {
			return err
		}
		if err := os.Symlink(target, tmp); err != nil {
			return err
		}
	} else {
		info, err := os.Stat(b.BackupPath)
		if err != nil {
			return err
		}
		if _, err := copyWithChecksum(b.BackupPath, tmp, info.Mode().Perm()); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("cannot write temp restore file: %w", err)
		}
	}
	if err := os.Rename(tmp, targetPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cannot finalize restore: %w", err)
	}
	return nil
}

func (m *Manager) check(b Backup) error {
	valid, err := m.Verify(b)
	if err != nil {
		return fmt.Errorf("cannot verify backup: %w", err)
	}
	if !valid {
		return fmt.Errorf("backup integrity check failed: %s", b.BackupPath)
	}
	return nil
}

// Content describes a verified backup so it can be restored through a
// transaction: either a symlink target or a readable file with its mode.
type Content struct {
	LinkTarget string // non-empty for symlink backups
	Mode       os.FileMode
	Open       func() (io.ReadCloser, error)
}

// Open verifies b and returns its content.
func (m *Manager) Open(b Backup) (*Content, error) {
	if err := m.check(b); err != nil {
		return nil, err
	}
	if b.Symlink {
		target, err := os.Readlink(b.BackupPath)
		if err != nil {
			return nil, err
		}
		return &Content{LinkTarget: target}, nil
	}
	info, err := os.Stat(b.BackupPath)
	if err != nil {
		return nil, err
	}
	return &Content{
		Mode: info.Mode().Perm(),
		Open: func() (io.ReadCloser, error) { return os.Open(b.BackupPath) },
	}, nil
}

// prune removes the oldest backups of relativePath beyond the limit.
func (m *Manager) prune(relativePath string) {
	backups, err := m.List(relativePath)
	if err != nil || len(backups) <= m.maxKeep {
		return
	}
	for _, b := range backups[m.maxKeep:] {
		if err := os.Remove(b.BackupPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not prune old backup %s: %v\n", b.BackupPath, err)
			continue
		}
		os.Remove(b.BackupPath + ".sha256")
	}
}

// Checksum returns "sha256:<hex>" for a regular file, or a checksum of the
// target for a symlink.
func Checksum(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		return linkChecksum(target), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func linkChecksum(target string) string {
	h := sha256.Sum256([]byte("symlink:" + target))
	return "sha256:" + hex.EncodeToString(h[:])
}

func readChecksum(backupPath string) string {
	data, err := os.ReadFile(backupPath + ".sha256")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// copyWithChecksum copies src to a new file dst with mode and returns the
// checksum of what was written.
func copyWithChecksum(src, dst string, mode os.FileMode) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(dst, mode)
	}
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
