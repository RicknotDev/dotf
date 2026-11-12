# DOTF architecture

This document describes what the code does today (v0.7.0). Package paths are
relative to `internal/`.

## Overview

```
dotf install
  │
  ├─ resolve source        cli/install.go   (path, or git URL cloned to <state>/repos)
  ├─ detect + dotf.yaml    detect/, cli/profile.go, layer/
  ├─ lock + recovery       lock/, transaction.Recover
  ├─ pre-install hooks     hook/            (only with --allow-hooks / DOTF_ALLOW_HOOKS=1)
  ├─ packages              pkg/             (unless --no-packages)
  ├─ plan + execute        deploy/          (one transaction for all file changes)
  ├─ save state            state/
  └─ post-install hooks    hook/
```

`dotf apply` runs the same deploy engine without hooks and packages.
`--dry-run` stops after building the plan: it takes no lock, writes no state,
creates no directories, runs no hooks and installs no packages.

The state directory is `$XDG_STATE_HOME/dotf` (default `~/.local/state/dotf`),
mode 0700. It contains `state.json`, `backups/`, `transactions/`, `repos/`,
`hooks.log` and `dotf.lock`.

## 1. Layers and detection

`detect/` reads `/etc/os-release`, environment variables (`XDG_CURRENT_DESKTOP`,
`SHELL`, `TERMINAL`, `TERM_PROGRAM`...), `/proc` and `/sys` to fill a
`profile.Profile` (distro, session, WM, desktop, shell, terminal, GPU, hostname,
device type).

- `XDG_CURRENT_DESKTOP` is a colon-separated list; the first component with a
  known name wins (`ubuntu:GNOME` → `gnome`).
- GPU: vendor IDs of DRM cards, falling back to PCI devices of class `0x03xxxx`.
  With several GPUs the discrete one wins (nvidia > amd > intel).
- VMs are detected from DMI strings; a Microsoft vendor only counts as a VM
  with the product "Virtual Machine" (Surface devices are not VMs).
- Layer names are sanitized (`profile.SafeName`): lowercase, one path component,
  no `..`. Anything else is dropped.

Layers, lowest to highest priority:
`base`, `distro/`, `desktop/`, `wm/`, `shell/`, `terminal/`, `gpu/`, `device/`,
`host/`. Only directories that exist under `layers/` are active.

### dotf.yaml

Optional, at the repository root (`cli/profile.go`):

```yaml
profile: laptop            # default profile (overridden by --profile)
detect:                    # force detected values; "none" disables a layer type
  shell: zsh
include: [wm/sway]         # extra layers
exclude: [gpu/nvidia]      # layers to drop
profiles:
  laptop:
    detect: {device: laptop}
    include: [host/work]
```

The selected profile's `detect` values override the top-level ones; its
`include`/`exclude` lists are appended. An unknown `--profile` is an error.
The v0.6 keys `layers:` and `hooks.enabled` are accepted and ignored (with a
warning). DOTF never writes `dotf.yaml`.

## 2. Deploy engine (`deploy/`)

1. **Collect.** Walk every active layer. Each file maps to `~/<path relative to
   the layer>`; a higher-priority layer replaces a lower one. `hooks/`,
   `packages/`, `secrets/` and `.git` at the root of a layer (and any `.git`
   directory) are not deployed. Files under `secrets/<path>.age|.gpg|.asc` become
   secret entries for `~/<path>`.
2. **Plan.** For each entry, validate the source (`safety.ValidateLayerFile`) and
   target (`safety.ValidateTargetPath`), then compare with `$HOME` and the state:

   | Action      | When |
   |-------------|------|
   | `create`    | nothing at the target |
   | `unchanged` | already the right symlink / identical copy / same secret |
   | `update`    | the target is still what dotf installed earlier |
   | `replace`   | something else is there; it is backed up first |
   | `skip`      | unsafe path, target is a directory, parent is not a directory |
   | `remove`    | installed earlier but no longer in any active layer |

   Planning only reads.
3. **Execute.** All changes go through one transaction. If a backup cannot be
   made, that file is left untouched and reported as an error (exit code 1).
   Any failure while changing `$HOME`, or SIGINT/SIGTERM, rolls back the whole
   transaction. Files removed from the repository are only deleted when they are
   still exactly what dotf installed.

Modes: symlinks (default) or copies (`--copy`, keeps the source mode).
Secrets are always regular files with mode 0600.

## 3. Transactions (`transaction/`)

Each transaction has a directory `<state>/transactions/<id>/` with an
append-only `journal` (one JSON object per line) and a stash.

- Before changing a path, the journal records what was there: nothing, a symlink
  (with its target) or a regular file (copied into the stash with its mode).
  The entry is fsynced before the change is made.
- New content is created under a temporary name in the target directory and
  `rename`d over the target. Directories created on the way are journaled too.
- `Commit` appends `{"op":"commit"}` and deletes the directory.
- `Rollback` undoes every entry, newest first, restoring the previous content.
  If something cannot be restored the journal is kept.
- A journal without a commit line is an interrupted transaction.
  `transaction.Recover` rolls it back; install, apply, unapply and restore call it
  right after taking the lock, and `doctor --fix` calls it explicitly. A torn
  last line (crash while writing it) is ignored, since that operation never
  started. Journals from v0.6 (`*.journal`) are handled in a best-effort way.

## 4. Locking (`lock/`)

`dotf.lock` in the state directory, locked with `flock(2)` (`LOCK_EX|LOCK_NB`,
retried until a 30 s timeout). The kernel releases the lock when the process
exits, so there is no stale-lock heuristic. The PID and hostname in the file are
informational. `Release` unlinks the file while still holding the lock, and
`Acquire` checks that the file it locked is still the one at the path.
`ForceRelease` (`doctor --unlock`) only removes a file nobody holds.

## 5. Path safety (`safety/`)

- Repository files must stay inside the repository, including through symlink
  chains (max 40, loops rejected).
- Targets must stay inside `$HOME`; absolute paths and `..` are rejected
  (`..foo` is a valid name). Maximum path length 4096.
- `SensitivePaths` (`/etc/passwd`, `/boot`, `/usr/bin`, ...) are rejected,
  except entries that are `$HOME` or one of its parents: for root, `$HOME` is
  `/root`, and its dotfiles are installable.
- `~/.ssh/authorized_keys` and `authorized_keys2` are never managed.

## 6. State (`state/`)

`state.json` (mode 0600) records the repository, the installed layers, and for
each installed file its layer, type (`symlink`, `copy`, `secret`), source and,
for copies and secrets, the checksum of what was placed (secrets also record the
checksum of the encrypted file). Nothing is written until `Save`. A file that
cannot be parsed is kept as `state.json.corrupt-<time>` and a warning is shown.
`status`, `explain`, `doctor` and `inspect` use the recorded repository when the
current directory is not one.

## 7. Backups (`backup/`)

`<state>/backups/<url.PathEscape(rel)>@<unix-ns>.bak`, plus `<name>.sha256` with
the checksum computed when the backup was made. The backup keeps the original
mode; symlinks are backed up as symlinks. Lookups match the exact original path.
The newest `DOTF_BACKUP_KEEP` versions (default 5) are kept per file. v0.6 names
(`path__to__file.<ns>.bak`) are still listed; they have no stored checksum.

## 8. Restore and unapply

- `restore <path>...` / `restore --all` use the newest backup of each path,
  verify its checksum and write it through a transaction (mode preserved). If
  the current file is not something dotf installed, it is backed up first. The
  restored path is removed from the state. Any failure rolls everything back and
  exits non-zero. Without arguments, `restore` previews.
- `unapply` removes symlinks that still point to their recorded source and copies
  or secrets whose checksum still matches; anything modified is left in place
  with a warning. Removal is transactional.

## 9. Hooks (`hook/`)

`layers/<layer>/hooks/{pre,post}-install[.sh]` (also `error`, `pre/post-update`,
`pre/post-restore`, which are discovered but only `pre-install`, `post-install`
and `error` are run by install). Hooks run only with `--allow-hooks` or
`DOTF_ALLOW_HOOKS=1`; the repository cannot enable them. They run with
`/bin/sh`, in the repository root, in their own process group; on timeout
(60 s) the whole group is killed. Environment: `DOTF_HOOK_TYPE`,
`DOTF_HOOK_LAYER`, `DOTF_DIR`, `DOTF_HOME`, `DOTF_PROFILE`. Output is appended to
`<state>/hooks.log`. A failing pre-install hook stops the install before any
change. Hooks are not sandboxed beyond this.

## 10. Packages (`pkg/`)

The first available manager is used, in this order: paru, yay, pacman, apt
(apt-get), dnf, zypper, nix. Only the lists meant for it are read:
`packages/pacman.txt` (pacman, paru, yay), `packages/aur.txt` (paru, yay),
`apt.txt`, `dnf.txt`, `zypper.txt`, `nix.txt` (`.list` and no extension also
work). One or more names per line, `#` starts a comment. Names starting with `-`
are rejected. pacman, apt, dnf and zypper run through `sudo` when not root.

## 11. Secrets (`secret/`)

`age` (identities in `~/.config/dotf/keys/*.age` and `~/.config/age/*.txt`) or
`gpg --batch --decrypt`. The plaintext is streamed into a 0600 temporary file in
the target directory and renamed into place; it never goes to `/tmp`. The
existing file is backed up first like any other file. `--no-secrets` skips them.

## 12. Output and exit codes (`cli/`)

Results go to stdout (JSON with `--json`), progress to stderr (hidden by
`--quiet` and `--json`), errors to stderr always (as JSON objects with
`--json`). Colors are used only when stdout is a terminal and `NO_COLOR` is
unset. Global flags may appear anywhere.

| Code | Meaning |
|------|---------|
| 0    | success |
| 1    | error (including `status` with problems, also with `--json`) |
| 2    | conflict with `apply --no-interactive` |
| 3    | nothing to do |
| 64   | usage error |

## Not implemented

These were described in earlier documents but do not exist: merging of
YAML/JSON/TOML files, an audit log, memory-only (memfd) decryption, resource
limits for hooks, and rebuilding the state from a filesystem scan.
