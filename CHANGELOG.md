# Changelog

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
