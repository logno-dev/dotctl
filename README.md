# dotctl

A system-aware dotfiles manager built in Go with native symlink management.

## Features

- **System-aware deployment**: Configure packages for specific operating systems (Linux, macOS, Arch, Ubuntu, etc.)
- **Template system**: Create system-specific configurations with conditional blocks
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

dotctl uses a `dotctl.yaml` file in your dotfiles directory. This file is automatically created with sensible defaults and supports comments for better documentation.

### Example Configuration

```yaml
# dotctl configuration file
# This file defines your dotfiles packages and their target systems

packages:
  # Simple system assignments
  nvim: all           # Neovim config for all systems
  tmux: macos         # tmux only on macOS
  bat: all            # bat config for all systems  
  shell: all          # Shell configs for all systems
  .oh-my-zsh: all     # Oh My Zsh for all systems
  
  # Multiple systems assignment
  git:                # Git config for specific systems
    systems:
      - linux
      - macos
    description: "Git configuration files"
  
  hyprland:           # Hyprland only on Arch
    systems:
      - arch
    description: "Hyprland window manager config"

# Files and directories to exclude from all packages
global_excludes:
  - .git
  - .DS_Store
  - "*.pyc"
  - __pycache__

# GitHub integration settings
github:
  repository: username/my-dotfiles  # Your GitHub repository
  branch: main                      # Target branch (optional, defaults to main)
```

### Supported Systems

- `all` - Deploy on all systems
- `linux` - Any Linux distribution
- `macos` - macOS
- `arch` - Arch Linux
- `ubuntu` - Ubuntu
- `debian` - Debian
- `fedora` - Fedora

## Template System

dotctl supports a powerful template system that allows you to create system-specific configurations while maintaining a single source file. This is perfect for configs that need minor differences between operating systems.

### How Templates Work

1. **Create template files** with a `.template` extension
2. **Use conditional blocks** to include system-specific content
3. **Deploy normally** - dotctl automatically processes templates during deployment

### Template Syntax

Use conditional blocks to include content for specific systems:

```bash
{{#if macos}}
# macOS-specific content
export PATH="/opt/homebrew/bin:$PATH"
eval "$(/opt/homebrew/bin/brew shellenv)"
{{/if}}

{{#if arch}}
# Arch Linux-specific content
export PATH="/usr/local/bin:$PATH"
alias pacman='sudo pacman'
{{/if}}

{{#if linux}}
# Any Linux distribution
export EDITOR=nvim
{{/if}}

# Common content for all systems
alias ll='ls -alF'
alias grep='grep --color=auto'
```

### Template Examples

#### Shell Configuration (`.zshrc.template`)

```bash
# Path to your oh-my-zsh installation
export ZSH="$HOME/.oh-my-zsh"

{{#if macos}}
# macOS-specific paths
eval "$(/opt/homebrew/bin/brew shellenv)"
export PATH="/opt/homebrew/bin:$PATH"
{{/if}}

{{#if arch}}
# Arch Linux-specific paths
export PATH="/usr/local/bin:$PATH"
{{/if}}

# Common configuration
export EDITOR=nvim
ZSH_THEME="robbyrussell"
source $ZSH/oh-my-zsh.sh
```

#### Tmux Configuration (`tmux.conf.template`)

```bash
{{#if macos}}
# macOS: Use Ctrl+x as prefix
set-option -g prefix C-x
bind-key C-x send-prefix
{{/if}}

{{#if arch}}
# Arch Linux: Use Ctrl+d as prefix  
set-option -g prefix C-d
bind-key C-d send-prefix
{{/if}}

# Common tmux settings
unbind C-b
set -s escape-time 0
bind | split-window -h
bind - split-window -v
```

### Using Templates

```bash
# Create your template files
~/.dotfiles/shell/.zshrc.template
~/.dotfiles/tmux/tmux.conf.template

# Deploy packages - templates are processed automatically
dotctl deploy shell tmux

# Results in system-specific configs:
# ~/.zshrc (with Arch-specific paths on Arch, macOS paths on macOS)
# ~/.config/tmux/tmux.conf (with Ctrl+d on Arch, Ctrl+x on macOS)
```

### Available Conditions

- `{{#if macos}}` - macOS only
- `{{#if linux}}` - Any Linux distribution
- `{{#if arch}}` - Arch Linux only
- `{{#if ubuntu}}` - Ubuntu only
- `{{#if debian}}` - Debian only
- `{{#if fedora}}` - Fedora only

### Template Benefits

- **Single source of truth**: One template file instead of multiple system-specific files
- **Automatic processing**: Templates processed during deployment
- **System-aware**: Different content for different operating systems
- **Maintainable**: Easy to add new systems or modify shared content
- **Flexible**: Works with any text-based configuration file

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
- `dotctl sync` intelligently handles both local and upstream changes:
  1. **Fetches upstream changes** to check if the remote has updates
  2. **Stashes local changes** if needed before pulling upstream updates
  3. **Pulls upstream changes** and merges them with your local repository
  4. **Restores local changes** and handles any merge conflicts
  5. **Commits and pushes** your changes to the remote repository
- `dotctl pull` pulls the latest changes from the configured branch
- If the dotfiles directory doesn't exist when pulling, it will clone the repository
- **Merge conflict handling**: If conflicts occur during sync, dotctl will notify you and provide guidance for manual resolution

## Directory Structure

Your dotfiles directory should be organized like this:

```
~/.dotfiles/
├── dotctl.yaml          # Configuration file
├── nvim/                # Config package → ~/.config/nvim/
│   ├── init.lua
│   └── lua/
├── tmux/                # Config package → ~/.config/tmux/
│   ├── tmux.conf.template    # Template file (processed during deployment)
│   └── tmux.conf            # Generated from template
├── shell/               # Home package → ~/.zshrc, ~/.bashrc
│   ├── .zshrc.template      # Template file (processed during deployment)
│   ├── .zshrc               # Generated from template
│   └── .bashrc
└── .oh-my-zsh/          # Home package → ~/.oh-my-zsh/
    └── themes/
```

When deployed, dotctl:
- **Processes templates**: `.template` files → system-specific configs
- **Creates symlinks**: 
  - **Config packages**: `~/.config/nvim/` → `../.dotfiles/nvim/`
  - **Shell package**: `~/.zshrc` → `.dotfiles/shell/.zshrc` (generated from template)
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