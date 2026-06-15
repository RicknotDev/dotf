# DOTF — Zero-Configuration Linux Setup Runtime

DOTF automatically detects your Linux environment and installs the correct configuration files from a layered repository. No manual profile selection, no symlink management, no documentation required.

```bash
git clone <repository>
cd <repository>
dotf install
```

## Features

- **Automatic environment detection** — distro, WM, session, shell, terminal, GPU, hostname, device type
- **Layer-based configuration** — resolves `base`, `distro/`, `wm/`, `shell/`, `terminal/`, `gpu/`, `device/`, `host/` layers automatically
- **Transaction safety** — write-ahead logging with automatic rollback on failure
- **Crash recovery** — incomplete transactions detected and rolled back on next run
- **Versioned backups** — SHA-256 checksummed, 5 versions kept by default
- **Cross-process locking** — prevents concurrent installs
- **Package management** — auto-installs packages from `layers/*/packages/*.txt`
- **Secret deployment** — age/GPG encrypted secrets, memory-only decryption
- **Sandboxed hooks** — opt-in pre/post install scripts with timeouts and logging
- **Malicious repository defense** — path traversal protection, symlink chain validation, sensitive path blocking

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

### 1. Clone a dotfiles repository

```bash
git clone https://github.com/example/dotfiles
cd dotfiles
```

### 2. See what DOTF detects

```bash
dotf explain
```

Example output:
```
Detected:
  distro: arch
  session: wayland
  wm: hyprland
  shell: fish
  terminal: wezterm
  gpu: amd
  hostname: thinkpad
  device_type: laptop

Activated Layers:
  base
  distro/arch
  wm/hyprland
  shell/fish
  terminal/wezterm
  gpu/amd
  host/thinkpad
```

### 3. Preview what will be installed

```bash
dotf install --dry-run
```

### 4. Install

```bash
dotf install
```

## Usage

| Command | Description |
|---------|-------------|
| `dotf install` | Detect environment, resolve layers, install dotfiles |
| `dotf install --dry-run` | Preview without installing |
| `dotf install --copy` | Copy files instead of symlinks (containers) |
| `dotf install --allow-hooks` | Enable hook execution (disabled by default) |
| `dotf explain` | Show detected environment and layer decisions |
| `dotf doctor` | Run diagnostics |
| `dotf doctor --fix` | Repair detected issues |
| `dotf doctor --emergency` | Full system recovery |
| `dotf inspect` | Deep inspection of files, layers, state |
| `dotf inspect file <path>` | Show which layer provides a file |
| `dotf inspect layer <name>` | Show all files in a layer |
| `dotf inspect state` | Show full state information |
| `dotf inspect overrides` | Show all file overrides |
| `dotf restore` | Preview available restores |
| `dotf restore --all` | Restore all backed-up files |

## Creating Your Dotfiles Repository

### Layer structure

```
layers/
├── base/                         # Always included
│   └── .config/
├── distro/
│   ├── arch/
│   ├── fedora/
│   └── ubuntu/
├── wm/
│   ├── hyprland/
│   └── qtile/
├── shell/
│   ├── fish/
│   └── zsh/
├── terminal/
│   ├── kitty/
│   └── wezterm/
├── gpu/
│   ├── amd/
│   └── nvidia/
├── device/
│   ├── laptop/
│   └── desktop/
└── host/
    └── thinkpad/
```

### Layer priority (highest first)

1. `host/<hostname>` — machine-specific config
2. `device/<type>` — laptop/desktop/server/vm/container
3. `gpu/<vendor>` — amd/intel/nvidia
4. `terminal/<name>` — kitty/wezterm/foot/ghostty
5. `shell/<name>` — fish/zsh/bash/nushell
6. `wm/<name>` — hyprland/qtile/river/dwm/awesome
7. `desktop/<name>` — gnome/kde/xfce
8. `distro/<name>` — arch/fedora/ubuntu/nixos
9. `base` — always included

Higher-priority files override lower-priority files.

### Package lists

```
layers/base/packages/pacman.txt
layers/base/packages/apt.txt
```

One package per line. Lines starting with `#` are comments.

### Hooks

```
layers/base/hooks/pre-install.sh
layers/base/hooks/post-install.sh
```

Hooks are disabled by default. Enable with `dotf install --allow-hooks`.

### Secrets

```
layers/base/secrets/github_token.age
layers/base/secrets/ssh_key.gpg
```

Decryption keys: `~/.config/dotf/keys/` (age) or GPG keyring.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full system design.

## Development

```bash
make build    # Build the binary
make test     # Run tests
make vet      # Run go vet
make lint     # Run linter
make clean    # Clean build artifacts
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

See [SECURITY.md](SECURITY.md) for the security policy and vulnerability reporting.

## License

MIT
