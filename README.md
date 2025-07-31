# dotctl

A system-aware dotfiles manager built in Go with native symlink management.

## Features

- **System-aware deployment**: Configure packages for specific operating systems (Linux, macOS, Arch, Ubuntu, etc.)
- **Native symlink management**: No external dependencies - uses Go's built-in symlink functionality
- **GitHub integration**: Sync your dotfiles to/from GitHub repositories using GitHub CLI
- **Zero dependencies**: Built with Go standard library only - no need to install GNU Stow
- **Dry-run support**: Preview changes before applying them
- **Automatic system detection**: Detects your OS and Linux distribution automatically
- **JSON configuration**: Simple, readable configuration format
- **Smart package adoption**: Automatically adopt new config directories from `~/.config/`

## Installation

### Prerequisites

**No external dependencies required!** dotctl uses native Go symlinks.

For GitHub integration (optional), you need GitHub CLI:

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

### Method 1: Start Fresh

1. **Initialize your dotfiles directory** (default: `~/.dotfiles`):
   ```bash
   mkdir ~/.dotfiles
   ```

2. **Create package directories** for your configurations:
   ```bash
   mkdir ~/.dotfiles/nvim
   mkdir ~/.dotfiles/tmux
   mkdir ~/.dotfiles/shell
   ```

3. **Move your dotfiles** into the appropriate packages:
   ```bash
   mv ~/.config/nvim ~/.dotfiles/nvim/nvim
   mv ~/.config/tmux ~/.dotfiles/tmux/tmux
   mv ~/.zshrc ~/.dotfiles/shell/
   ```

4. **Initialize configuration**:
   ```bash
   dotctl init
   ```

5. **Deploy your dotfiles**:
   ```bash
   dotctl deploy
   ```

### Method 2: Adopt Existing Configs

1. **Initialize dotctl**:
   ```bash
   mkdir ~/.dotfiles
   dotctl init
   ```

2. **Adopt existing config directories**:
   ```bash
   # Adopt all new config directories
   dotctl adopt
   
   # Or adopt specific packages
   dotctl adopt nvim tmux
   ```

3. **Deploy to other systems**:
   ```bash
   dotctl deploy
   ```

## Usage

### Commands

- `dotctl init` - Initialize configuration by scanning package directories
- `dotctl deploy [packages...]` - Deploy packages (default: all for current system)
- `dotctl undeploy [packages...]` - Undeploy packages
- `dotctl status` - Show current status and package information
- `dotctl add <package> [systems...]` - Add package to configuration
- `dotctl remove <package>` - Remove package from configuration
- `dotctl adopt [package] [systems...]` - Adopt config directories from ~/.config
- `dotctl github-repo <owner/repo> [branch]` - Set GitHub repository for sync
- `dotctl sync` - Sync dotfiles to GitHub repository
- `dotctl pull` - Pull dotfiles from GitHub repository

### Options

- `--dotfiles-dir <path>` - Path to dotfiles directory (default: `~/.dotfiles`)
- `--dry-run` - Show what would be done without executing
- `--help` - Show help message

### Examples

```bash
# Initialize configuration from existing packages
dotctl init

# Deploy all packages for current system
dotctl deploy

# Deploy specific packages
dotctl deploy nvim tmux

# Preview what would be deployed
dotctl --dry-run deploy

# Check status
dotctl status

# Add a package for all systems
dotctl add shell all

# Add a package for specific systems
dotctl add nvim linux macos

# Remove a package from configuration
dotctl remove old-package

# Adopt all new config directories
dotctl adopt

# Adopt specific package for all systems
dotctl adopt new-app

# Adopt specific package for specific systems
dotctl adopt new-app arch linux

# Preview what would be adopted
dotctl --dry-run adopt

# Use custom dotfiles directory
dotctl --dotfiles-dir ~/my-dotfiles deploy

# GitHub integration
dotctl github-repo username/my-dotfiles
dotctl sync                          # Push to GitHub
dotctl pull                          # Pull from GitHub
```

## Package Types

dotctl automatically determines where packages should be deployed based on their names:

### Config Packages (→ `~/.config/`)
- **Any package name that doesn't start with `.` and isn't named `shell`**
- Examples: `nvim`, `tmux`, `bat`, `gh`, `kitty`
- Creates: `~/.config/PACKAGE_NAME/` → `../.dotfiles/PACKAGE_NAME/`

### Home Packages (→ `~/`)
- **Packages starting with `.`**: `.oh-my-zsh`, `.vim`, etc.
- **`shell` package**: Contains shell configs like `.zshrc`, `.bashrc`
- Creates: `~/PACKAGE_NAME/` or individual files in `~/`

## Configuration

dotctl uses a `dotctl.json` file in your dotfiles directory. This file is automatically created with sensible defaults.

### Example Configuration

```json
{
  "packages": {
    "nvim": "all",
    "tmux": "macos",
    "bat": "all",
    "shell": "all",
    ".oh-my-zsh": "all"
  },
  "global_excludes": [
    ".git",
    ".DS_Store",
    "*.pyc",
    "__pycache__"
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

## Adopting New Packages

When you install new applications that create config directories in `~/.config/`, you can easily adopt them:

```bash
# Install new app (creates ~/.config/new-app/)
sudo pacman -S new-app

# Adopt it into dotctl
dotctl adopt new-app arch        # For specific systems
dotctl adopt new-app             # For all systems
dotctl adopt                     # Adopt all new packages

# Now it's managed by dotctl
dotctl status                    # Shows new-app as deployable
dotctl deploy new-app            # Deploy on other systems
```

The adopt command:
- **Moves** `~/.config/PACKAGE/` → `~/.dotfiles/PACKAGE/`
- **Creates symlink** `~/.config/PACKAGE/` → `../.dotfiles/PACKAGE/`
- **Adds to configuration** with specified systems
- **Preserves functionality** - apps continue working normally

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
├── nvim/                # Config package → ~/.config/nvim/
│   ├── init.lua
│   └── lua/
├── tmux/                # Config package → ~/.config/tmux/
│   └── tmux.conf
├── shell/               # Home package → ~/.zshrc, ~/.bashrc
│   ├── .zshrc
│   └── .bashrc
└── .oh-my-zsh/          # Home package → ~/.oh-my-zsh/
    └── themes/
```

When deployed, dotctl creates symlinks:
- **Config packages**: `~/.config/nvim/` → `../.dotfiles/nvim/`
- **Shell package**: `~/.zshrc` → `.dotfiles/shell/.zshrc`
- **Dot packages**: `~/.oh-my-zsh/` → `.dotfiles/.oh-my-zsh/`

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

dotctl uses native Go symlink functionality with system-awareness:

1. **System Detection**: Automatically detects your operating system and Linux distribution
2. **Package Filtering**: Only deploys packages configured for your current system
3. **Native Symlinks**: Uses Go's `os.Symlink()` to create and manage symlinks
4. **Smart Targeting**: Config packages go to `~/.config/`, home packages go to `~/`
5. **Configuration Management**: Maintains a JSON configuration file for package-to-system mappings

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]