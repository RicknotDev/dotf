# DOTF — Zero-Configuration Linux Dotfiles Runtime

DOTF detects your Linux environment and installs the matching configuration files from a layered repository. Profiles in an optional `dotf.yaml` let you override detection when you need to.

```bash
git clone https://github.com/RicknotDev/dotf.git
cd dotf
dotf install
```

## Installation

### From binary (recommended)

Download the latest release from [GitHub Releases](https://github.com/RicknotDev/dotf/releases).

```bash
chmod +x dotf
sudo mv dotf /usr/local/bin/
```

### From source

```bash
git clone https://github.com/RicknotDev/dotf.git
cd dotf
make build
sudo make install
```

## Quick Start

```bash
# Clone your dotfiles repo
git clone https://github.com/you/dotfiles
cd dotfiles

# See what DOTF detects
dotf explain

# Preview what will be installed
dotf install --dry-run

# Install
dotf install
```

## Usage

| Command | Description |
|---------|-------------|
| `dotf install` | Detect environment, resolve layers, install dotfiles |
| `dotf install --dry-run` | Preview without installing |
| `dotf install --copy` | Copy files instead of symlinks (containers) |
| `dotf install --allow-hooks` | Enable hook execution (disabled by default; also `DOTF_ALLOW_HOOKS=1`) |
| `dotf install --profile <name>` | Use a profile from `dotf.yaml` |
| `dotf install --no-packages` / `--no-secrets` | Skip package installation / secret deployment |
| `dotf install <git-url>` | Clone (or update) a repository into the state directory and install it |
| `dotf apply` | Deploy files only (no hooks, no packages); `--no-interactive` aborts on conflicts |
| `dotf status` | Show the state of every installed file (exit 1 on problems) |
| `dotf unapply` | Remove the files dotf installed (modified files are kept) |
| `dotf install --reorganize` | Auto-convert flat dotfiles to layers/ structure |
| `dotf explain` | Show detected environment and layer decisions |
| `dotf doctor` | Run diagnostics |
| `dotf doctor --fix` | Roll back interrupted runs, recreate missing links |
| `dotf doctor --unlock` | Force release stale lock |
| `dotf doctor --verify-backups` | Verify backup integrity |
| `dotf doctor --list-hooks` | List all available hooks |
| `dotf inspect` | Deep inspection of files, layers, and state |
| `dotf inspect file <path>` | Show which layer provides a file |
| `dotf inspect layer <name>` | Show all files in a layer |
| `dotf inspect state` | Show full state information |
| `dotf inspect overrides` | Show all file overrides |
| `dotf restore --preview` | Show available restores |
| `dotf restore --all` | Restore all backed-up files (newest version) |
| `dotf restore <path>` | Restore one file |

## Repository Structure

DOTF expects your dotfiles organized in a `layers/` directory with this structure:

```
layers/
├── base/                         # Always included (lowest priority)
│   ├── .config/
│   ├── .local/
│   └── .zshrc
├── distro/                       # Distribution-specific overrides
│   └── arch/
├── wm/                           # Window manager configs
│   ├── hyprland/
│   └── awesome/
├── shell/                        # Shell configs
│   ├── bash/
│   ├── zsh/
│   └── fish/
├── terminal/                     # Terminal emulator configs
│   ├── alacritty/
│   ├── ghostty/
│   └── kitty/
├── gpu/                          # GPU-specific configs
│   ├── amd/
│   └── nvidia/
├── device/                       # Device type configs
│   ├── laptop/
│   └── desktop/
└── host/                         # Hostname-specific configs (highest priority)
    └── my-machine/
```

Files inside each layer directory mirror paths relative to `$HOME`. For example:
- `layers/base/.config/alacritty/alacritty.toml` → `~/.config/alacritty/alacritty.toml`
- `layers/shell/zsh/.zshrc` → `~/.zshrc`

Layer priority (highest wins when files overlap): `host` > `device` > `gpu` > `terminal` > `shell` > `wm` > `desktop` > `distro` > `base`

Inside each layer, `hooks/`, `packages/` and `secrets/` hold DOTF metadata and are never linked into `$HOME`:

- `hooks/pre-install.sh`, `hooks/post-install.sh`: run only with `--allow-hooks`
- `packages/pacman.txt`, `aur.txt`, `apt.txt`, `dnf.txt`, `zypper.txt`, `nix.txt`: only the list for the detected package manager is used
- `secrets/<path>.age` (or `.gpg`): decrypted to `~/<path>` with mode 0600

## dotf.yaml (optional)

```yaml
profile: laptop          # default profile
detect:                  # force detected values ("none" disables one)
  shell: zsh
include: [wm/sway]       # extra layers
exclude: [gpu/nvidia]    # layers to drop
profiles:
  laptop:
    detect: {device: laptop}
    include: [host/work]
```

DOTF never writes this file, and it cannot enable hooks.

## Features

- **Environment detection**: distro, session, WM, DE, shell, terminal, GPU, hostname, device type
- **Layer-based configuration**: the highest-priority layer wins for each file
- **Transactions**: every change is journaled with what was there before; a failure or Ctrl-C rolls back the whole run, and an interrupted run is rolled back the next time dotf starts
- **Safe dry runs**: `--dry-run` writes nothing and runs nothing
- **Versioned backups**: in `~/.local/state/dotf/backups` (`$XDG_STATE_HOME/dotf/backups`), SHA-256 checksum stored at creation, original permissions kept, 5 versions per file (`DOTF_BACKUP_KEEP`)
- **Locking**: `flock(2)`, released by the kernel when dotf exits
- **Path safety**: blocks traversal, sensitive system paths, symlink escapes and `~/.ssh/authorized_keys`
- **Packages**: per-manager package lists, AUR helpers preferred, `sudo` when needed
- **Secrets**: age/GPG files decrypted straight to their destination with mode 0600
- **Opt-in hooks**: timeouts that kill the whole process group, logged to `hooks.log`
- **Scriptable**: `--json`, `--quiet`, exit codes 0/1/2/3/64

Not implemented: merging YAML/JSON/TOML files, an audit log, memory-only decryption, hook resource limits. See [ARCHITECTURE.md](ARCHITECTURE.md).

## Development

```bash
make build    # Build the binary
make test     # Run tests (with a temporary HOME)
make lint     # gofmt check + go vet
make e2e      # End-to-end test of the binary in a temporary HOME
```

## License

MIT
