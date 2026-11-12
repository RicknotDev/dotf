# Changelog

## v0.7.0 (unreleased)

This release fixes the problems found in an audit of v0.6.0. Several of them
could lose user data.

### Fixed: data safety

- **Rollback works.** Transactions journal what existed before each change and
  roll back every operation (not only unfinished ones), restoring previous files
  and symlinks. Files are replaced atomically (temp file + rename). Interrupted
  runs are rolled back after taking the lock on the next run.
- **No deletion without backup.** If a backup fails, the file is left alone.
  Foreign symlinks are backed up too.
- **Backups**: `.zshrc` no longer matches `.zshrc.local`; paths containing `__`
  are unambiguous; the checksum is stored at creation and verified against that;
  original permissions are kept; directories are 0700.
- **`--dry-run` writes nothing**: no state, no lock, no `dotf.yaml`, no hooks.
- **`hooks/`, `packages/`, `secrets/` are no longer linked into `$HOME`.**
- **Running as root works** (`/root` is no longer rejected);
  `~/.ssh/authorized_keys` is blocked instead. Files named `..something` work.
- **Locking** uses `flock(2)`: no race on creation and no PID guessing.
  `doctor --unlock` refuses to remove a lock held by a running process.
- `unapply` only removes what dotf installed and the user did not modify.
- `restore` uses the newest backup, fails with a non-zero exit code, runs in a
  transaction and updates the state.

### New

- Secrets are deployed: `secrets/<path>.age|.gpg` to `~/<path>` (0600). `--no-secrets`.
- `dotf.yaml` profiles: `detect`, `include`, `exclude`, `profiles`, `--profile`.
- `doctor --fix` / `--emergency` roll back interrupted transactions and recreate
  missing links and copies.
- `install` and `apply` share one engine; files removed from the repository are
  cleaned up; unchanged files are left alone; git URLs are updated on later runs.
- Packages: per-manager lists, paru/yay preferred, `sudo` when not root,
  `--no-packages`, option-like names rejected.
- `--json` on every command; errors are always printed (JSON on stderr with `--json`).
- `DOTF_BACKUP_KEEP`.

### Changed

- `dotf.yaml` is no longer generated. `layers:` and `hooks.enabled` are ignored;
  hooks run only with `--allow-hooks` or `DOTF_ALLOW_HOOKS=1`.
- Exit codes come from the error type; usage errors exit 64. `status` exits 1 on
  problems also with `--json`.
- `--quiet` no longer hides errors. Global flags work anywhere and accept
  `--filter=value`; `key:value` filters use the key.
- Colors only when stdout is a terminal. `version` and `--help` print to stdout.
- `status`, `explain`, `doctor` and `inspect` work outside the repository using
  the recorded one; output is sorted. `inspect overrides` shows the right winner.
- Hooks run from the repository root, are killed with their process group on
  timeout, and a failing pre-install hook stops the install.
- Detection: `ubuntu:GNOME` is gnome; Google and AMD-chipset PCI IDs are not
  GPUs; hybrid GPUs prefer the discrete card; the PCI fallback uses the device
  class; Surface devices are not VMs; the shell fallback uses the parent
  process; tmux and vscode are not terminals; layer names are sanitized.
- `--reorganize`: picom, rofi and starship go to `base`; README is not copied;
  no empty layer directories; file modes are kept; larger app map.
- Module path is `github.com/RicknotDev/dotf`.

### Project

- CI: real gofmt check, version embedded in builds, release assets named
  `dotf-linux-amd64` / `dotf-linux-arm64`, end-to-end test in a temporary HOME.
- `dist/` is no longer tracked.
- Documentation describes only what exists.

## v0.6.0 (2026-06-16)

### New Features

- **New user experience**: Running `dotf` without arguments now shows a welcome screen with a quick-start guide in 3 steps, making it easy for first-time users to get started without reading documentation.
- **`dotf status` command**: Shows the installation status of all dotfiles at a glance. Supports `--short` (show only issues), `--json` (machine-readable), `--filter` (filter by status or path), and returns non-zero exit code when files have issues — perfect for health-check scripts.
- **`dotf apply` command**: Applies resolved configuration layers to your home directory. Supports `--dry-run` for preview, `--diff` to show what would change, `--profile` to override auto-detection, and `--no-interactive` for CI/CD environments. Creates automatic backups of existing files before overwriting.
- **`dotf unapply` command**: Cleanly reverts applied changes by removing all managed symlinks. Supports `--dry-run` for preview and `--json` for scripting.
- **`dotf.yaml` configuration**: DOTF now reads `dotf.yaml` (or `dotf.yml`) from the repository root for profile and layer configuration. Automatically generates `dotf.yaml` during `dotf install` if none exists.
- **`--json` global flag**: All commands support machine-readable JSON output for scripting and automation. Errors are reported as structured JSON on stderr while data goes to stdout.
- **`--quiet` global flag**: Suppresses all non-error output, ideal for scripts where only exit codes matter.
- **`--no-color` global flag and `NO_COLOR` support**: Disables colored output. Respects the `NO_COLOR` environment variable per the no-color.org standard.
- **`--filter` global flag**: Filters status output by expression (e.g., `--filter broken`, `--filter "status:ok"`).
- **Proper exit codes**: Exit code 0 for success, 1 for general errors, 2 for unresolved conflicts, 3 for nothing-to-do. Makes DOTF fully scriptable.

### Workflow Improvements

- **New user workflow**: `dotf` → welcome screen → `dotf install` → automatic detection → done. No documentation needed.
- **Existing user workflow**: `dotf apply --profile X` applies cleanly, `dotf status --short` shows only issues, `dotf unapply` reverts cleanly, `dotf apply` after `unapply` gives the same state.
- **Power user workflow**: All commands are scriptable with `--json`, `--quiet`, `--no-color`, and proper exit codes. `dotf status` with non-zero exit code for health monitoring.
- **CI/CD workflow**: `dotf apply --no-interactive` aborts on conflicts with exit code 2 instead of hanging waiting for input.

### Bug Fixes

- **Error swallowing in backup pruning**: The `prune()` function in `backup.go` now logs warnings instead of silently ignoring `os.Remove` failures.
- **Error swallowing in transaction journal**: The `flushJournal()` function now cleans up temp files on rename failure instead of leaving them behind.
- **Error swallowing in hook logging**: Hook log writes now report errors instead of silently ignoring them.
- **Error swallowing in `inspect` command**: `os.Getwd()` errors are now properly checked and returned instead of being discarded.
- **Error swallowing in `doctor` command**: `os.Getwd()` errors are now handled with a clear warning instead of being discarded.
- **Error swallowing in secret decryption**: `os.UserHomeDir()` errors are now properly returned instead of silently ignored.
- **Lock duration bug**: `lock.Acquire()` was being called with an integer literal (nanoseconds) instead of `time.Duration` (seconds) in some commands, causing near-instant timeouts. Fixed to use `30 * time.Second`.
- **Conflict handling in `apply`**: The `apply` command now creates versioned backups before overwriting existing files, matching the behavior of `install`.
- **`dotf` without arguments no longer exits with error**: Now shows the welcome screen and exits with 0.

### Deprecations

- Running `dotf` without arguments now shows a welcome screen instead of a minimal usage message. The full usage is still available via `dotf --help`.

### Notes

- This release focuses on making DOTF accessible to new users and scriptable for power users while hardening the core against common failure modes.
- Integration tests and additional edge-case coverage are planned for the next release cycle.
