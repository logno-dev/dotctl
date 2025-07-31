# dotctl

A system-aware dotfiles manager built in Go that uses GNU Stow for symlink management.

## Features

- **System-aware deployment**: Configure packages for specific operating systems (Linux, macOS, Arch, Ubuntu, etc.)
- **GNU Stow integration**: Leverages the proven GNU Stow tool for reliable symlink management
- **GitHub integration**: Sync your dotfiles to/from GitHub repositories using GitHub CLI
- **Zero dependencies**: Built with Go standard library only
- **Dry-run support**: Preview changes before applying them
- **Automatic system detection**: Detects your OS and Linux distribution automatically
- **JSON configuration**: Simple, readable configuration format

## Installation

### Prerequisites

You need GNU Stow installed on your system:

```bash
# macOS
brew install stow

# Arch Linux
sudo pacman -S stow

# Ubuntu/Debian
sudo apt install stow

# Fedora
sudo dnf install stow
```

For GitHub integration, you also need GitHub CLI:

```bash
# macOS
brew install gh

# Arch Linux
sudo pacman -S github-cli

# Ubuntu/Debian
sudo apt install gh

# Fedora
sudo dnf install gh
```

### Install dotctl

#### Option 1: Build from source

```bash
git clone <repository-url>
cd dotctl
make build
make install
```

#### Option 2: Install to user directory (no sudo required)

```bash
make install-user
```

Make sure `~/bin` is in your PATH.

## Quick Start

1. **Initialize your dotfiles directory** (default: `~/.dotfiles`):
   ```bash
   mkdir ~/.dotfiles
   ```

2. **Create package directories** for your configurations:
   ```bash
   mkdir ~/.dotfiles/vim
   mkdir ~/.dotfiles/tmux
   mkdir ~/.dotfiles/shell
   ```

3. **Move your dotfiles** into the appropriate packages:
   ```bash
   mv ~/.vimrc ~/.dotfiles/vim/
   mv ~/.tmux.conf ~/.dotfiles/tmux/
   mv ~/.bashrc ~/.dotfiles/shell/
   ```

4. **Add packages to configuration**:
   ```bash
   dotctl add vim all
   dotctl add tmux linux macos
   dotctl add shell all
   ```

5. **Deploy your dotfiles**:
   ```bash
   dotctl deploy
   ```

## Usage

### Commands

- `dotctl deploy [packages...]` - Deploy packages (default: all for current system)
- `dotctl undeploy [packages...]` - Undeploy packages
- `dotctl status` - Show current status and package information
- `dotctl add <package> [systems...]` - Add package to configuration
- `dotctl remove <package>` - Remove package from configuration
- `dotctl github-repo <owner/repo> [branch]` - Set GitHub repository for sync
- `dotctl sync` - Sync dotfiles to GitHub repository
- `dotctl pull` - Pull dotfiles from GitHub repository

### Options

- `--dotfiles-dir <path>` - Path to dotfiles directory (default: `~/.dotfiles`)
- `--dry-run` - Show what would be done without executing
- `--help` - Show help message

### Examples

```bash
# Deploy all packages for current system
dotctl deploy

# Deploy specific packages
dotctl deploy vim tmux

# Preview what would be deployed
dotctl --dry-run deploy

# Check status
dotctl status

# Add a package for all systems
dotctl add shell all

# Add a package for specific systems
dotctl add vim linux macos

# Remove a package from configuration
dotctl remove old-package

# Use custom dotfiles directory
dotctl --dotfiles-dir ~/my-dotfiles deploy

# GitHub integration
dotctl github-repo username/my-dotfiles
dotctl sync                          # Push to GitHub
dotctl pull                          # Pull from GitHub
```

## Configuration

dotctl uses a `dotctl.json` file in your dotfiles directory. This file is automatically created with sensible defaults.

### Example Configuration

```json
{
  "packages": {
    "vim": "all",
    "tmux": {
      "systems": ["linux", "macos"]
    },
    "shell": "all",
    "i3": "linux"
  },
  "global_excludes": [
    ".git",
    ".DS_Store",
    "*.pyc",
    "__pycache__"
  ],
  "stow_options": [
    "--verbose",
    "--target=/home/username"
  ],
  "github": {
    "repository": "username/my-dotfiles",
    "branch": "main"
  }
}
```

### Supported Systems

- `all` - Deploy on all systems
- `linux` - Any Linux distribution
- `macos` - macOS
- `arch` - Arch Linux
- `ubuntu` - Ubuntu
- `debian` - Debian
- `fedora` - Fedora

## GitHub Integration

dotctl can sync your dotfiles to and from GitHub repositories using the GitHub CLI.

### Setup

1. **Install GitHub CLI** (see Prerequisites above)

2. **Authenticate with GitHub**:
   ```bash
   gh auth login
   ```

3. **Configure your repository**:
   ```bash
   dotctl github-repo username/my-dotfiles
   # Or specify a branch
   dotctl github-repo username/my-dotfiles develop
   ```

### Usage

- **Sync to GitHub** (commit and push changes):
  ```bash
  dotctl sync
  ```

- **Pull from GitHub** (pull latest changes):
  ```bash
  dotctl pull
  ```

- **Preview sync operations**:
  ```bash
  dotctl --dry-run sync
  dotctl --dry-run pull
  ```

### How it Works

- If your dotfiles directory isn't a git repository, `dotctl sync` will initialize it and add the GitHub remote
- `dotctl sync` adds all files, commits with a timestamp, and pushes to the configured repository
- `dotctl pull` pulls the latest changes from the configured branch
- If the dotfiles directory doesn't exist when pulling, it will clone the repository

## Directory Structure

Your dotfiles directory should be organized like this:

```
~/.dotfiles/
├── dotctl.json          # Configuration file
├── vim/                 # Vim package
│   └── .vimrc
├── tmux/                # Tmux package
│   └── .tmux.conf
├── shell/               # Shell package
│   ├── .bashrc
│   └── .zshrc
└── i3/                  # i3 window manager (Linux only)
    └── .config/
        └── i3/
            └── config
```

When deployed, dotctl will create symlinks in your home directory pointing to the files in your dotfiles packages.

## Development

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Format code
make fmt

# Run in development
make run ARGS="status"
```

### Creating Releases

```bash
make release VERSION=v1.0.0
```

This creates release archives in `build/releases/`.

## How It Works

dotctl is a wrapper around GNU Stow that adds system-awareness and configuration management:

1. **System Detection**: Automatically detects your operating system and Linux distribution
2. **Package Filtering**: Only deploys packages configured for your current system
3. **Stow Integration**: Uses GNU Stow to create and manage symlinks
4. **Configuration Management**: Maintains a JSON configuration file for package-to-system mappings

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]