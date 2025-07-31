package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type PackageConfig struct {
	Systems     []string `json:"systems,omitempty"`
	Description string   `json:"description,omitempty"`
}

type GitHubConfig struct {
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

type Config struct {
	Packages       map[string]interface{} `json:"packages"`
	GlobalExcludes []string               `json:"global_excludes"`
	StowOptions    []string               `json:"stow_options"`
	GitHub         *GitHubConfig          `json:"github,omitempty"`
}

type DotfilesManager struct {
	DotfilesDir string
	ConfigFile  string
	System      string
	Config      *Config
}

func NewDotfilesManager(dotfilesDir string) (*DotfilesManager, error) {
	if dotfilesDir == "" {
		// First, check if we're already in a dotfiles directory (contains dotctl.json)
		if cwd, err := os.Getwd(); err == nil {
			if _, err := os.Stat(filepath.Join(cwd, "dotctl.json")); err == nil {
				dotfilesDir = cwd
			}
		}

		// If not found in current directory, use default location
		if dotfilesDir == "" {
			usr, err := user.Current()
			if err != nil {
				return nil, fmt.Errorf("failed to get current user: %w", err)
			}
			dotfilesDir = filepath.Join(usr.HomeDir, ".dotfiles")
		}
	}

	manager := &DotfilesManager{
		DotfilesDir: dotfilesDir,
		ConfigFile:  filepath.Join(dotfilesDir, "dotctl.json"),
		System:      detectSystem(),
	}

	config, err := manager.loadConfig()
	if err != nil {
		return nil, err
	}
	manager.Config = config

	return manager, nil
}

func detectSystem() string {
	system := runtime.GOOS
	switch system {
	case "darwin":
		return "macos"
	case "linux":
		return detectLinuxDistro()
	default:
		return system
	}
}

func detectLinuxDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux"
	}

	content := strings.ToLower(string(data))
	switch {
	case strings.Contains(content, "arch"):
		return "arch"
	case strings.Contains(content, "ubuntu"):
		return "ubuntu"
	case strings.Contains(content, "debian"):
		return "debian"
	case strings.Contains(content, "fedora"):
		return "fedora"
	default:
		return "linux"
	}
}

func (dm *DotfilesManager) loadConfig() (*Config, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	defaultConfig := &Config{
		Packages:       make(map[string]interface{}),
		GlobalExcludes: []string{".git", ".DS_Store", "*.pyc", "__pycache__"},
		StowOptions:    []string{"--verbose", "--target=" + usr.HomeDir},
	}

	if _, err := os.Stat(dm.ConfigFile); os.IsNotExist(err) {
		// Don't create config automatically - let init command handle it
		return defaultConfig, nil
	}

	data, err := os.ReadFile(dm.ConfigFile)
	if err != nil {
		return defaultConfig, nil
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		return defaultConfig, nil
	}

	// Merge with defaults
	if config.Packages == nil {
		config.Packages = make(map[string]interface{})
	}
	if config.GlobalExcludes == nil {
		config.GlobalExcludes = defaultConfig.GlobalExcludes
	}
	if config.StowOptions == nil {
		config.StowOptions = defaultConfig.StowOptions
	}

	return &config, nil
}

func (dm *DotfilesManager) saveConfig(config *Config) error {
	if config != nil {
		dm.Config = config
	}

	// Ensure dotfiles directory exists
	if err := os.MkdirAll(dm.DotfilesDir, 0755); err != nil {
		return fmt.Errorf("failed to create dotfiles directory: %w", err)
	}

	data, err := json.MarshalIndent(dm.Config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(dm.ConfigFile, data, 0644)
}

func (dm *DotfilesManager) isStowAvailable() bool {
	_, err := exec.LookPath("stow")
	return err == nil
}

func (dm *DotfilesManager) isGitHubCLIAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func (dm *DotfilesManager) isGitHubAuthenticated() bool {
	if !dm.isGitHubCLIAvailable() {
		return false
	}

	cmd := exec.Command("gh", "auth", "status")
	err := cmd.Run()
	return err == nil
}

func (dm *DotfilesManager) getPackagesForSystem(system string) []string {
	if system == "" {
		system = dm.System
	}

	var packages []string
	for packageName, packageConfig := range dm.Config.Packages {
		if shouldDeployPackage(packageConfig, system) {
			packages = append(packages, packageName)
		}
	}

	sort.Strings(packages)
	return packages
}

func shouldDeployPackage(packageConfig interface{}, system string) bool {
	switch config := packageConfig.(type) {
	case string:
		return config == "all" || config == system
	case map[string]interface{}:
		systemsInterface, exists := config["systems"]
		if !exists {
			return true // Default to all systems
		}

		systemsSlice, ok := systemsInterface.([]interface{})
		if !ok {
			return false
		}

		for _, sys := range systemsSlice {
			if sysStr, ok := sys.(string); ok {
				if sysStr == "all" || sysStr == system {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

func (dm *DotfilesManager) scanPackages() ([]string, error) {
	var packages []string

	if _, err := os.Stat(dm.DotfilesDir); os.IsNotExist(err) {
		return packages, nil
	}

	entries, err := os.ReadDir(dm.DotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dotfiles directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		// Skip git directory, config files, and cache directories
		if name == ".git" || name == "dotctl.json" || name == "__pycache__" || strings.HasSuffix(name, ".tmp") {
			continue
		}

		// Include directories (both regular and dotfiles)
		if entry.IsDir() {
			// Special handling for .config directory - scan its subdirectories as individual packages
			if name == ".config" {
				configPackages, err := dm.scanConfigPackages()
				if err != nil {
					return nil, fmt.Errorf("failed to scan .config packages: %w", err)
				}
				packages = append(packages, configPackages...)
			} else {
				packages = append(packages, name)
			}
		}
	}

	sort.Strings(packages)
	return packages, nil
}

func (dm *DotfilesManager) scanConfigPackages() ([]string, error) {
	var packages []string
	configDir := filepath.Join(dm.DotfilesDir, ".config")

	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read .config directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files and common non-package items
		if strings.HasPrefix(name, ".") || name == "__pycache__" || strings.HasSuffix(name, ".tmp") {
			continue
		}

		// Include directories as config packages with .config/ prefix
		if entry.IsDir() {
			packages = append(packages, ".config/"+name)
		}
	}

	return packages, nil
}

func (dm *DotfilesManager) deployPackage(packageName string, dryRun bool) error {
	var packageDir string
	var stowPackageName string

	// Handle .config packages specially
	if strings.HasPrefix(packageName, ".config/") {
		// For .config packages, we need to stow from the .config directory
		// but only the specific subdirectory
		configSubdir := strings.TrimPrefix(packageName, ".config/")
		packageDir = filepath.Join(dm.DotfilesDir, ".config", configSubdir)
		stowPackageName = configSubdir
	} else {
		packageDir = filepath.Join(dm.DotfilesDir, packageName)
		stowPackageName = packageName
	}

	if _, err := os.Stat(packageDir); os.IsNotExist(err) {
		return fmt.Errorf("package '%s' not found at %s", packageName, packageDir)
	}

	if !dm.isStowAvailable() {
		return fmt.Errorf("GNU stow is not available. Please install it first:\n" +
			"  - On macOS: brew install stow\n" +
			"  - On Arch: sudo pacman -S stow\n" +
			"  - On Ubuntu/Debian: sudo apt install stow")
	}

	// Build stow command
	args := []string{"stow"}
	args = append(args, dm.Config.StowOptions...)
	args = append(args, stowPackageName)

	if dryRun {
		args = append(args, "--no")
		fmt.Printf("DRY RUN: Would execute: %s\n", strings.Join(args, " "))
	} else {
		fmt.Printf("Deploying %s...\n", packageName)
	}

	cmd := exec.Command("stow", args[1:]...)

	// Set working directory based on package type
	if strings.HasPrefix(packageName, ".config/") {
		cmd.Dir = filepath.Join(dm.DotfilesDir, ".config")
	} else {
		cmd.Dir = dm.DotfilesDir
	}

	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("error deploying %s: %w\nOutput: %s", packageName, err, string(output))
	}

	if !dryRun {
		fmt.Printf("✓ Successfully deployed %s\n", packageName)
	}
	if len(output) > 0 {
		fmt.Printf("%s\n", string(output))
	}

	return nil
}

func (dm *DotfilesManager) undeployPackage(packageName string, dryRun bool) error {
	if !dm.isStowAvailable() {
		return fmt.Errorf("GNU stow is not available")
	}

	var stowPackageName string

	// Handle .config packages specially
	if strings.HasPrefix(packageName, ".config/") {
		stowPackageName = strings.TrimPrefix(packageName, ".config/")
	} else {
		stowPackageName = packageName
	}

	args := []string{"--delete"}
	args = append(args, dm.Config.StowOptions...)
	args = append(args, stowPackageName)

	if dryRun {
		args = append(args, "--no")
		fmt.Printf("DRY RUN: Would execute: stow %s\n", strings.Join(args, " "))
	} else {
		fmt.Printf("Undeploying %s...\n", packageName)
	}

	cmd := exec.Command("stow", args...)

	// Set working directory based on package type
	if strings.HasPrefix(packageName, ".config/") {
		cmd.Dir = filepath.Join(dm.DotfilesDir, ".config")
	} else {
		cmd.Dir = dm.DotfilesDir
	}

	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("error undeploying %s: %w\nOutput: %s", packageName, err, string(output))
	}

	if !dryRun {
		fmt.Printf("✓ Successfully undeployed %s\n", packageName)
	}

	return nil
}

func (dm *DotfilesManager) deployAll(packages []string, dryRun bool) {
	if len(packages) == 0 {
		packages = dm.getPackagesForSystem("")
	}

	if len(packages) == 0 {
		fmt.Printf("No packages configured for system '%s'\n", dm.System)
		fmt.Printf("\nTo diagnose this issue, run: dotctl debug\n")
		fmt.Printf("Or check your configuration with: dotctl status\n")
		return
	}

	fmt.Printf("Deploying packages for %s: %s\n", dm.System, strings.Join(packages, ", "))

	successCount := 0
	for _, pkg := range packages {
		if err := dm.deployPackage(pkg, dryRun); err != nil {
			fmt.Printf("✗ %v\n", err)
		} else {
			successCount++
		}
	}

	fmt.Printf("\nDeployment complete: %d/%d packages successful\n", successCount, len(packages))
}

func (dm *DotfilesManager) undeployAll(packages []string, dryRun bool) {
	if len(packages) == 0 {
		packages = dm.getPackagesForSystem("")
	}

	if len(packages) == 0 {
		fmt.Println("No packages to undeploy")
		return
	}

	fmt.Printf("Undeploying packages: %s\n", strings.Join(packages, ", "))

	successCount := 0
	for _, pkg := range packages {
		if err := dm.undeployPackage(pkg, dryRun); err != nil {
			fmt.Printf("✗ %v\n", err)
		} else {
			successCount++
		}
	}

	fmt.Printf("\nUndeployment complete: %d/%d packages successful\n", successCount, len(packages))
}

func (dm *DotfilesManager) status() error {
	fmt.Printf("Dotfiles directory: %s\n", dm.DotfilesDir)
	fmt.Printf("Current system: %s\n", dm.System)
	fmt.Printf("GNU stow available: %s\n", boolToCheckmark(dm.isStowAvailable()))
	fmt.Printf("GitHub CLI available: %s\n", boolToCheckmark(dm.isGitHubCLIAvailable()))
	if dm.isGitHubCLIAvailable() {
		fmt.Printf("GitHub authenticated: %s\n", boolToCheckmark(dm.isGitHubAuthenticated()))
	}
	if dm.Config.GitHub != nil && dm.Config.GitHub.Repository != "" {
		fmt.Printf("GitHub repository: %s\n", dm.Config.GitHub.Repository)
		if dm.Config.GitHub.Branch != "" {
			fmt.Printf("GitHub branch: %s\n", dm.Config.GitHub.Branch)
		}
	}
	fmt.Println()

	allPackages, err := dm.scanPackages()
	if err != nil {
		return err
	}

	configuredPackages := make(map[string]bool)
	for pkg := range dm.Config.Packages {
		configuredPackages[pkg] = true
	}

	deployablePackages := make(map[string]bool)
	for _, pkg := range dm.getPackagesForSystem("") {
		deployablePackages[pkg] = true
	}

	fmt.Println("Package status:")
	for _, pkg := range allPackages {
		var statusParts []string
		if configuredPackages[pkg] {
			if deployablePackages[pkg] {
				statusParts = append(statusParts, "✓ deployable")
			} else {
				statusParts = append(statusParts, "- not for this system")
			}
		} else {
			statusParts = append(statusParts, "? not configured")
		}
		fmt.Printf("  %s: %s\n", pkg, strings.Join(statusParts, ", "))
	}

	// Show orphaned config entries
	var orphaned []string
	for pkg := range configuredPackages {
		found := false
		for _, available := range allPackages {
			if available == pkg {
				found = true
				break
			}
		}
		if !found {
			orphaned = append(orphaned, pkg)
		}
	}

	if len(orphaned) > 0 {
		sort.Strings(orphaned)
		fmt.Printf("\nOrphaned config entries: %s\n", strings.Join(orphaned, ", "))
	}

	return nil
}

func (dm *DotfilesManager) addPackage(packageName string, systems []string) error {
	if len(systems) == 0 {
		systems = []string{"all"}
	}

	if len(systems) == 1 && isSimpleSystem(systems[0]) {
		dm.Config.Packages[packageName] = systems[0]
	} else {
		dm.Config.Packages[packageName] = map[string]interface{}{
			"systems": systems,
		}
	}

	if err := dm.saveConfig(nil); err != nil {
		return err
	}

	fmt.Printf("Added package '%s' for systems: %s\n", packageName, strings.Join(systems, ", "))
	return nil
}

func (dm *DotfilesManager) removePackage(packageName string) error {
	if _, exists := dm.Config.Packages[packageName]; !exists {
		fmt.Printf("Package '%s' not found in configuration\n", packageName)
		return nil
	}

	delete(dm.Config.Packages, packageName)
	if err := dm.saveConfig(nil); err != nil {
		return err
	}

	fmt.Printf("Removed package '%s' from configuration\n", packageName)
	return nil
}

func (dm *DotfilesManager) initializeConfig(dryRun bool) error {
	// Check if config already exists
	if _, err := os.Stat(dm.ConfigFile); err == nil {
		fmt.Printf("Configuration file already exists at %s\n", dm.ConfigFile)
		fmt.Println("Run 'dotctl status' to see current configuration")
		return nil
	}

	// Scan for packages
	packages, err := dm.scanPackages()
	if err != nil {
		return fmt.Errorf("failed to scan packages: %w", err)
	}

	if len(packages) == 0 {
		fmt.Printf("No package directories found in %s\n", dm.DotfilesDir)
		fmt.Println("Create package directories first, then run 'dotctl init'")
		return nil
	}

	if dryRun {
		fmt.Printf("DRY RUN: Would create configuration with packages: %s\n", strings.Join(packages, ", "))
		fmt.Printf("DRY RUN: All packages would be configured for current system: %s\n", dm.System)
		return nil
	}

	// Create new config with detected packages
	newConfig := &Config{
		Packages:       make(map[string]interface{}),
		GlobalExcludes: []string{".git", ".DS_Store", "*.pyc", "__pycache__"},
		StowOptions:    []string{"--verbose"},
		GitHub: &GitHubConfig{
			Repository: "username/dotfiles", // Replace with your GitHub repository
			Branch:     "main",
		},
	}

	// Set target directory
	usr, err := user.Current()
	if err == nil {
		newConfig.StowOptions = append(newConfig.StowOptions, "--target="+usr.HomeDir)
	}

	// Add all detected packages for current system
	for _, pkg := range packages {
		newConfig.Packages[pkg] = dm.System
	}

	// Save the configuration
	dm.Config = newConfig
	if err := dm.saveConfig(nil); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✓ Initialized configuration with %d packages for system '%s'\n", len(packages), dm.System)
	fmt.Printf("Packages configured: %s\n", strings.Join(packages, ", "))
	fmt.Printf("Configuration saved to: %s\n", dm.ConfigFile)
	fmt.Println("\nYou can now run 'dotctl deploy' to deploy your dotfiles")
	fmt.Println("Use 'dotctl add <package> <systems...>' to configure packages for other systems")

	return nil
}

func (dm *DotfilesManager) setGitHubRepo(repository, branch string) error {
	if dm.Config.GitHub == nil {
		dm.Config.GitHub = &GitHubConfig{}
	}

	dm.Config.GitHub.Repository = repository
	if branch != "" {
		dm.Config.GitHub.Branch = branch
	} else {
		dm.Config.GitHub.Branch = "main"
	}

	if err := dm.saveConfig(nil); err != nil {
		return err
	}

	fmt.Printf("Set GitHub repository to '%s' (branch: %s)\n", repository, dm.Config.GitHub.Branch)
	return nil
}

func (dm *DotfilesManager) syncToGitHub(dryRun bool) error {
	if dm.Config.GitHub == nil || dm.Config.GitHub.Repository == "" {
		return fmt.Errorf("no GitHub repository configured. Use 'dotctl github-repo <owner/repo>' first")
	}

	if !dm.isGitHubCLIAvailable() {
		return fmt.Errorf("GitHub CLI (gh) is not available. Please install it:\n" +
			"  - Visit: https://cli.github.com/\n" +
			"  - Or use: brew install gh")
	}

	if !dm.isGitHubAuthenticated() {
		return fmt.Errorf("GitHub CLI is not authenticated. Run 'gh auth login' first")
	}

	// Check if dotfiles directory is a git repository
	gitDir := filepath.Join(dm.DotfilesDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if dryRun {
			fmt.Printf("DRY RUN: Would initialize git repository in %s\n", dm.DotfilesDir)
			fmt.Printf("DRY RUN: Would add remote origin %s\n", dm.Config.GitHub.Repository)
		} else {
			fmt.Printf("Initializing git repository in %s...\n", dm.DotfilesDir)
			if err := dm.runGitCommand("init"); err != nil {
				return fmt.Errorf("failed to initialize git repository: %w", err)
			}

			// Add remote origin
			repoURL := fmt.Sprintf("https://github.com/%s.git", dm.Config.GitHub.Repository)
			if err := dm.runGitCommand("remote", "add", "origin", repoURL); err != nil {
				return fmt.Errorf("failed to add remote origin: %w", err)
			}
		}
	}

	branch := dm.Config.GitHub.Branch
	if branch == "" {
		branch = "main"
	}

	if dryRun {
		fmt.Printf("DRY RUN: Would add all files to git\n")
		fmt.Printf("DRY RUN: Would commit changes\n")
		fmt.Printf("DRY RUN: Would push to %s:%s\n", dm.Config.GitHub.Repository, branch)
		return nil
	}

	fmt.Printf("Syncing to GitHub repository %s...\n", dm.Config.GitHub.Repository)

	// Add all files
	if err := dm.runGitCommand("add", "."); err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}

	// Check if there are changes to commit
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dm.DotfilesDir
	if err := cmd.Run(); err == nil {
		fmt.Println("No changes to commit")
		return nil
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Update dotfiles - %s", getCurrentTimestamp())
	if err := dm.runGitCommand("commit", "-m", commitMsg); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Push to GitHub
	if err := dm.runGitCommand("push", "-u", "origin", branch); err != nil {
		return fmt.Errorf("failed to push to GitHub: %w", err)
	}

	fmt.Printf("✓ Successfully synced to GitHub repository %s\n", dm.Config.GitHub.Repository)
	return nil
}

func (dm *DotfilesManager) pullFromGitHub(dryRun bool) error {
	if dm.Config.GitHub == nil || dm.Config.GitHub.Repository == "" {
		return fmt.Errorf("no GitHub repository configured")
	}

	if !dm.isGitHubCLIAvailable() {
		return fmt.Errorf("GitHub CLI (gh) is not available")
	}

	if !dm.isGitHubAuthenticated() {
		return fmt.Errorf("GitHub CLI is not authenticated. Run 'gh auth login' first")
	}

	// Check if dotfiles directory is a git repository
	gitDir := filepath.Join(dm.DotfilesDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if dryRun {
			fmt.Printf("DRY RUN: Would clone repository %s to %s\n", dm.Config.GitHub.Repository, dm.DotfilesDir)
		} else {
			fmt.Printf("Cloning repository %s...\n", dm.Config.GitHub.Repository)
			repoURL := fmt.Sprintf("https://github.com/%s.git", dm.Config.GitHub.Repository)

			// Clone to a temporary directory first, then move contents
			tempDir := dm.DotfilesDir + ".tmp"
			cmd := exec.Command("git", "clone", repoURL, tempDir)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to clone repository: %w", err)
			}

			// Move contents from temp directory to dotfiles directory
			if err := os.Rename(tempDir, dm.DotfilesDir); err != nil {
				return fmt.Errorf("failed to move cloned repository: %w", err)
			}
		}
		return nil
	}

	branch := dm.Config.GitHub.Branch
	if branch == "" {
		branch = "main"
	}

	if dryRun {
		fmt.Printf("DRY RUN: Would pull from %s:%s\n", dm.Config.GitHub.Repository, branch)
		return nil
	}

	fmt.Printf("Pulling from GitHub repository %s...\n", dm.Config.GitHub.Repository)

	// Pull changes
	if err := dm.runGitCommand("pull", "origin", branch); err != nil {
		return fmt.Errorf("failed to pull from GitHub: %w", err)
	}

	fmt.Printf("✓ Successfully pulled from GitHub repository %s\n", dm.Config.GitHub.Repository)
	return nil
}

func (dm *DotfilesManager) runGitCommand(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dm.DotfilesDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git command failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func getCurrentTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func isSimpleSystem(system string) bool {
	simple := []string{"all", "linux", "macos", "arch", "ubuntu", "debian", "fedora"}
	for _, s := range simple {
		if s == system {
			return true
		}
	}
	return false
}

func boolToCheckmark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func printUsage() {
	fmt.Println(`dotctl - System-aware dotfiles manager

Usage:
  dotctl <command> [options] [args]

Commands:
  init                    Initialize configuration by scanning package directories
  deploy [packages...]    Deploy packages (default: all for current system)
  undeploy [packages...]  Undeploy packages (default: all for current system)
  status                  Show current status
  add <package> [systems...] Add package to configuration
  remove <package>        Remove package from configuration
  github-repo <owner/repo> [branch] Set GitHub repository for sync
  sync                    Sync dotfiles to GitHub repository
  pull                    Pull dotfiles from GitHub repository

Options:
  --dotfiles-dir <path>   Path to dotfiles directory (default: ~/.dotfiles)
  --dry-run              Show what would be done without executing
  --help                 Show this help message

Examples:
  dotctl init                      # Initialize config from existing packages
  dotctl deploy                    # Deploy all packages for current system
  dotctl deploy vim tmux           # Deploy specific packages
  dotctl undeploy shell            # Undeploy specific package
  dotctl status                    # Show current status
  dotctl add vim linux macos      # Add vim package for Linux and macOS
  dotctl add shell all             # Add shell package for all systems
  dotctl remove vim                # Remove vim from configuration
  dotctl github-repo user/dotfiles # Set GitHub repository
  dotctl sync                      # Push dotfiles to GitHub
  dotctl pull                      # Pull dotfiles from GitHub
  dotctl --dry-run deploy          # Show what would be deployed`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var dotfilesDir string
	var dryRun bool
	var args []string

	// Simple argument parsing
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--help" || arg == "-h":
			printUsage()
			return
		case arg == "--dry-run":
			dryRun = true
		case arg == "--dotfiles-dir":
			if i+1 < len(os.Args) {
				dotfilesDir = os.Args[i+1]
				i++ // Skip next argument
			} else {
				fmt.Println("Error: --dotfiles-dir requires a path")
				os.Exit(1)
			}
		case strings.HasPrefix(arg, "--dotfiles-dir="):
			dotfilesDir = strings.TrimPrefix(arg, "--dotfiles-dir=")
		default:
			args = append(args, arg)
		}
	}

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]
	commandArgs := args[1:]

	manager, err := NewDotfilesManager(dotfilesDir)
	if err != nil {
		fmt.Printf("Error initializing dotfiles manager: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "init":
		if err := manager.initializeConfig(dryRun); err != nil {
			fmt.Printf("Error initializing configuration: %v\n", err)
			os.Exit(1)
		}

	case "deploy":
		manager.deployAll(commandArgs, dryRun)

	case "undeploy":
		manager.undeployAll(commandArgs, dryRun)

	case "status":
		if err := manager.status(); err != nil {
			fmt.Printf("Error getting status: %v\n", err)
			os.Exit(1)
		}

	case "add":
		if len(commandArgs) == 0 {
			fmt.Println("Error: add command requires a package name")
			os.Exit(1)
		}
		packageName := commandArgs[0]
		systems := commandArgs[1:]
		if err := manager.addPackage(packageName, systems); err != nil {
			fmt.Printf("Error adding package: %v\n", err)
			os.Exit(1)
		}

	case "remove":
		if len(commandArgs) == 0 {
			fmt.Println("Error: remove command requires a package name")
			os.Exit(1)
		}
		if err := manager.removePackage(commandArgs[0]); err != nil {
			fmt.Printf("Error removing package: %v\n", err)
			os.Exit(1)
		}

	case "github-repo":
		if len(commandArgs) == 0 {
			fmt.Println("Error: github-repo command requires a repository (owner/repo)")
			os.Exit(1)
		}
		repository := commandArgs[0]
		branch := ""
		if len(commandArgs) > 1 {
			branch = commandArgs[1]
		}
		if err := manager.setGitHubRepo(repository, branch); err != nil {
			fmt.Printf("Error setting GitHub repository: %v\n", err)
			os.Exit(1)
		}

	case "sync":
		if err := manager.syncToGitHub(dryRun); err != nil {
			fmt.Printf("Error syncing to GitHub: %v\n", err)
			os.Exit(1)
		}

	case "pull":
		if err := manager.pullFromGitHub(dryRun); err != nil {
			fmt.Printf("Error pulling from GitHub: %v\n", err)
			os.Exit(1)
		}

	case "debug":
		// Debug command to test package filtering
		fmt.Printf("Current system: %s\n", manager.System)
		fmt.Printf("Total packages in config: %d\n", len(manager.Config.Packages))
		fmt.Println("\nPackage analysis:")
		for pkgName, pkgConfig := range manager.Config.Packages {
			deployable := shouldDeployPackage(pkgConfig, manager.System)
			fmt.Printf("  %s: %+v -> deployable for %s: %t\n", pkgName, pkgConfig, manager.System, deployable)
		}

		// Test with different systems
		testSystems := []string{"arch", "linux", "macos", "ubuntu"}
		for _, testSys := range testSystems {
			packages := manager.getPackagesForSystem(testSys)
			fmt.Printf("\nPackages for %s: %d packages\n", testSys, len(packages))
			if len(packages) > 0 {
				fmt.Printf("  %s\n", strings.Join(packages, ", "))
			}
		}

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}
