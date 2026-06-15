# DOTF — Hardened Architecture

## Overview

DOTF is a zero-configuration Linux setup runtime. This document describes the hardened architecture after a comprehensive security, reliability, and production-readiness review.

---

## 1. Transaction System

### Design

Every mutating operation (install, update, restore, uninstall) runs inside a **transaction** with three phases:

```
BEGIN → EXECUTE → VALIDATE → COMMIT / ROLLBACK
```

### Implementation

- **Operation journal**: Each file operation is recorded in a journal file (`~/.local/state/dotf/journal`) before execution.
- **Write-ahead log**: Operations are logged *before* execution (WAL pattern).
- **Rollback**: If any operation fails, the journal is walked in reverse to undo all completed operations.
- **Crash recovery**: On startup, check for incomplete journals and either roll forward or roll back.

### Transaction boundaries

| Operation | Begin | Commit |
|-----------|-------|--------|
| Symlink creation | Log `CREATE symlink path→target` | Mark complete |
| File backup | Log `BACKUP original→backup` | Mark complete |
| File copy | Log `COPY source→target` | Mark complete |
| Directory creation | Log `MKDIR path` | Mark complete |

### Failure handling

- Power loss: On next run, detect incomplete transaction → rollback all logged uncommitted ops.
- Disk full: Catch ENOSPC, abort transaction, rollback.
- User interrupt (SIGINT/SIGTERM): Graceful abort and rollback.
- Process crash: Same as power loss — journal recovery on restart.

### Recovery procedures

```
dotf doctor --fix   # Detect and repair incomplete transactions
```

---

## 2. Path Safety Engine

### Design

All paths are validated through a centralized safety layer before any filesystem operation.

### Rules

1. **No path traversal**: Reject any path containing `..` or `.` components that escape the target directory.
2. **No absolute paths from repository**: Reject symlink targets that are absolute paths (from repo files), unless explicitly allowed.
3. **No symlink loops**: Detect and reject chains longer than 40 hops.
4. **No self-references**: Reject symlinks that point back to themselves.
5. **No sensitive paths**: Block symlinks targeting `/etc/passwd`, `/etc/shadow`, `/root/`, `/home/*/.ssh/authorized_keys`, etc.
6. **No repository escape**: All layer file paths must resolve within the repository root.
7. **Normalize all paths**: Use `filepath.Clean` and `filepath.Abs` before validation.

### Sensitive path list

```
/etc/passwd
/etc/shadow
/etc/sudoers
/etc/sudoers.d/
/root/
/home/*/.ssh/authorized_keys
/home/*/.gnupg/
/home/*/.config/systemd/
/home/*/.bashrc       (allowed with confirmation)
```

### Algorithm

```
func SafePath(repoRoot, homeDir, layerFile string) error {
    // 1. Resolve layer file to absolute, clean path
    // 2. Verify it's within repoRoot
    // 3. Compute relative path within layer
    // 4. Compute target path within homeDir
    // 5. Verify target is within homeDir
    // 6. Check target against sensitive path list
    // 7. Return error or nil
}
```

---

## 3. Symlink Security

### Design

Treat repository symlinks as untrusted. Validate every symlink before creating it.

### Validation chain

1. **Source validation**: Layer file must exist and be a regular file (not a symlink from the repo).
2. **Target validation**: Target path must be within `$HOME` (or configured DOTF_HOME).
3. **Chain validation**: If the layer file is a symlink, follow and validate the chain (max 40 hops).
4. **No privileged paths**: Block symlinks to `/etc`, `/usr`, `/bin`, `/boot`, `/dev`, `/proc`, `/sys`.
5. **No self-references**: Detect and reject.

### Behavior

- Violation → hard error, transaction rollback, clear user message.
- No silent failure. No partial installation.

---

## 4. Locking System

### Design

Prevent concurrent DOTF operations. One lock per repository.

### Implementation

- **Lock file**: `~/.local/state/dotf/lock` (PID-based).
- **Stale detection**: If lock is held by a PID that no longer exists, reclaim it.
- **Timeout**: Acquire lock with 30-second timeout; fail if not acquired.
- **Cross-process**: Uses `flock`-style file locking via `os.OpenFile` with O_CREATE and O_EXCL.
- **Graceful handling**: Clear message if lock is held: "Another DOTF process is running (PID %d). Wait or run `dotf doctor --unlock`."

### Recovery

```
dotf doctor --unlock   # Force release stale lock
```

---

## 5. State Storage

### Design

DOTF maintains a state file for reconciliation and audit. State is never the sole source of truth — the filesystem is.

### State file

`~/.local/state/dotf/state.json`

```json
{
  "version": 1,
  "repository": "/home/user/.dotfiles",
  "last_install": "2026-06-15T12:00:00Z",
  "installed_layers": ["base", "distro/arch", "wm/hyprland"],
  "installed_files": {
    ".config/kitty/kitty.conf": {
      "layer": "distro/arch",
      "type": "symlink",
      "source": "/home/user/.dotfiles/layers/distro/arch/.config/kitty/kitty.conf",
      "checksum": "sha256:..."
    }
  },
  "backup_manifest": {
    ".config/kitty/kitty.conf.20260615-120000.bak": {
      "original": ".config/kitty/kitty.conf",
      "checksum": "sha256:..."
    }
  }
}
```

### Self-healing

- If state is corrupted or missing, rebuild it by scanning `$HOME` for symlinks pointing to the repository.
- Checksums verify file integrity.
- `dotf doctor` detects and repairs state inconsistencies.

---

## 6. Backup Architecture

### Design

Every existing file that would be overwritten is backed up before modification.

### Backup format

- **Location**: `~/.local/share/dotf/backups/`
- **Naming**: `{relative_path}.{timestamp}.bak`
- **Metadata**: State file records backup-to-original mapping with checksums.

### Versioning

- Multiple backups of the same file are retained (timestamped).
- Default: keep last 5 versions.
- Configurable: `DOTF_BACKUP_KEEP=10`.

### Integrity

- SHA-256 checksum computed at backup time.
- Checksum stored in state.
- `dotf doctor --verify-backups` checks all backup integrity.

### Crash safety

- Backup is created *before* the target file is modified (WAL order).
- If crash occurs between backup and symlink creation, the backup is preserved and the incomplete transaction is rolled back.

---

## 7. Restore Architecture

### Design

Restore returns files to their pre-DOTF state using backups.

### Workflow

```
dotf restore                    # Interactive: show available restores
dotf restore --all              # Restore all backed-up files
dotf restore .config/kitty/     # Restore specific files or directories
dotf restore --preview          # Show what would be restored
```

### Preview mode

- Shows: file, backup date, original checksum, current state.
- Allows user to confirm before proceeding.

### Partial restore

- Restore specific files by path pattern.
- Restore from specific backup timestamp.
- Restore specific layers.

### Validation

- Verify backup checksum before restore.
- Confirm target file hasn't been modified since DOTF installed it (compare checksum).
- If target was modified, warn: "File was modified after DOTF installed it. Overwrite? [y/N]"

---

## 8. Hook System

### Design

Hooks are scripts that run before/after installation phases. They are potentially dangerous and must be sandboxed.

### Hook types

| Type | Timing | Default |
|------|--------|---------|
| `pre-install` | Before any file operations | Disabled |
| `post-install` | After all file operations | Enabled |
| `pre-update` | Before update | Disabled |
| `post-update` | After update | Enabled |
| `pre-restore` | Before restore | Disabled |
| `post-restore` | After restore | Enabled |
| `error` | On transaction failure | Always runs |

### Hook location

```
layers/base/hooks/pre-install.sh
layers/base/hooks/post-install.sh
layers/base/hooks/error.sh
```

### Permission model

1. **Explicit opt-in**: No hooks run by default. User must run `dotf install --allow-hooks` or set `DOTF_ALLOW_HOOKS=1`.
2. **Visibility**: All hooks are printed with full path before execution.
3. **Confirmation**: `dotf doctor --list-hooks` shows all available hooks.
4. **Timeout**: Hooks are killed after 60 seconds (configurable).
5. **Logging**: All hook output is captured to `~/.local/state/dotf/hooks.log`.
6. **No input**: Hooks run with stdin closed.
7. **Resource limits**: Hooks run with `RLIMIT_NPROC` and `RLIMIT_NOFILE` limits.

### Execution

```go
func executeHook(name, path string, env []string, timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    cmd := exec.CommandContext(ctx, "/bin/sh", path)
    cmd.Env = append(os.Environ(), env...)
    cmd.Stdin = nil
    cmd.Stdout = logWriter
    cmd.Stderr = logWriter
    return cmd.Run()
}
```

---

## 9. Secret Management

### Design

Support age and GPG for encrypted secrets. Secrets are decrypted only for the minimum required time.

### Secret file format

```
layers/base/secrets/github_token.age   # age-encrypted
layers/base/secrets/ssh_key.gpg         # GPG-encrypted
```

### Decryption

- Decrypted to a `tmpfs` / `memfd` file (never written to disk).
- Deleted immediately after use.
- Permission: `0600` while decrypted.

### Lifecycle

1. **Discovery**: Scan `secrets/` directories in resolved layers.
2. **Identity**: Use `~/.config/dotf/keys/` for age keys, `gpg --decrypt` for GPG.
3. **Decrypt**: Decrypt to `memfd` (Linux 3.17+ `memfd_create`).
4. **Deploy**: Symlink/copy decrypted file to target.
5. **Destroy**: Close memfd, zero memory.

### Safety

- Never log decrypted content.
- Never write decrypted content to persistent storage.
- `dotf explain --secrets` shows which secrets would be deployed (names only, no content).
- `dotf doctor --check-secrets` verifies encryption keys exist.

---

## 10. Package Abstraction Layer

### Design

Abstract package management behind a unified interface. Support all major package managers.

### Interface

```go
type PackageManager interface {
    Name() string
    Available() bool                          // Is this PM installed?
    Install(pkgs []string) error              // Install packages
    Remove(pkgs []string) error               // Remove packages
    Update() error                            // Update package database
    Upgrade() error                           // Upgrade all packages
    ListInstalled() ([]string, error)         // List installed packages
    IsInstalled(pkg string) (bool, error)     // Check if package is installed
}
```

### Supported managers

| Manager | Detection | Implementation |
|---------|-----------|----------------|
| pacman | `which pacman` | `pacman -S --noconfirm` |
| paru | `which paru` | `paru -S --noconfirm` |
| yay | `which yay` | `yay -S --noconfirm` |
| apt | `which apt` | `apt install -y` |
| dnf | `which dnf` | `dnf install -y` |
| zypper | `which zypper` | `zypper install -y` |
| nix | `which nix-env` | `nix-env -iA` |

### Package list format

```
layers/base/packages/pacman.txt
layers/base/packages/apt.txt
```

One package per line. Lines starting with `#` are comments.

### Error handling

- Capture stderr.
- Report specific package failures.
- On failure: offer retry, continue with remaining packages, or abort.
- Partial installs are recorded so retries skip already-installed packages.

### Conflicts

- Detect conflicting packages before installation.
- `dotf doctor --check-packages` validates all package lists.

---

## 11. Merge Engine

### Design

When multiple layers provide the same file, DOTF must handle merges safely. For binary/text files, the highest-priority layer wins (no merge). For structured formats, attempt a merge but validate results.

### Merge rules

| Format | Strategy |
|--------|----------|
| Binary | Winner takes all (highest priority) |
| Text | Winner takes all |
| YAML | Deep merge if valid YAML; fall back to winner if invalid |
| JSON | Deep merge if valid JSON; fall back to winner if invalid |
| TOML | Deep merge if valid TOML; fall back to winner if invalid |
| INI/conf | Section-level merge; duplicate keys use winner |

### Validation

- Merged output must be valid in the target format.
- If merge produces invalid output, fall back to winner (with warning).
- Users can opt out of merging: `dotf install --no-merge`.

### Implementation

```go
func Merge(files []MergeFile) ([]byte, error) {
    // Detect format from extension
    // Parse all files
    // Deep merge in priority order
    // Validate output
    // Return merged bytes or error
}
```

---

## 12. Explainability Subsystem

### Commands

| Command | Description |
|---------|-------------|
| `dotf explain` | Show detected environment, resolved layers, file overrides |
| `dotf doctor` | Run diagnostics, check integrity, repair issues |
| `dotf inspect` | Deep inspection of specific files or layers |

### `dotf doctor` checks

1. **Repository integrity**: Verify layers directory structure
2. **State integrity**: Verify state file, rebuild if needed
3. **Lock integrity**: Check for stale locks
4. **Backup integrity**: Verify backup checksums
5. **Symlink integrity**: Check all installed symlinks
6. **Path safety**: Scan repository for unsafe paths
7. **Package availability**: Check package managers
8. **Secret availability**: Check encryption keys
9. **Hook availability**: List all hooks
10. **Transaction integrity**: Check for incomplete transactions

### `dotf inspect` subcommands

```
dotf inspect file .config/kitty/kitty.conf   # Show which layer provides this file
dotf inspect layer distro/arch                # Show all files in a layer
dotf inspect overrides                        # Show all overrides with details
dotf inspect backup                           # Show backup manifest
dotf inspect state                            # Show full state
```

---

## 13. Malicious Repository Defense

### Defense layers

1. **Path validation**: Block traversal, escape, self-references, loops (Safety Engine).
2. **Symlink validation**: Block privileged paths, chain attacks.
3. **Hook restrictions**: Opt-in only, timeouts, logging, no stdin.
4. **Secret restrictions**: No plaintext secrets in repo, encrypted only.
5. **Package restrictions**: Warn on dangerous packages, require confirmation.
6. **File type restrictions**: Block executable files in config directories by default.
7. **Permission restrictions**: Never create SUID/SGID files.
8. **Size restrictions**: Warn on files > 10MB in the repository.

### Audit trail

- All operations logged to `~/.local/state/dotf/audit.log`.
- Log format: `timestamp operation path result`.
- Never deleted, only rotated.

---

## 14. Scalability Strategy

### Performance targets

- Layer resolution: < 100ms for 50 layers
- Status checks: < 500ms for 10k files
- Backup operations: < 1s for 1k files
- Startup: < 50ms

### Optimizations

- **Lazy file walking**: Don't walk layers until needed. Walk only when resolving overrides or installing.
- **Parallel layer scanning**: Walk layers in parallel using goroutines.
- **Cached directory listings**: Cache directory listings within a single transaction.
- **Efficient override computation**: Use filepath.WalkDir with early exit when only checking existence.

---

## 15. Disaster Recovery Strategy

### Scenarios

| Scenario | Recovery |
|----------|----------|
| Power loss during install | Run `dotf doctor --fix` → rollback incomplete transaction |
| State file corruption | `dotf doctor --fix` → rebuild state by scanning filesystem |
| Lock file stale | `dotf doctor --unlock` |
| Backup corruption | `dotf doctor --verify-backups` → list corrupted backups |
| Accidental uninstall | `dotf restore --all` |
| Broken symlinks | `dotf doctor --fix` → reinstall broken symlinks |
| Repository deleted | `dotf restore --all` (backups preserved independently) |

### Emergency recovery

```
dotf doctor --emergency   # Full system recovery
```

This command:
1. Detects and releases stale locks
2. Rolls back incomplete transactions
3. Rebuilds state from filesystem scan
4. Verifies all backup checksums
5. Reports any inconsistencies
6. Offers to restore any corrupted files

---

## Summary of Changes from v0.1

| Area | v0.1 | Hardened |
|------|------|----------|
| File operations | Direct | Transactional with WAL journal |
| Path safety | None | Full validation engine |
| Symlink security | Trusting | Untrusted, validated |
| Locking | None | Cross-process PID-based |
| State | None | JSON state with self-healing |
| Backups | Single .bak | Versioned, checksummed |
| Restore | None | Full restore workflow |
| Hooks | None | Sandboxed, opt-in |
| Secrets | None | age/GPG with memfd |
| Packages | None | Unified abstraction |
| Merges | Winner-only | Format-aware merge |
| Explain | Basic explain | explain, doctor, inspect |
| Malicious defense | None | Multi-layer defense |
| Scalability | Untested | Lazy, parallel, cached |
| Recovery | Manual | Automated doctor --fix |
