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

dotctl automatically determines where packages should be deployed based on their names and configuration:

### Config Packages (→ `~/.config/`)
- **Any package name that doesn't start with `.` and isn't named `shell`**
- Examples: `nvim`, `tmux`, `bat`, `gh`, `kitty`
- Creates: `~/.config/PACKAGE_NAME/` → `../.dotfiles/PACKAGE_NAME/`

### Home Packages (→ `~/`)
- **Packages starting with `.`**: `.oh-my-zsh`, `.vim`, etc.
- **`shell` package**: Contains shell configs like `.zshrc`, `.bashrc`
- **Packages with `home: true` setting**: Any package can be forced to deploy to `~/` instead of `~/.config/`
- Creates: `~/PACKAGE_NAME/` or individual files in `~/`

### Home Setting Override
You can force any package to be symlinked to the `$HOME` directory instead of `~/.config/` by using the `home` setting:

```yaml
packages:
  my-scripts:
    systems: [all]
    home: true        # Forces symlink to ~/my-scripts instead of ~/.config/my-scripts
    description: "Custom scripts directory"
```

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

  # Advanced package configuration with home setting
  my-scripts:
    systems: [all]
    home: true        # Symlink to $HOME instead of ~/.config
    description: "Custom scripts directory"

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

### Package Configuration Options

When using the extended package configuration format, you can specify:

- **`systems`**: Array of systems where the package should be deployed
- **`description`**: Optional description of the package
- **`home`**: Boolean flag to force symlink to `$HOME` instead of `~/.config/`

```yaml
packages:
  # Simple format (deploys to default location based on package name)
  nvim: all
  
  # Extended format with home setting
  my-dotfiles:
    systems: [linux, macos]
    home: true
    description: "Personal configuration files"
```

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
  5. **Detects template conflicts** between template files and base config files
  6. **Commits and pushes** your changes to the remote repository
- `dotctl pull` pulls the latest changes from the configured branch
- If the dotfiles directory doesn't exist when pulling, it will clone the repository
- **Merge conflict handling**: If conflicts occur during sync, dotctl will notify you and provide guidance for manual resolution

## Template Merging

When working with templates, you might encounter situations where:
- You've manually edited a generated base config file (e.g., `.zshrc`)
- The template file (e.g., `.zshrc.template`) has been updated remotely
- Both files have conflicting changes that need to be merged

dotctl provides an intelligent merge system to handle these conflicts interactively.

### Understanding Template Conflicts

A template conflict occurs when:
1. A `.template` file exists (e.g., `shell/.zshrc.template`)
2. The generated base file exists (e.g., `shell/.zshrc`)
3. The base file has been manually edited
4. The template would generate different content than what's in the base file

This commonly happens when:
- You make quick edits to the generated config file instead of the template
- Remote changes to the template haven't been merged with your local edits
- Different systems have diverged in their configurations

### Checking for Template Conflicts

Before syncing, you can check if there are any template conflicts:

```bash
dotctl merge-check
```

This will scan all packages and report any conflicts between template files and their generated base files, showing:
- Which files have conflicts
- The location of the template and base files
- Line count differences

### Resolving Template Conflicts

To interactively resolve template conflicts:

```bash
dotctl merge-resolve
```

For each conflict, you'll be presented with 8 options:

#### Option 1: Keep Local Changes in Base File
Keeps your manually edited base file as-is, ignoring the template. Use this when:
- Your local edits are exactly what you want
- The template changes aren't relevant to your setup
- You plan to update the template later to match your edits
- **Note**: This doesn't update the template, so the divergence will persist

#### Option 2: Use Template Output
Discards your local edits and uses the template-generated content. Use this when:
- The template is the source of truth
- Your local edits were temporary or experimental
- Remote template changes should override local modifications

#### Option 3: Update Template with Base File Changes (Smart Merge)
**This is the key feature for propagating changes back to the template!**

Intelligently analyzes your changes and merges them into the template while preserving conditional blocks:

**How it works:**
1. **Parses template structure**: Identifies all `{{#if system}}` blocks and common sections
2. **Compares line-by-line**: Detects what lines were added, removed, or modified
3. **Maps changes to sections**: Determines which template section each change belongs to
4. **Shows analysis**: Displays where each change will be placed
5. **Offers auto-merge or manual edit**

**Example:**
```
=== SMART MERGE ANALYSIS ===
Found 3 difference(s) between base file and template output
Template has 3 section(s):
  - Conditional block for 'macos' (5 lines)
  - Common section (12 lines)
  - Conditional block for 'linux' (4 lines)

Change placement analysis:
1. ADD: alias myalias='echo hello'
   → Suggested: Add to common section
2. MODIFY: alias ll='ls -la' -> alias ll='ls -laF'
   → Suggested: Add to common section
3. ADD: export HOMEBREW_NO_ANALYTICS=1
   → Suggested: Add to conditional block for 'macos'

Options:
  1. Auto-merge (apply all suggestions)
  2. Manual edit (I'll help you place changes)
  3. Cancel
```

**Auto-merge** will:
- Insert new lines in the recommended sections
- Update modified lines in place
- Preserve all conditional blocks
- Maintain template structure

If template has no conditionals, it offers simple replacement instead.

#### Option 4: Merge Base Changes into Template Interactively
**The smart way to update templates with conditional blocks!**

Opens an interactive workflow to help you merge base file changes into the template while preserving conditional blocks:

1. Analyzes the template structure and shows:
   - How many lines are in conditional blocks for each system
   - How many common lines exist outside conditionals

2. Presents sub-options:
   - **Edit template manually**: Opens your editor with the template and a reference file containing your base file changes side-by-side (in vim/nvim)
   - **Show base file content**: View your base file changes to help with manual editing
   - **Replace entire template**: Same as Option 3 (lose conditionals)
   - **Cancel**: Return to main menu

When editing manually:
- The template opens in your `$EDITOR` (vim opens with vertical split)
- A reference file shows your base file changes
- You manually copy relevant changes into the appropriate sections
- Conditional blocks (`{{#if system}}`) are preserved
- Common changes go outside conditional blocks

#### Option 5: Show Diff
Displays a side-by-side comparison of:
- Your local changes (current base file)
- What the template would generate

This helps you understand what's different before making a decision.

#### Option 6: Three-Way Merge View
Shows a comprehensive comparison of all versions:
1. **Local changes** - Your current base file
2. **Remote base** - The base file from origin/main (if available)
3. **Remote template output** - What the remote template would generate
4. **Current template output** - What your local template would generate

This is the most informative option when both local and remote changes exist.

#### Option 7: Manual Edit Base File
Opens your `$EDITOR` (defaults to vim) with merge conflict markers for the base file:

```bash
<<<<<<< LOCAL (your changes)
# Your local edits
export PATH="/custom/path:$PATH"
=======
# Template-generated content
export PATH="/opt/homebrew/bin:$PATH"
>>>>>>> TEMPLATE (generated from .zshrc.template)
```

Edit the file to resolve the conflict, remove the markers, and save. This gives you complete control to:
- Combine changes from both versions
- Cherry-pick specific lines
- Add new content
- Carefully craft the final result

**Note**: This only edits the base file, not the template. Consider using Option 4 to update the template as well.

#### Option 8: Skip
Skips this particular file for now. Use this when:
- You need more time to decide
- You want to handle the conflict manually later
- You're not sure which option is best yet

### Automatic Merge Detection During Sync

Template conflict detection is automatically integrated into the sync workflow:

```bash
dotctl sync
```

During sync, if template conflicts are detected:
1. You'll be prompted to resolve them interactively
2. Each resolved conflict is automatically staged for commit
3. After all conflicts are resolved, sync continues normally
4. Changes are committed and pushed to GitHub

You can decline the interactive resolution and handle conflicts manually:

```bash
# When prompted during sync:
Resolve conflicts interactively? [Y/n]: n
```

### Best Practices

1. **Edit templates, not base files**: Always edit the `.template` file, not the generated file
2. **Check before sync**: Run `dotctl merge-check` before `dotctl sync` to catch conflicts early
3. **Use three-way merge**: When in doubt, view all versions with option 4
4. **Keep templates in sync**: Regularly merge local edits back into templates
5. **Document template changes**: Add comments explaining system-specific sections

### Example Workflow: Simple Template (No Conditionals)

```bash
# You've edited ~/.bashrc directly instead of the template
vim ~/.bashrc
# Added: alias myalias='echo hello'

# Check for conflicts
dotctl merge-check
# Found 1 template merge conflict(s):
# 1. ~/.dotfiles/shell/.bashrc

# Resolve conflicts and update template (smart merge)
dotctl merge-resolve
# [1/1] Resolving conflict for: ~/.dotfiles/shell/.bashrc
# Options:
#   3. Update template with base file changes (propagate changes)
# Choice [1-8]: 3

# Template has no conditional blocks. Using direct replacement.
# ⚠️  This will replace the template file content...
# Continue? [y/N]: y
# ✓ Updated template: ~/.dotfiles/shell/.bashrc.template
# ✓ Resolved and staged ~/.dotfiles/shell/.bashrc

# Now sync your changes
dotctl sync
# ✓ Successfully synced with GitHub
```

### Example Workflow: Smart Auto-Merge (With Conditionals)

```bash
# You have a template with system-specific sections
cat ~/.dotfiles/shell/.zshrc.template
# {{#if macos}}
# export PATH="/opt/homebrew/bin:$PATH"
# {{/if}}
# {{#if linux}}
# export PATH="/usr/local/bin:$PATH"
# {{/if}}
# alias ll='ls -la'

# You edited the base file directly on macOS
vim ~/.zshrc
# Added: alias myalias='echo hello'
# Changed: alias ll='ls -laF'  (added F flag)
# Added: export HOMEBREW_NO_ANALYTICS=1

# Resolve with smart merge
dotctl merge-resolve
# [1/1] Resolving conflict for: ~/.dotfiles/shell/.zshrc
# Choice [1-8]: 3  (Update template with base file changes)

# === SMART MERGE ANALYSIS ===
# Found 3 difference(s) between base file and template output
# Template has 3 section(s):
#   - Conditional block for 'macos' (1 lines)
#   - Common section (1 lines)
#   - Conditional block for 'linux' (1 lines)
#
# Change placement analysis:
# 1. ADD: alias myalias='echo hello'
#    → Suggested: Add to common section
# 2. MODIFY: alias ll='ls -la' -> alias ll='ls -laF'
#    → Suggested: Add to common section (high confidence)
# 3. ADD: export HOMEBREW_NO_ANALYTICS=1
#    → Suggested: Add to conditional block for 'macos' (high confidence)
#
# Options:
#   1. Auto-merge (apply all suggestions)
#   2. Manual edit (I'll help you place changes)
#   3. Cancel
# Choice [1-3]: 1

# ✓ Auto-merged changes into template: ~/.dotfiles/shell/.zshrc.template
# ✓ Resolved and staged ~/.dotfiles/shell/.zshrc

# Result:
cat ~/.dotfiles/shell/.zshrc.template
# {{#if macos}}
# export PATH="/opt/homebrew/bin:$PATH"
# export HOMEBREW_NO_ANALYTICS=1    # ← Auto-added here!
# {{/if}}
# {{#if linux}}
# export PATH="/usr/local/bin:$PATH"
# {{/if}}
# alias ll='ls -laF'                 # ← Auto-modified!
# alias myalias='echo hello'         # ← Auto-added here!

# Conditionals preserved, changes intelligently placed!
```

### Under the Hood

The merge system:
- Detects template files (`.template` extension)
- Processes templates with system-specific conditions
- Compares processed output with actual base files
- Identifies differences and creates conflict records
- Fetches remote versions from git for three-way comparison
- Stages resolved files automatically
- Integrates seamlessly with the sync workflow

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

Dotctl is released under the [MIT License](https://opensource.org/licenses/MIT).

## Contributing

Contributions welcome.
