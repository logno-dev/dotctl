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

	"gopkg.in/yaml.v3"
)

type PackageConfig struct {
	Systems     []string `yaml:"systems,omitempty" json:"systems,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Home        bool     `yaml:"home,omitempty" json:"home,omitempty"`
}

type GitHubConfig struct {
	Repository string `yaml:"repository,omitempty" json:"repository,omitempty"`
	Branch     string `yaml:"branch,omitempty" json:"branch,omitempty"`
}

type Config struct {
	Packages       map[string]interface{} `yaml:"packages" json:"packages"`
	GlobalExcludes []string               `yaml:"global_excludes" json:"global_excludes"`
	StowOptions    []string               `yaml:"stow_options" json:"stow_options"`
	GitHub         *GitHubConfig          `yaml:"github,omitempty" json:"github,omitempty"`
}

type DotfilesManager struct {
	DotfilesDir string
	ConfigFile  string
	System      string
	Config      *Config
}

func NewDotfilesManager(dotfilesDir string) (*DotfilesManager, error) {
	if dotfilesDir == "" {
		// First, check if we're already in a dotfiles directory (contains config file)
		if cwd, err := os.Getwd(); err == nil {
			// Check for YAML config first, then JSON for backwards compatibility
			yamlConfigPath := filepath.Join(cwd, "dotctl.yaml")
			jsonConfigPath := filepath.Join(cwd, "dotctl.json")

			if _, err := os.Stat(yamlConfigPath); err == nil {
				dotfilesDir = cwd
				fmt.Printf("Debug: Found dotctl.yaml in current directory: %s\n", yamlConfigPath)
			} else if _, err := os.Stat(jsonConfigPath); err == nil {
				dotfilesDir = cwd
				fmt.Printf("Debug: Found dotctl.json in current directory: %s\n", jsonConfigPath)
			}
		}

		// If not found in current directory, use default location
		if dotfilesDir == "" {
			usr, err := user.Current()
			if err != nil {
				return nil, fmt.Errorf("failed to get current user: %w", err)
			}
			dotfilesDir = filepath.Join(usr.HomeDir, ".dotfiles")
			fmt.Printf("Debug: Using default dotfiles directory: %s\n", dotfilesDir)
		}
	} else {
		fmt.Printf("Debug: Using specified dotfiles directory: %s\n", dotfilesDir)
	}

	// Determine config file path (prefer YAML, fallback to JSON)
	configFile := filepath.Join(dotfilesDir, "dotctl.yaml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		jsonConfigFile := filepath.Join(dotfilesDir, "dotctl.json")
		if _, err := os.Stat(jsonConfigFile); err == nil {
			configFile = jsonConfigFile
		}
	}

	manager := &DotfilesManager{
		DotfilesDir: dotfilesDir,
		ConfigFile:  configFile,
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
		StowOptions:    []string{}, // No longer used - kept for config compatibility
	}

	if _, err := os.Stat(dm.ConfigFile); os.IsNotExist(err) {
		// Don't create config automatically - let init command handle it
		return defaultConfig, nil
	}

	data, err := os.ReadFile(dm.ConfigFile)
	if err != nil {
		fmt.Printf("Warning: Could not read config file %s: %v\n", dm.ConfigFile, err)
		return defaultConfig, nil
	}

	var config Config

	// Determine if this is a YAML or JSON file based on extension
	isYAML := strings.HasSuffix(dm.ConfigFile, ".yaml") || strings.HasSuffix(dm.ConfigFile, ".yml")

	if isYAML {
		if err := yaml.Unmarshal(data, &config); err != nil {
			fmt.Printf("Error parsing YAML config: %v\n", err)
			fmt.Printf("Config file content: %s\n", string(data))
			return defaultConfig, nil
		}
	} else {
		// JSON parsing
		if err := json.Unmarshal(data, &config); err != nil {
			fmt.Printf("Error parsing JSON config: %v\n", err)
			fmt.Printf("Config file content: %s\n", string(data))
			return defaultConfig, nil
		}

		// If we successfully loaded a JSON config, migrate it to YAML
		if err := dm.migrateJSONToYAML(&config); err != nil {
			fmt.Printf("Warning: Failed to migrate JSON config to YAML: %v\n", err)
		} else {
			// Migration successful, reload the config from the new YAML file
			return dm.loadConfig()
		}
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

	// Always ensure the target directory in stow options matches the current user's home directory
	// This fixes issues when moving configs between different systems (macOS vs Linux)
	config.StowOptions = updateStowTargetOption(config.StowOptions, usr.HomeDir)

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

	// Always save as YAML (prefer .yaml extension)
	if strings.HasSuffix(dm.ConfigFile, ".json") {
		// Update config file path to use YAML extension
		dm.ConfigFile = strings.TrimSuffix(dm.ConfigFile, ".json") + ".yaml"
	}

	data, err := yaml.Marshal(dm.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Add a header comment to the YAML file
	header := `# dotctl configuration file
# This file defines your dotfiles packages and their target systems
# For more information, visit: https://github.com/your-repo/dotctl

`
	finalData := append([]byte(header), data...)

	return os.WriteFile(dm.ConfigFile, finalData, 0644)
}

// migrateJSONToYAML migrates an existing JSON config to YAML format
func (dm *DotfilesManager) migrateJSONToYAML(config *Config) error {
	jsonPath := dm.ConfigFile
	yamlPath := strings.TrimSuffix(jsonPath, ".json") + ".yaml"

	fmt.Printf("Migrating configuration from JSON to YAML...\n")

	// Update the config file path to YAML
	dm.ConfigFile = yamlPath

	// Save the config as YAML
	if err := dm.saveConfig(config); err != nil {
		return fmt.Errorf("failed to save YAML config: %w", err)
	}

	// Remove the old JSON file
	if err := os.Remove(jsonPath); err != nil {
		fmt.Printf("Warning: Could not remove old JSON config file: %v\n", err)
	} else {
		fmt.Printf("✓ Successfully migrated config from %s to %s\n",
			filepath.Base(jsonPath), filepath.Base(yamlPath))
	}

	return nil
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

func (dm *DotfilesManager) getPackageConfig(packageName string) *PackageConfig {
	packageConfigInterface, exists := dm.Config.Packages[packageName]
	if !exists {
		return nil
	}

	switch config := packageConfigInterface.(type) {
	case string:
		// Simple string configuration (e.g., "all", "linux")
		return &PackageConfig{
			Systems: []string{config},
		}
	case map[string]interface{}:
		// Complex configuration object
		packageConfig := &PackageConfig{}

		if systemsInterface, exists := config["systems"]; exists {
			if systemsSlice, ok := systemsInterface.([]interface{}); ok {
				for _, sys := range systemsSlice {
					if sysStr, ok := sys.(string); ok {
						packageConfig.Systems = append(packageConfig.Systems, sysStr)
					}
				}
			}
		}

		if descInterface, exists := config["description"]; exists {
			if desc, ok := descInterface.(string); ok {
				packageConfig.Description = desc
			}
		}

		if homeInterface, exists := config["home"]; exists {
			if home, ok := homeInterface.(bool); ok {
				packageConfig.Home = home
			}
		}

		return packageConfig
	default:
		return nil
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
			packages = append(packages, name)
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
	packageDir := filepath.Join(dm.DotfilesDir, packageName)

	if _, err := os.Stat(packageDir); os.IsNotExist(err) {
		return fmt.Errorf("package '%s' not found at %s", packageName, packageDir)
	}

	// Determine target directory and symlink path
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	var targetDir string
	var symlinkPath string

	// Check if package has home setting enabled
	packageConfig := dm.getPackageConfig(packageName)
	if packageConfig != nil && packageConfig.Home {
		// Home setting enabled - symlink to $HOME directory
		targetDir = usr.HomeDir
		symlinkPath = filepath.Join(targetDir, packageName)
	} else if isConfigPackage(packageName) {
		// Config packages go to ~/.config/PACKAGE_NAME
		targetDir = filepath.Join(usr.HomeDir, ".config")
		symlinkPath = filepath.Join(targetDir, packageName)
	} else {
		// Home packages: handle special cases
		if packageName == "shell" {
			// Shell package contents go directly to home directory
			return dm.deployShellPackage(packageDir, usr.HomeDir, dryRun)
		} else {
			// Other home packages (like .oh-my-zsh) go to ~/PACKAGE_NAME
			targetDir = usr.HomeDir
			symlinkPath = filepath.Join(targetDir, packageName)
		}
	}

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	if dryRun {
		fmt.Printf("DRY RUN: Would create symlink %s -> %s\n", symlinkPath, packageDir)
		return nil
	}

	fmt.Printf("Deploying %s...\n", packageName)

	// Check if symlink already exists
	if _, err := os.Lstat(symlinkPath); err == nil {
		// Remove existing symlink or file
		if err := os.Remove(symlinkPath); err != nil {
			return fmt.Errorf("failed to remove existing %s: %w", symlinkPath, err)
		}
	}

	// Check if package contains templates
	if err := dm.processPackageTemplates(packageDir, dryRun); err != nil {
		return fmt.Errorf("failed to process templates in %s: %w", packageName, err)
	}

	// Create the symlink
	relativePackageDir, err := filepath.Rel(targetDir, packageDir)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path: %w", err)
	}

	if err := os.Symlink(relativePackageDir, symlinkPath); err != nil {
		return fmt.Errorf("failed to create symlink %s -> %s: %w", symlinkPath, relativePackageDir, err)
	}

	fmt.Printf("✓ Successfully deployed %s\n", packageName)
	fmt.Printf("LINK: %s -> %s\n", symlinkPath, relativePackageDir)

	return nil
}

func (dm *DotfilesManager) undeployPackage(packageName string, dryRun bool) error {
	// Determine target directory and symlink path
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	var symlinkPath string

	// Check if package has home setting enabled
	packageConfig := dm.getPackageConfig(packageName)
	if packageConfig != nil && packageConfig.Home {
		// Home setting enabled - symlink is in $HOME directory
		symlinkPath = filepath.Join(usr.HomeDir, packageName)
	} else if isConfigPackage(packageName) {
		// Config packages are at ~/.config/PACKAGE_NAME
		symlinkPath = filepath.Join(usr.HomeDir, ".config", packageName)
	} else {
		if packageName == "shell" {
			// Shell package: remove individual files from home directory
			return dm.undeployShellPackage(filepath.Join(dm.DotfilesDir, packageName), usr.HomeDir, dryRun)
		} else {
			// Other home packages are at ~/PACKAGE_NAME
			symlinkPath = filepath.Join(usr.HomeDir, packageName)
		}
	}

	if dryRun {
		fmt.Printf("DRY RUN: Would remove symlink %s\n", symlinkPath)
		return nil
	}

	fmt.Printf("Undeploying %s...\n", packageName)

	// Check if symlink exists
	if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
		fmt.Printf("✓ %s is not deployed\n", packageName)
		return nil
	}

	// Remove the symlink
	if err := os.Remove(symlinkPath); err != nil {
		return fmt.Errorf("failed to remove symlink %s: %w", symlinkPath, err)
	}

	fmt.Printf("✓ Successfully undeployed %s\n", packageName)
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
	// GNU stow no longer required - using native symlinks
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

func (dm *DotfilesManager) adoptConfigDirectories(dryRun bool, args []string) error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	configDir := filepath.Join(usr.HomeDir, ".config")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		fmt.Println("No ~/.config directory found")
		return nil
	}

	// Parse arguments: first arg might be package name, rest are systems
	var targetPackages []string
	var systems []string

	if len(args) > 0 {
		// Check if first argument looks like a package name (not a known system)
		firstArg := args[0]
		if !isKnownSystem(firstArg) {
			// First argument is likely a package name
			targetPackages = []string{firstArg}
			systems = args[1:]
		} else {
			// All arguments are systems (adopt all packages)
			systems = args
		}
	}

	// Default systems if none provided
	if len(systems) == 0 {
		systems = []string{"all"}
	}

	// Get currently managed packages
	managedPackages := make(map[string]bool)
	for packageName := range dm.Config.Packages {
		if isConfigPackage(packageName) {
			managedPackages[packageName] = true
		}
	}

	var newPackages []string

	if len(targetPackages) > 0 {
		// Adopt specific packages
		for _, packageName := range targetPackages {
			// Skip if already managed
			if managedPackages[packageName] {
				fmt.Printf("Package '%s' is already managed\n", packageName)
				continue
			}

			// Check if it's already a symlink
			configPath := filepath.Join(configDir, packageName)
			if info, err := os.Lstat(configPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
				fmt.Printf("Package '%s' is already a symlink\n", packageName)
				continue
			}

			// Check if directory exists
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				fmt.Printf("Package '%s' not found in ~/.config\n", packageName)
				continue
			}

			newPackages = append(newPackages, packageName)
		}
	} else {
		// Adopt all unmanaged packages
		entries, err := os.ReadDir(configDir)
		if err != nil {
			return fmt.Errorf("failed to read ~/.config directory: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			packageName := entry.Name()

			// Skip if already managed
			if managedPackages[packageName] {
				continue
			}

			// Skip common non-package directories
			if shouldSkipDirectory(packageName) {
				continue
			}

			// Check if it's already a symlink (managed by something else)
			configPath := filepath.Join(configDir, packageName)
			if info, err := os.Lstat(configPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
				continue
			}

			newPackages = append(newPackages, packageName)
		}
	}

	if len(newPackages) == 0 {
		if len(targetPackages) > 0 {
			fmt.Println("No specified packages available to adopt")
		} else {
			fmt.Println("No new config directories found to adopt")
		}
		return nil
	}

	if len(targetPackages) > 0 {
		fmt.Printf("Adopting specific package(s): %s\n", strings.Join(newPackages, ", "))
	} else {
		fmt.Printf("Found %d new config directories to adopt:\n", len(newPackages))
		for _, pkg := range newPackages {
			fmt.Printf("  - %s\n", pkg)
		}
	}

	if dryRun {
		fmt.Printf("\nDRY RUN: Would adopt these packages for systems: %s\n", strings.Join(systems, ", "))
		return nil
	}

	fmt.Printf("\nAdopting packages for systems: %s\n", strings.Join(systems, ", "))

	// Adopt each package
	adoptedCount := 0
	for _, packageName := range newPackages {
		if err := dm.adoptSinglePackage(packageName, systems, configDir); err != nil {
			fmt.Printf("✗ Failed to adopt %s: %v\n", packageName, err)
		} else {
			adoptedCount++
			fmt.Printf("✓ Adopted %s\n", packageName)
		}
	}

	// Save updated configuration
	if err := dm.saveConfig(nil); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("\nSuccessfully adopted %d/%d packages\n", adoptedCount, len(newPackages))
	return nil
}
func (dm *DotfilesManager) adoptSinglePackage(packageName string, systems []string, configDir string) error {
	sourcePath := filepath.Join(configDir, packageName)
	targetPath := filepath.Join(dm.DotfilesDir, packageName)

	// Move the directory from ~/.config to ~/.dotfiles
	if err := os.Rename(sourcePath, targetPath); err != nil {
		return fmt.Errorf("failed to move %s to dotfiles: %w", packageName, err)
	}

	// Create symlink back to ~/.config
	relativeTargetPath, err := filepath.Rel(configDir, targetPath)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path: %w", err)
	}

	if err := os.Symlink(relativeTargetPath, sourcePath); err != nil {
		// Try to move back if symlink fails
		os.Rename(targetPath, sourcePath)
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Add to configuration
	if len(systems) == 1 && isSimpleSystem(systems[0]) {
		dm.Config.Packages[packageName] = systems[0]
	} else {
		dm.Config.Packages[packageName] = map[string]interface{}{
			"systems": systems,
		}
	}

	return nil
}

func shouldSkipDirectory(name string) bool {
	// Skip common directories that shouldn't be managed
	skipDirs := []string{
		"pulse", "systemd", "dconf", "gconf", "ibus", "fontconfig",
		"gtk-2.0", "gtk-3.0", "gtk-4.0", "qt5ct", "qt6ct", "Trolltech.conf",
		"mimeapps.list", "user-dirs.dirs", "user-dirs.locale",
	}

	for _, skip := range skipDirs {
		if name == skip {
			return true
		}
	}

	return false
}

func isKnownSystem(name string) bool {
	knownSystems := []string{"all", "linux", "macos", "arch", "ubuntu", "debian", "fedora", "windows"}
	for _, system := range knownSystems {
		if name == system {
			return true
		}
	}
	return false
}

func (dm *DotfilesManager) processTemplate(templatePath, outputPath string) error {
	// Read template file
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	// Process template with current system
	processedContent := dm.processTemplateContent(string(templateContent))

	// Write processed content to output file
	if err := os.WriteFile(outputPath, []byte(processedContent), 0644); err != nil {
		return fmt.Errorf("failed to write processed template: %w", err)
	}

	return nil
}

func (dm *DotfilesManager) processTemplateContent(content string) string {
	// Simple template processing for {{#if system}} blocks
	lines := strings.Split(content, "\n")
	var result []string
	var skipBlock bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for {{#if system}} blocks
		if strings.HasPrefix(trimmed, "{{#if ") && strings.HasSuffix(trimmed, "}}") {
			// Extract condition
			condition := strings.TrimSpace(trimmed[6 : len(trimmed)-2])

			// Check if condition matches current system
			skipBlock = !dm.matchesCondition(condition)
			continue
		}

		// Check for {{/if}} end blocks
		if trimmed == "{{/if}}" {
			skipBlock = false
			continue
		}
		// Add line if not in a skipped block
		if !skipBlock {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

func (dm *DotfilesManager) matchesCondition(condition string) bool {
	switch condition {
	case "macos":
		return dm.System == "macos"
	case "linux":
		return dm.System == "arch" || dm.System == "ubuntu" || dm.System == "debian" || dm.System == "fedora" || dm.System == "linux"
	case "arch":
		return dm.System == "arch"
	case "ubuntu":
		return dm.System == "ubuntu"
	case "debian":
		return dm.System == "debian"
	case "fedora":
		return dm.System == "fedora"
	default:
		return dm.System == condition
	}
}

func (dm *DotfilesManager) processPackageTemplates(packageDir string, dryRun bool) error {
	// Walk through package directory and process any .template files
	return filepath.Walk(packageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if this is a template file
		if strings.HasSuffix(info.Name(), ".template") {
			outputPath := strings.TrimSuffix(path, ".template")

			if dryRun {
				fmt.Printf("DRY RUN: Would process template %s -> %s\n", path, outputPath)
				return nil
			}

			// Process the template
			if err := dm.processTemplate(path, outputPath); err != nil {
				return fmt.Errorf("failed to process template %s: %w", path, err)
			}

			fmt.Printf("TEMPLATE: %s -> %s\n", path, outputPath)
		}

		return nil
	})
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
		fmt.Printf("DRY RUN: Would fetch from upstream\n")
		fmt.Printf("DRY RUN: Would check for local changes\n")
		fmt.Printf("DRY RUN: Would stash local changes if needed\n")
		fmt.Printf("DRY RUN: Would pull upstream changes\n")
		fmt.Printf("DRY RUN: Would restore local changes and merge\n")
		fmt.Printf("DRY RUN: Would add all files to git\n")
		fmt.Printf("DRY RUN: Would commit changes\n")
		fmt.Printf("DRY RUN: Would push to %s:%s\n", dm.Config.GitHub.Repository, branch)
		return nil
	}

	fmt.Printf("Syncing with GitHub repository %s...\n", dm.Config.GitHub.Repository)

	// Step 1: Fetch upstream changes to check if we're behind
	fmt.Printf("Fetching upstream changes...\n")
	if err := dm.runGitCommand("fetch", "origin", branch); err != nil {
		return fmt.Errorf("failed to fetch from upstream: %w", err)
	}

	// Step 2: Check if we have local changes
	hasLocalChanges, err := dm.hasLocalChanges()
	if err != nil {
		return fmt.Errorf("failed to check for local changes: %w", err)
	}

	// Step 3: Check if we're behind upstream
	isBehind, err := dm.isBehindUpstream(branch)
	if err != nil {
		return fmt.Errorf("failed to check upstream status: %w", err)
	}

	var stashCreated bool

	// Step 4: If we have local changes and need to pull, stash them
	if hasLocalChanges && isBehind {
		fmt.Printf("Local changes detected, stashing before pull...\n")
		if err := dm.runGitCommand("stash", "push", "-m", "dotctl-sync-stash-"+getCurrentTimestamp()); err != nil {
			return fmt.Errorf("failed to stash local changes: %w", err)
		}
		stashCreated = true
	}

	// Step 5: Pull upstream changes if we're behind
	if isBehind {
		fmt.Printf("Pulling upstream changes...\n")
		if err := dm.runGitCommand("pull", "origin", branch); err != nil {
			// If pull failed and we stashed, try to restore
			if stashCreated {
				fmt.Printf("Pull failed, restoring stashed changes...\n")
				dm.runGitCommand("stash", "pop")
			}
			return fmt.Errorf("failed to pull from upstream: %w", err)
		}
		fmt.Printf("✓ Successfully pulled upstream changes\n")
	}

	// Step 6: If we stashed changes, restore them and handle conflicts
	if stashCreated {
		fmt.Printf("Restoring local changes...\n")
		if err := dm.runGitCommand("stash", "pop"); err != nil {
			// Check if it's a merge conflict
			if dm.hasMergeConflicts() {
				fmt.Printf("⚠️  Merge conflicts detected after restoring local changes.\n")
				fmt.Printf("Please resolve conflicts manually and run 'dotctl sync' again.\n")
				fmt.Printf("Conflicted files can be found with: git status\n")
				return fmt.Errorf("merge conflicts detected - manual resolution required")
			}
			return fmt.Errorf("failed to restore stashed changes: %w", err)
		}
		fmt.Printf("✓ Successfully restored local changes\n")
	}

	// Step 7: Add all files (including any resolved conflicts or new changes)
	if err := dm.runGitCommand("add", "."); err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}

	// Step 8: Check if there are changes to commit
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dm.DotfilesDir
	if err := cmd.Run(); err == nil {
		fmt.Println("✓ Repository is up to date, no changes to sync")
		return nil
	}

	// Step 9: Commit changes
	commitMsg := fmt.Sprintf("Update dotfiles - %s", getCurrentTimestamp())
	if err := dm.runGitCommand("commit", "-m", commitMsg); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Step 10: Push to GitHub
	if err := dm.runGitCommand("push", "origin", branch); err != nil {
		return fmt.Errorf("failed to push to GitHub: %w", err)
	}

	fmt.Printf("✓ Successfully synced with GitHub repository %s\n", dm.Config.GitHub.Repository)
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

// hasLocalChanges checks if there are uncommitted changes in the working directory
func (dm *DotfilesManager) hasLocalChanges() (bool, error) {
	// Check for staged changes
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dm.DotfilesDir
	if err := cmd.Run(); err != nil {
		// Exit code 1 means there are staged changes
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("failed to check staged changes: %w", err)
	}

	// Check for unstaged changes
	cmd = exec.Command("git", "diff", "--quiet")
	cmd.Dir = dm.DotfilesDir
	if err := cmd.Run(); err != nil {
		// Exit code 1 means there are unstaged changes
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("failed to check unstaged changes: %w", err)
	}

	// Check for untracked files
	cmd = exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = dm.DotfilesDir
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check untracked files: %w", err)
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

// isBehindUpstream checks if the local branch is behind the upstream branch
func (dm *DotfilesManager) isBehindUpstream(branch string) (bool, error) {
	// Get the commit hash of the local branch
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dm.DotfilesDir
	localOutput, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get local commit hash: %w", err)
	}
	localHash := strings.TrimSpace(string(localOutput))

	// Get the commit hash of the upstream branch
	cmd = exec.Command("git", "rev-parse", "origin/"+branch)
	cmd.Dir = dm.DotfilesDir
	upstreamOutput, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get upstream commit hash: %w", err)
	}
	upstreamHash := strings.TrimSpace(string(upstreamOutput))

	return localHash != upstreamHash, nil
}

// hasMergeConflicts checks if there are merge conflicts in the working directory
func (dm *DotfilesManager) hasMergeConflicts() bool {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = dm.DotfilesDir
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
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

func isConfigPackage(packageName string) bool {
	// Packages that go to home directory (~/)
	// - Packages starting with "." (like .oh-my-zsh, .zshrc)
	// - shell package (contains shell configs like .zshrc, .bashrc)
	if strings.HasPrefix(packageName, ".") || packageName == "shell" {
		return false // Goes to ~/
	}

	// Everything else goes to ~/.config/
	return true
}

func (dm *DotfilesManager) deployShellPackage(packageDir, homeDir string, dryRun bool) error {
	// For shell package, symlink each file directly to home directory
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return fmt.Errorf("failed to read shell package directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories in shell package
		}

		fileName := entry.Name()
		sourcePath := filepath.Join(packageDir, fileName)

		// Check if this is a template file
		if strings.HasSuffix(fileName, ".template") {
			// Process template
			outputFileName := strings.TrimSuffix(fileName, ".template")
			targetPath := filepath.Join(homeDir, outputFileName)

			if dryRun {
				fmt.Printf("DRY RUN: Would process template %s -> %s\n", sourcePath, targetPath)
				continue
			}

			// Remove existing file
			if _, err := os.Lstat(targetPath); err == nil {
				if err := os.Remove(targetPath); err != nil {
					return fmt.Errorf("failed to remove existing %s: %w", targetPath, err)
				}
			}

			// Process template
			if err := dm.processTemplate(sourcePath, targetPath); err != nil {
				return fmt.Errorf("failed to process template %s: %w", fileName, err)
			}

			fmt.Printf("TEMPLATE: %s -> %s\n", sourcePath, targetPath)
		} else {
			// Regular file - create symlink
			targetPath := filepath.Join(homeDir, fileName)

			if dryRun {
				fmt.Printf("DRY RUN: Would create symlink %s -> %s\n", targetPath, sourcePath)
				continue
			}

			// Check if target already exists
			if _, err := os.Lstat(targetPath); err == nil {
				// Remove existing symlink or file
				if err := os.Remove(targetPath); err != nil {
					return fmt.Errorf("failed to remove existing %s: %w", targetPath, err)
				}
			}

			// Create relative path for symlink
			relativeSourcePath, err := filepath.Rel(homeDir, sourcePath)
			if err != nil {
				return fmt.Errorf("failed to calculate relative path: %w", err)
			}

			// Create the symlink
			if err := os.Symlink(relativeSourcePath, targetPath); err != nil {
				return fmt.Errorf("failed to create symlink %s -> %s: %w", targetPath, relativeSourcePath, err)
			}

			fmt.Printf("LINK: %s -> %s\n", targetPath, relativeSourcePath)
		}
	}

	return nil
}

func (dm *DotfilesManager) undeployShellPackage(packageDir, homeDir string, dryRun bool) error {
	// For shell package, remove each symlinked file from home directory
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return fmt.Errorf("failed to read shell package directory: %w", err)
	}

	for _, entry := range entries {
		targetPath := filepath.Join(homeDir, entry.Name())

		if dryRun {
			fmt.Printf("DRY RUN: Would remove symlink %s\n", targetPath)
			continue
		}

		// Check if symlink exists
		if _, err := os.Lstat(targetPath); os.IsNotExist(err) {
			continue // Skip if doesn't exist
		}

		// Remove the symlink
		if err := os.Remove(targetPath); err != nil {
			return fmt.Errorf("failed to remove symlink %s: %w", targetPath, err)
		}

		fmt.Printf("UNLINK: %s\n", targetPath)
	}

	return nil
}

func boolToCheckmark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func updateStowTargetOption(stowOptions []string, homeDir string) []string {
	var updatedOptions []string
	targetFound := false

	for _, option := range stowOptions {
		if strings.HasPrefix(option, "--target=") {
			// Replace with current home directory
			updatedOptions = append(updatedOptions, "--target="+homeDir)
			targetFound = true
		} else {
			updatedOptions = append(updatedOptions, option)
		}
	}

	// If no target option was found, add it
	if !targetFound {
		updatedOptions = append(updatedOptions, "--target="+homeDir)
	}

	return updatedOptions
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
  adopt [package] [systems...]  Adopt config directories from ~/.config (default: all packages, all systems)
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
  dotctl adopt                     # Adopt all new config directories for all systems
  dotctl adopt arch linux          # Adopt all new config directories for specific systems
  dotctl adopt new-app             # Adopt specific package for all systems
  dotctl adopt new-app arch        # Adopt specific package for specific systems
  dotctl --dry-run adopt           # Preview what would be adopted
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

	case "adopt":
		if err := manager.adoptConfigDirectories(dryRun, commandArgs); err != nil {
			fmt.Printf("Error adopting config directories: %v\n", err)
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
		// Debug command to test package filtering and filesystem operations
		fmt.Printf("=== FILESYSTEM DEBUG ===\n")
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error getting current directory: %v\n", err)
		} else {
			fmt.Printf("Current working directory: %s\n", cwd)
		}

		fmt.Printf("Dotfiles directory: %s\n", manager.DotfilesDir)
		fmt.Printf("Config file path: %s\n", manager.ConfigFile)

		// Check if dotfiles directory exists
		if stat, err := os.Stat(manager.DotfilesDir); err != nil {
			fmt.Printf("Dotfiles directory error: %v\n", err)
		} else {
			fmt.Printf("Dotfiles directory exists: %t, is dir: %t\n", true, stat.IsDir())
		}

		// Check if config file exists
		if stat, err := os.Stat(manager.ConfigFile); err != nil {
			fmt.Printf("Config file error: %v\n", err)
		} else {
			fmt.Printf("Config file exists: %t, size: %d bytes\n", true, stat.Size())
		}

		// Try to read config file directly
		if data, err := os.ReadFile(manager.ConfigFile); err != nil {
			fmt.Printf("Error reading config file: %v\n", err)
		} else {
			fmt.Printf("Config file content length: %d bytes\n", len(data))
			if len(data) > 0 {
				previewLen := 200
				if len(data) < previewLen {
					previewLen = len(data)
				}
				fmt.Printf("Config file preview (first %d chars): %s\n", previewLen, string(data[:previewLen]))
			}
		}

		fmt.Printf("\n=== SYSTEM DETECTION ===\n")
		fmt.Printf("Runtime GOOS: %s\n", runtime.GOOS)
		fmt.Printf("Detected system: %s\n", manager.System)

		// Check /etc/os-release on Linux systems
		if runtime.GOOS == "linux" {
			if data, err := os.ReadFile("/etc/os-release"); err != nil {
				fmt.Printf("Error reading /etc/os-release: %v\n", err)
			} else {
				fmt.Printf("/etc/os-release content:\n%s\n", string(data))
			}
		}

		fmt.Printf("\n=== PACKAGE ANALYSIS ===\n")
		fmt.Printf("Total packages in config: %d\n", len(manager.Config.Packages))

		if len(manager.Config.Packages) > 0 {
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
		} else {
			fmt.Println("No packages found in configuration - this suggests config loading failed")
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}
