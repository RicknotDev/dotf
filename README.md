# DOTF — Zero-Configuration Linux Setup Runtime

DOTF automatically detects your Linux environment and installs the correct configuration files from a layered repository. No manual profile selection, no symlink management, no documentation required.

```bash
git clone <repository>
cd <repository>
dotf install
```

## Installation

### From binary (recommended)

Download the latest release from [GitHub Releases](https://github.com/RicknotDev/dotf/releases/tag/v0.3.0).

```bash
chmod +x dotf
sudo mv dotf /usr/local/bin/
```

### From source

```bash
git clone https://github.com/codebuff/dotf.git
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
| `dotf install --allow-hooks` | Enable hook execution (disabled by default) |
| `dotf install --reorganize` | Auto-convert flat dotfiles to layers/ structure |
| `dotf explain` | Show detected environment and layer decisions |
| `dotf doctor` | Run diagnostics |
| `dotf doctor --fix` | Repair detected issues |
| `dotf doctor --unlock` | Force release stale lock |
| `dotf doctor --verify-backups` | Verify backup integrity |
| `dotf doctor --list-hooks` | List all available hooks |
| `dotf inspect` | Deep inspection of files, layers, and state |
| `dotf inspect file <path>` | Show which layer provides a file |
| `dotf inspect layer <name>` | Show all files in a layer |
| `dotf inspect state` | Show full state information |
| `dotf inspect overrides` | Show all file overrides |
| `dotf restore --preview` | Show available restores |
| `dotf restore --all` | Restore all backed-up files |

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

## Features

- **Automatic environment detection** — distro, session, WM, DE, shell, terminal, GPU, hostname, device type
- **Layer-based configuration** — resolves `base`, `distro/`, `wm/`, `shell/`, `terminal/`, `gpu/`, `device/`, `host/` layers automatically
- **Transaction safety** — write-ahead logging with automatic rollback on failure
- **Graceful abort** — SIGINT/SIGTERM handled without corrupting state
- **Crash recovery** — incomplete transactions detected and rolled back on next run
- **Versioned backups** — SHA-256 checksummed, 5 versions kept by default, auto-pruning
- **Cross-process locking** — prevents concurrent installs, stale lock detection
- **Path safety** — blocks traversal escapes, sensitive paths, symlink loops
- **State management** — JSON state with self-healing from filesystem scan
- **Package management** — auto-installs packages from `layers/*/packages/*.txt`
- **Secret deployment** — age/GPG encrypted secrets, memory-only decryption
- **Sandboxed hooks** — opt-in pre/post install scripts with timeouts and logging
- **Restore** — preview and restore from any backed-up version

## Development

```bash
make build    # Build the binary
make test     # Run tests
make vet      # Run go vet
```

## License

MIT
