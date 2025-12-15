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
	return dm.deployPackageWithOptions(packageName, dryRun, false)
}

func (dm *DotfilesManager) deployPackageWithOptions(packageName string, dryRun bool, interactive bool) error {
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
			return dm.deployShellPackageWithOptions(packageDir, usr.HomeDir, dryRun, interactive)
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
	if err := dm.processPackageTemplatesWithOptions(packageDir, dryRun, interactive); err != nil {
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
	dm.deployAllWithOptions(packages, dryRun, false)
}

func (dm *DotfilesManager) deployAllWithOptions(packages []string, dryRun bool, interactive bool) {
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
		if err := dm.deployPackageWithOptions(pkg, dryRun, interactive); err != nil {
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
	return dm.processTemplateWithOptions(templatePath, outputPath, false)
}

func (dm *DotfilesManager) processTemplateWithOptions(templatePath, outputPath string, interactive bool) error {
	// Read template file
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	// Process template with current system
	processedContent := dm.processTemplateContent(string(templateContent))

	// Check if output file already exists
	if existingContent, err := os.ReadFile(outputPath); err == nil {
		// File exists - check if content differs
		if string(existingContent) == processedContent {
			// Content is identical, no need to overwrite
			return nil
		}

		// Content differs - handle based on mode
		if interactive {
			// Show diff and ask user
			shouldOverwrite, err := dm.promptForTemplateOverwrite(templatePath, outputPath, string(existingContent), processedContent)
			if err != nil {
				return fmt.Errorf("failed to prompt for overwrite: %w", err)
			}
			if !shouldOverwrite {
				fmt.Printf("TEMPLATE: Skipped %s (user declined overwrite)\n", outputPath)
				return nil
			}
		} else {
			// Non-interactive mode: always overwrite with warning
			fmt.Printf("TEMPLATE: Overwriting existing file %s (template takes precedence)\n", outputPath)
		}
	}

	// Track template overwrite for commit marking (before writing)
	if existingContent, err := os.ReadFile(outputPath); err == nil {
		if string(existingContent) != processedContent {
			dm.trackTemplateOverwrite(outputPath)
		}
	}

	// Write processed content to output file
	if err := os.WriteFile(outputPath, []byte(processedContent), 0644); err != nil {
		return fmt.Errorf("failed to write processed template: %w", err)
	}

	return nil
}

// Track template overwrites for commit marking
var templateOverwrites []string

func (dm *DotfilesManager) trackTemplateOverwrite(filePath string) {
	// Convert to relative path from dotfiles directory
	relPath, err := filepath.Rel(dm.DotfilesDir, filePath)
	if err != nil {
		relPath = filePath
	}
	templateOverwrites = append(templateOverwrites, relPath)
}

// TemplateMergeConflict represents a conflict between a template and its base config file
type TemplateMergeConflict struct {
	TemplatePath   string // Path to the .template file
	BasePath       string // Path to the generated base file
	LocalContent   string // Current local content of base file
	RemoteBase     string // Remote version of base file (if exists)
	RemoteTemplate string // Remote version of template file
}

// detectTemplateMergeConflicts scans for conflicts between templates and base files
func (dm *DotfilesManager) detectTemplateMergeConflicts() ([]TemplateMergeConflict, error) {
	var conflicts []TemplateMergeConflict

	// Walk through dotfiles directory looking for .template files
	err := filepath.Walk(dm.DotfilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		// Check if this is a template file
		if strings.HasSuffix(info.Name(), ".template") {
			basePath := strings.TrimSuffix(path, ".template")

			// Check if base file exists
			if _, err := os.Stat(basePath); err == nil {
				// Base file exists - check for conflicts
				localContent, err := os.ReadFile(basePath)
				if err != nil {
					return fmt.Errorf("failed to read base file %s: %w", basePath, err)
				}

				// Process template to see what it would generate
				templateContent, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read template %s: %w", path, err)
				}
				processedTemplate := dm.processTemplateContent(string(templateContent))

				// If content differs, we have a potential merge conflict
				if string(localContent) != processedTemplate {
					conflict := TemplateMergeConflict{
						TemplatePath: path,
						BasePath:     basePath,
						LocalContent: string(localContent),
					}

					// Try to get remote versions if in git repo
					relBasePath, _ := filepath.Rel(dm.DotfilesDir, basePath)
					relTemplatePath, _ := filepath.Rel(dm.DotfilesDir, path)

					// Get remote base file content
					cmd := exec.Command("git", "show", "origin/main:"+relBasePath)
					cmd.Dir = dm.DotfilesDir
					if output, err := cmd.Output(); err == nil {
						conflict.RemoteBase = string(output)
					}

					// Get remote template content
					cmd = exec.Command("git", "show", "origin/main:"+relTemplatePath)
					cmd.Dir = dm.DotfilesDir
					if output, err := cmd.Output(); err == nil {
						conflict.RemoteTemplate = string(output)
					}

					conflicts = append(conflicts, conflict)
				}
			}
		}

		return nil
	})

	return conflicts, err
}

// promptForTemplateMerge prompts the user to resolve a template merge conflict
func (dm *DotfilesManager) promptForTemplateMerge(conflict TemplateMergeConflict) (string, error) {
	fmt.Printf("\n⚠️  Template merge conflict detected\n")
	fmt.Printf("Template: %s\n", conflict.TemplatePath)
	fmt.Printf("Base file: %s\n", conflict.BasePath)
	fmt.Println()

	// Process current template to show what it would generate
	templateContent, _ := os.ReadFile(conflict.TemplatePath)
	newTemplateOutput := dm.processTemplateContent(string(templateContent))

	fmt.Println("Options:")
	fmt.Println("  1. Keep local changes in base file (ignore template)")
	fmt.Println("  2. Use template output (discard local changes)")
	fmt.Println("  3. Update template with base file changes (propagate changes)")
	fmt.Println("  4. Merge base changes into template interactively")
	fmt.Println("  5. Show diff between local and template output")
	fmt.Println("  6. Show three-way merge (local | remote base | template)")
	fmt.Println("  7. Manual edit base file (opens editor)")
	fmt.Println("  8. Skip this file")
	fmt.Printf("\nChoice [1-8]: ")

	var choice string
	fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		// Keep local changes in base file only
		return conflict.LocalContent, nil

	case "2":
		// Use template output
		return newTemplateOutput, nil

	case "3":
		// Update template with base file changes (smart merge)
		if err := dm.smartMergeIntoTemplate(conflict); err != nil {
			if err.Error() != "cancelled by user" {
				fmt.Printf("Error updating template: %v\n", err)
			}
			return dm.promptForTemplateMerge(conflict)
		}
		// After updating template, regenerate base file
		templateContent, _ := os.ReadFile(conflict.TemplatePath)
		return dm.processTemplateContent(string(templateContent)), nil

	case "4":
		// Merge base changes into template interactively
		if err := dm.mergeIntoTemplateInteractive(conflict); err != nil {
			fmt.Printf("Error merging into template: %v\n", err)
			return dm.promptForTemplateMerge(conflict)
		}
		// After updating template, regenerate base file
		templateContent, _ := os.ReadFile(conflict.TemplatePath)
		return dm.processTemplateContent(string(templateContent)), nil

	case "5":
		// Show diff
		dm.showDiff(conflict.LocalContent, newTemplateOutput, "Local changes", "Template output")
		return dm.promptForTemplateMerge(conflict) // Ask again

	case "6":
		// Show three-way merge
		if conflict.RemoteBase != "" && conflict.RemoteTemplate != "" {
			dm.showThreeWayDiff(conflict)
			fmt.Println("\nAfter reviewing the differences, choose how to proceed:")
			return dm.promptForTemplateMerge(conflict)
		} else {
			fmt.Println("Remote versions not available for three-way merge")
			return dm.promptForTemplateMerge(conflict)
		}

	case "7":
		// Manual edit base file
		return dm.manualEditMerge(conflict)

	case "8":
		// Skip
		return "", fmt.Errorf("skipped by user")

	default:
		fmt.Printf("Invalid choice '%s', please try again\n", choice)
		return dm.promptForTemplateMerge(conflict)
	}
}

// showDiff displays a side-by-side comparison of two contents
func (dm *DotfilesManager) showDiff(content1, content2, label1, label2 string) {
	fmt.Printf("\n=== DIFF: %s vs %s ===\n", label1, label2)

	lines1 := strings.Split(content1, "\n")
	lines2 := strings.Split(content2, "\n")

	fmt.Printf("--- %s\n", label1)
	fmt.Printf("+++ %s\n\n", label2)

	maxLines := len(lines1)
	if len(lines2) > maxLines {
		maxLines = len(lines2)
	}

	diffCount := 0
	for i := 0; i < maxLines && diffCount < 20; i++ {
		var line1, line2 string
		if i < len(lines1) {
			line1 = lines1[i]
		}
		if i < len(lines2) {
			line2 = lines2[i]
		}

		if line1 != line2 {
			if line1 != "" {
				fmt.Printf("-%s\n", line1)
			}
			if line2 != "" {
				fmt.Printf("+%s\n", line2)
			}
			diffCount++
		}
	}

	if diffCount >= 20 {
		fmt.Println("... (showing first 20 differences)")
	}
	fmt.Println()
}

// showThreeWayDiff shows local, remote base, and template output
func (dm *DotfilesManager) showThreeWayDiff(conflict TemplateMergeConflict) {
	fmt.Println("\n=== THREE-WAY COMPARISON ===")

	// Process remote template if available
	var remoteTemplateOutput string
	if conflict.RemoteTemplate != "" {
		remoteTemplateOutput = dm.processTemplateContent(conflict.RemoteTemplate)
	}

	// Process current local template
	templateContent, _ := os.ReadFile(conflict.TemplatePath)
	currentTemplateOutput := dm.processTemplateContent(string(templateContent))

	fmt.Println("\n[1] LOCAL CHANGES (current base file):")
	fmt.Println("---")
	dm.printPreview(conflict.LocalContent)

	if conflict.RemoteBase != "" {
		fmt.Println("\n[2] REMOTE BASE (from origin/main):")
		fmt.Println("---")
		dm.printPreview(conflict.RemoteBase)
	}

	if remoteTemplateOutput != "" {
		fmt.Println("\n[3] REMOTE TEMPLATE OUTPUT (what origin/main template would generate):")
		fmt.Println("---")
		dm.printPreview(remoteTemplateOutput)
	}

	fmt.Println("\n[4] CURRENT TEMPLATE OUTPUT (what local template would generate):")
	fmt.Println("---")
	dm.printPreview(currentTemplateOutput)
	fmt.Println()
}

// printPreview prints a preview of content (first 15 lines)
func (dm *DotfilesManager) printPreview(content string) {
	lines := strings.Split(content, "\n")
	maxLines := 15
	if len(lines) < maxLines {
		maxLines = len(lines)
	}

	for i := 0; i < maxLines; i++ {
		fmt.Printf("%3d: %s\n", i+1, lines[i])
	}

	if len(lines) > maxLines {
		fmt.Printf("... (%d more lines)\n", len(lines)-maxLines)
	}
}

// resolveTemplateConflicts resolves a list of template conflicts interactively
func (dm *DotfilesManager) resolveTemplateConflicts(conflicts []TemplateMergeConflict) error {
	fmt.Printf("\n=== RESOLVING TEMPLATE CONFLICTS ===\n")
	fmt.Printf("You have %d template conflict(s) to resolve.\n\n", len(conflicts))

	resolvedCount := 0
	skippedCount := 0

	for i, conflict := range conflicts {
		fmt.Printf("\n[%d/%d] Resolving conflict for: %s\n", i+1, len(conflicts), conflict.BasePath)

		resolvedContent, err := dm.promptForTemplateMerge(conflict)
		if err != nil {
			if err.Error() == "skipped by user" {
				fmt.Printf("Skipped %s\n", conflict.BasePath)
				skippedCount++
				continue
			}
			return err
		}

		// Write resolved content to base file
		if err := os.WriteFile(conflict.BasePath, []byte(resolvedContent), 0644); err != nil {
			return fmt.Errorf("failed to write resolved content to %s: %w", conflict.BasePath, err)
		}

		// Stage the resolved file
		relPath, _ := filepath.Rel(dm.DotfilesDir, conflict.BasePath)
		if err := dm.runGitCommand("add", relPath); err != nil {
			return fmt.Errorf("failed to stage resolved file: %w", err)
		}

		fmt.Printf("✓ Resolved and staged %s\n", conflict.BasePath)
		resolvedCount++
	}

	fmt.Printf("\n=== RESOLUTION SUMMARY ===\n")
	fmt.Printf("Resolved: %d\n", resolvedCount)
	fmt.Printf("Skipped: %d\n", skippedCount)

	if skippedCount > 0 {
		fmt.Printf("\nWarning: %d conflict(s) were skipped. You may need to resolve them manually.\n", skippedCount)
	}

	return nil
}

// LineDiff represents a difference between template output and base file
type LineDiff struct {
	Type            string // "added", "removed", "modified"
	BaseLineNum     int    // Line number in base file (0 if removed)
	TemplateLineNum int    // Line number in template output (0 if added)
	BaseContent     string // Content in base file
	TemplateContent string // Content in template output
}

// TemplateSection represents a section in the template file
type TemplateSection struct {
	Type         string   // "conditional", "common"
	System       string   // System name for conditional sections (empty for common)
	StartLine    int      // Starting line in template
	EndLine      int      // Ending line in template
	Content      []string // Lines in this section
	MatchedLines []string // Lines from base file that matched this section
}

// computeLineDiff compares base file with template output line by line
func computeLineDiff(baseContent, templateOutput string) []LineDiff {
	baseLines := strings.Split(baseContent, "\n")
	templateLines := strings.Split(templateOutput, "\n")

	var diffs []LineDiff

	// Simple line-by-line comparison for now
	// This can be enhanced with a proper diff algorithm (Myers, etc.)
	maxLen := len(baseLines)
	if len(templateLines) > maxLen {
		maxLen = len(templateLines)
	}

	for i := 0; i < maxLen; i++ {
		var baseLine, templateLine string
		if i < len(baseLines) {
			baseLine = baseLines[i]
		}
		if i < len(templateLines) {
			templateLine = templateLines[i]
		}

		if baseLine != templateLine {
			if baseLine == "" {
				// Line was removed from base
				diffs = append(diffs, LineDiff{
					Type:            "removed",
					TemplateLineNum: i + 1,
					TemplateContent: templateLine,
				})
			} else if templateLine == "" {
				// Line was added to base
				diffs = append(diffs, LineDiff{
					Type:        "added",
					BaseLineNum: i + 1,
					BaseContent: baseLine,
				})
			} else {
				// Line was modified
				diffs = append(diffs, LineDiff{
					Type:            "modified",
					BaseLineNum:     i + 1,
					TemplateLineNum: i + 1,
					BaseContent:     baseLine,
					TemplateContent: templateLine,
				})
			}
		}
	}

	return diffs
}

// parseTemplateSections breaks down a template into sections
func parseTemplateSections(templateContent string) []TemplateSection {
	lines := strings.Split(templateContent, "\n")
	var sections []TemplateSection
	var currentSection *TemplateSection

	// Start with a common section
	currentSection = &TemplateSection{
		Type:      "common",
		StartLine: 1,
		Content:   []string{},
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for conditional start
		if strings.HasPrefix(trimmed, "{{#if ") && strings.HasSuffix(trimmed, "}}") {
			// Save current section if it has content
			if len(currentSection.Content) > 0 {
				currentSection.EndLine = i
				sections = append(sections, *currentSection)
			}

			// Start new conditional section
			system := strings.TrimSpace(trimmed[6 : len(trimmed)-2])
			currentSection = &TemplateSection{
				Type:      "conditional",
				System:    system,
				StartLine: i + 1,
				Content:   []string{},
			}
			continue
		}

		// Check for conditional end
		if trimmed == "{{/if}}" {
			// Save current conditional section
			currentSection.EndLine = i
			sections = append(sections, *currentSection)

			// Start new common section
			currentSection = &TemplateSection{
				Type:      "common",
				StartLine: i + 1,
				Content:   []string{},
			}
			continue
		}

		// Add line to current section
		currentSection.Content = append(currentSection.Content, line)
	}

	// Save final section
	if len(currentSection.Content) > 0 {
		currentSection.EndLine = len(lines)
		sections = append(sections, *currentSection)
	}

	return sections
}

// smartMergeIntoTemplate intelligently merges base file changes into template
func (dm *DotfilesManager) smartMergeIntoTemplate(conflict TemplateMergeConflict) error {
	// Read template
	templateContent, err := os.ReadFile(conflict.TemplatePath)
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	// Check if template has conditionals
	hasConditionals := strings.Contains(string(templateContent), "{{#if")

	if !hasConditionals {
		// No conditionals - simple replacement
		fmt.Println("Template has no conditional blocks. Using direct replacement.")
		return dm.replaceTemplateSimple(conflict)
	}

	// Parse template into sections
	sections := parseTemplateSections(string(templateContent))

	// Process template to get what it would generate for current system
	templateOutput := dm.processTemplateContent(string(templateContent))

	// Compute differences
	diffs := computeLineDiff(conflict.LocalContent, templateOutput)

	if len(diffs) == 0 {
		fmt.Println("No differences detected between base file and template output.")
		return nil
	}

	fmt.Printf("\n=== SMART MERGE ANALYSIS ===\n")
	fmt.Printf("Found %d difference(s) between base file and template output\n", len(diffs))
	fmt.Printf("Template has %d section(s):\n", len(sections))
	for _, sec := range sections {
		if sec.Type == "conditional" {
			fmt.Printf("  - Conditional block for '%s' (%d lines)\n", sec.System, len(sec.Content))
		} else {
			fmt.Printf("  - Common section (%d lines)\n", len(sec.Content))
		}
	}
	fmt.Println()

	// Analyze where changes should go
	analysis := dm.analyzeChangePlacement(diffs, sections, templateOutput)

	// Show analysis and ask for confirmation
	fmt.Println("Change placement analysis:")
	for i, change := range analysis {
		fmt.Printf("%d. ", i+1)
		if change.Type == "added" {
			fmt.Printf("ADD: %s\n", truncate(change.BaseContent, 60))
		} else if change.Type == "modified" {
			fmt.Printf("MODIFY: %s -> %s\n",
				truncate(change.TemplateContent, 30),
				truncate(change.BaseContent, 30))
		} else {
			fmt.Printf("REMOVE: %s\n", truncate(change.TemplateContent, 60))
		}

		if change.RecommendedSection != nil {
			if change.RecommendedSection.Type == "conditional" {
				fmt.Printf("   → Suggested: Add to conditional block for '%s'\n", change.RecommendedSection.System)
			} else {
				fmt.Printf("   → Suggested: Add to common section\n")
			}
		} else {
			fmt.Printf("   → Suggested: Add to common section (default)\n")
		}
	}

	fmt.Println("\nOptions:")
	fmt.Println("  1. Auto-merge (apply all suggestions)")
	fmt.Println("  2. Manual edit (I'll help you place changes)")
	fmt.Println("  3. Cancel")
	fmt.Print("\nChoice [1-3]: ")

	var choice string
	fmt.Scanln(&choice)

	switch strings.TrimSpace(choice) {
	case "1":
		return dm.autoMergeTemplate(conflict, sections, analysis)
	case "2":
		return dm.editTemplateManually(conflict)
	case "3", "":
		return fmt.Errorf("cancelled by user")
	default:
		fmt.Println("Invalid choice")
		return dm.smartMergeIntoTemplate(conflict)
	}
}

// ChangePlacement represents where a change should go in the template
type ChangePlacement struct {
	LineDiff
	RecommendedSection *TemplateSection
	Confidence         string // "high", "medium", "low"
}

// analyzeChangePlacement determines where each change should go in the template
func (dm *DotfilesManager) analyzeChangePlacement(diffs []LineDiff, sections []TemplateSection, templateOutput string) []ChangePlacement {
	var placements []ChangePlacement

	templateLines := strings.Split(templateOutput, "\n")

	for _, diff := range diffs {
		placement := ChangePlacement{
			LineDiff:   diff,
			Confidence: "medium",
		}

		// Find which section this line is in (in the template output)
		if diff.TemplateLineNum > 0 && diff.TemplateLineNum <= len(templateLines) {
			// Find the corresponding section
			lineNum := diff.TemplateLineNum
			cumulativeLine := 0

			for i := range sections {
				sectionLen := len(sections[i].Content)
				if lineNum <= cumulativeLine+sectionLen {
					placement.RecommendedSection = &sections[i]
					placement.Confidence = "high"
					break
				}
				cumulativeLine += sectionLen
			}
		}

		// If we couldn't determine section (e.g., new line), default to common
		if placement.RecommendedSection == nil {
			// Find the last common section
			for i := len(sections) - 1; i >= 0; i-- {
				if sections[i].Type == "common" {
					placement.RecommendedSection = &sections[i]
					placement.Confidence = "low"
					break
				}
			}
		}

		placements = append(placements, placement)
	}

	return placements
}

// autoMergeTemplate automatically applies changes to the template
func (dm *DotfilesManager) autoMergeTemplate(conflict TemplateMergeConflict, sections []TemplateSection, placements []ChangePlacement) error {
	// Read template
	templateContent, err := os.ReadFile(conflict.TemplatePath)
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	templateLines := strings.Split(string(templateContent), "\n")

	// Apply changes section by section
	for _, placement := range placements {
		if placement.Type == "added" {
			// Find where to insert this line in the template
			if placement.RecommendedSection != nil {
				// Insert at end of recommended section
				insertPos := placement.RecommendedSection.EndLine

				// Handle conditional sections - insert before {{/if}}
				if placement.RecommendedSection.Type == "conditional" {
					insertPos-- // Insert before the {{/if}} line
				}

				// Insert the new line
				templateLines = insertLine(templateLines, insertPos, placement.BaseContent)

				// Update section end lines for subsequent sections
				for i := range sections {
					if sections[i].StartLine > insertPos {
						sections[i].StartLine++
						sections[i].EndLine++
					} else if sections[i].EndLine >= insertPos {
						sections[i].EndLine++
					}
				}
			}
		} else if placement.Type == "modified" {
			// Find and replace the line in the template
			// This requires mapping from template output line to template file line
			// For now, we'll search for the content
			for i, line := range templateLines {
				if strings.TrimSpace(line) == strings.TrimSpace(placement.TemplateContent) {
					templateLines[i] = placement.BaseContent
					break
				}
			}
		}
		// "removed" type: we don't remove lines from template, user can do that manually
	}

	// Write updated template
	newTemplateContent := strings.Join(templateLines, "\n")
	if err := os.WriteFile(conflict.TemplatePath, []byte(newTemplateContent), 0644); err != nil {
		return fmt.Errorf("failed to write template: %w", err)
	}

	// Stage the template file
	relPath, _ := filepath.Rel(dm.DotfilesDir, conflict.TemplatePath)
	if err := dm.runGitCommand("add", relPath); err != nil {
		fmt.Printf("Warning: Failed to stage template file: %v\n", err)
	}

	fmt.Printf("✓ Auto-merged changes into template: %s\n", conflict.TemplatePath)
	return nil
}

// insertLine inserts a line at the specified position
func insertLine(lines []string, pos int, content string) []string {
	if pos >= len(lines) {
		return append(lines, content)
	}

	lines = append(lines[:pos+1], lines[pos:]...)
	lines[pos] = content
	return lines
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// replaceTemplateSimple replaces template content with base file content
func (dm *DotfilesManager) replaceTemplateSimple(conflict TemplateMergeConflict) error {
	fmt.Println("\n⚠️  This will replace the template file content with your base file changes.")
	fmt.Print("Continue? [y/N]: ")

	var response string
	fmt.Scanln(&response)
	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		fmt.Println("Cancelled.")
		return fmt.Errorf("cancelled by user")
	}

	// Write base file content to template
	if err := os.WriteFile(conflict.TemplatePath, []byte(conflict.LocalContent), 0644); err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	// Stage the template file
	relPath, _ := filepath.Rel(dm.DotfilesDir, conflict.TemplatePath)
	if err := dm.runGitCommand("add", relPath); err != nil {
		fmt.Printf("Warning: Failed to stage template file: %v\n", err)
	}

	fmt.Printf("✓ Updated template: %s\n", conflict.TemplatePath)
	return nil
}

// mergeIntoTemplateInteractive intelligently merges base file changes into template
// while preserving existing conditional blocks
func (dm *DotfilesManager) mergeIntoTemplateInteractive(conflict TemplateMergeConflict) error {
	templateContent, err := os.ReadFile(conflict.TemplatePath)
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	fmt.Println("\n=== INTERACTIVE TEMPLATE UPDATE ===")
	fmt.Println("This will help you merge changes from the base file into the template")
	fmt.Println("while preserving conditional blocks.")
	fmt.Println()

	// Check if template has conditional blocks
	hasConditionals := strings.Contains(string(templateContent), "{{#if")

	if !hasConditionals {
		fmt.Println("Template has no conditional blocks. Using direct replacement.")
		return dm.replaceTemplateSimple(conflict)
	}

	fmt.Println("Template structure:")
	dm.analyzeTemplateStructure(string(templateContent))
	fmt.Println()

	fmt.Println("Options:")
	fmt.Println("  1. Edit template manually to merge changes")
	fmt.Println("  2. Show base file content (to help with manual edit)")
	fmt.Println("  3. Replace entire template (lose conditionals)")
	fmt.Println("  4. Cancel")
	fmt.Print("\nChoice [1-4]: ")

	var choice string
	fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		// Open template for editing
		return dm.editTemplateManually(conflict)

	case "2":
		// Show base content first, then ask again
		fmt.Println("\n=== BASE FILE CONTENT ===")
		dm.printPreview(conflict.LocalContent)
		fmt.Println("\nPress Enter to continue...")
		fmt.Scanln()
		return dm.mergeIntoTemplateInteractive(conflict)

	case "3":
		// Replace template completely
		return dm.replaceTemplateSimple(conflict)

	case "4", "":
		return fmt.Errorf("cancelled by user")

	default:
		fmt.Printf("Invalid choice '%s'\n", choice)
		return dm.mergeIntoTemplateInteractive(conflict)
	}
}

// analyzeTemplateStructure shows the structure of a template file
func (dm *DotfilesManager) analyzeTemplateStructure(templateContent string) {
	lines := strings.Split(templateContent, "\n")
	inConditional := false
	currentSystem := ""
	conditionalLines := 0
	commonLines := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "{{#if ") && strings.HasSuffix(trimmed, "}}") {
			currentSystem = strings.TrimSpace(trimmed[6 : len(trimmed)-2])
			inConditional = true
			conditionalLines = 0
			continue
		}

		if trimmed == "{{/if}}" {
			if inConditional {
				fmt.Printf("  - %d lines for system: %s\n", conditionalLines, currentSystem)
			}
			inConditional = false
			continue
		}

		if inConditional {
			conditionalLines++
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			commonLines++
		}
	}

	fmt.Printf("  - %d common lines (not in conditionals)\n", commonLines)
}

// editTemplateManually opens the template in an editor for manual updates
func (dm *DotfilesManager) editTemplateManually(conflict TemplateMergeConflict) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	// Create a helper file with base content for reference
	helperFile := conflict.TemplatePath + ".base-reference"
	helperContent := fmt.Sprintf(`# REFERENCE: Base file content (your changes)
# Copy relevant lines from here into the template file
# This file will be deleted after you close the editor
# Template file is: %s

%s`, conflict.TemplatePath, conflict.LocalContent)

	if err := os.WriteFile(helperFile, []byte(helperContent), 0644); err != nil {
		fmt.Printf("Warning: Could not create reference file: %v\n", err)
	} else {
		defer os.Remove(helperFile)
	}

	fmt.Printf("\nOpening template in %s...\n", editor)
	if helperFile != "" {
		fmt.Printf("Reference file (base content) opened in split: %s\n", helperFile)
		fmt.Println("\nTips:")
		fmt.Println("  - Look for {{#if system}} blocks in the template")
		fmt.Println("  - Merge your changes from the reference file")
		fmt.Println("  - Keep conditional blocks intact")
		fmt.Println("  - Common content goes outside conditionals")
		fmt.Println("\nPress Enter to open editor...")
		fmt.Scanln()
	}

	// Open both files in editor (vim supports this with -O for vertical split)
	var cmd *exec.Cmd
	if editor == "vim" || editor == "nvim" {
		cmd = exec.Command(editor, "-O", conflict.TemplatePath, helperFile)
	} else {
		// For other editors, just open the template
		cmd = exec.Command(editor, conflict.TemplatePath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	// Stage the updated template
	relPath, _ := filepath.Rel(dm.DotfilesDir, conflict.TemplatePath)
	if err := dm.runGitCommand("add", relPath); err != nil {
		fmt.Printf("Warning: Failed to stage template: %v\n", err)
	}

	fmt.Printf("✓ Template updated: %s\n", conflict.TemplatePath)
	return nil
}

// manualEditMerge opens an editor for manual conflict resolution
func (dm *DotfilesManager) manualEditMerge(conflict TemplateMergeConflict) (string, error) {
	// Create a temporary file with merge markers
	tempFile := conflict.BasePath + ".merge"

	// Get template output
	templateContent, _ := os.ReadFile(conflict.TemplatePath)
	templateOutput := dm.processTemplateContent(string(templateContent))

	// Create merge file with conflict markers
	mergeContent := fmt.Sprintf(`<<<<<<< LOCAL (your changes)
%s
=======
%s
>>>>>>> TEMPLATE (generated from %s)

# Instructions:
# 1. Resolve the conflict by editing the content above
# 2. Remove the conflict markers (<<<<<<, =======, >>>>>>>)
# 3. Save and close the editor
# 4. The resolved content will be used
`, conflict.LocalContent, templateOutput, filepath.Base(conflict.TemplatePath))

	if err := os.WriteFile(tempFile, []byte(mergeContent), 0644); err != nil {
		return "", fmt.Errorf("failed to create merge file: %w", err)
	}

	defer os.Remove(tempFile) // Clean up temp file

	// Open editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim" // Default to vim
	}

	fmt.Printf("Opening %s for manual merge...\n", editor)
	cmd := exec.Command(editor, tempFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	// Read the resolved content
	resolvedContent, err := os.ReadFile(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to read resolved content: %w", err)
	}

	// Check if conflict markers are still present
	if strings.Contains(string(resolvedContent), "<<<<<<<") ||
		strings.Contains(string(resolvedContent), "=======") ||
		strings.Contains(string(resolvedContent), ">>>>>>>") {
		fmt.Println("Warning: Conflict markers still present in file")
		fmt.Print("Continue anyway? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			return dm.promptForTemplateMerge(conflict)
		}
	}

	return string(resolvedContent), nil
}

func (dm *DotfilesManager) getTemplateOverwriteMessage() string {
	if len(templateOverwrites) == 0 {
		return ""
	}

	message := fmt.Sprintf("\n\n[TEMPLATE-OVERWRITES] The following files were regenerated from templates:\n")
	for _, file := range templateOverwrites {
		message += fmt.Sprintf("  - %s\n", file)
	}
	message += "\nTo revert template changes, use: git log --grep=\"TEMPLATE-OVERWRITES\" --oneline"

	// Clear the list after generating message
	templateOverwrites = nil

	return message
}

func (dm *DotfilesManager) showTemplateHistory() error {
	// Check if we're in a git repository
	gitDir := filepath.Join(dm.DotfilesDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		fmt.Println("Not a git repository. Template history is only available for git-managed dotfiles.")
		return nil
	}

	fmt.Println("Commits with template overwrites:")
	fmt.Println("=================================")

	// Search for commits with template overwrite markers
	cmd := exec.Command("git", "log", "--grep=TEMPLATE-OVERWRITES", "--oneline", "--reverse")
	cmd.Dir = dm.DotfilesDir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to search git history: %w", err)
	}

	if len(output) == 0 {
		fmt.Println("No template overwrites found in git history.")
		fmt.Println("\nTemplate overwrites are marked in commit messages when templates")
		fmt.Println("regenerate existing files during deployment.")
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i, line := range lines {
		fmt.Printf("%d. %s\n", i+1, line)
	}

	fmt.Printf("\nFound %d commits with template overwrites.\n", len(lines))
	fmt.Println("\nTo see details of a specific commit:")
	fmt.Println("  git show <commit-hash>")
	fmt.Println("\nTo revert a specific commit:")
	fmt.Println("  git revert <commit-hash>")
	fmt.Println("\nTo see what files were overwritten in a commit:")
	fmt.Println("  git show --name-only <commit-hash>")

	return nil
}

func (dm *DotfilesManager) promptForTemplateOverwrite(templatePath, outputPath, existingContent, newContent string) (bool, error) {
	fmt.Printf("\n⚠️  Template output file already exists: %s\n", outputPath)
	fmt.Printf("Template: %s\n", templatePath)
	fmt.Printf("System: %s\n\n", dm.System)

	// Show diff using a simple line-by-line comparison
	fmt.Println("Differences found:")
	fmt.Println("--- Existing file")
	fmt.Println("+++ Template output")

	existingLines := strings.Split(existingContent, "\n")
	newLines := strings.Split(newContent, "\n")

	maxLines := len(existingLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	diffCount := 0
	for i := 0; i < maxLines && diffCount < 10; i++ {
		var existingLine, newLine string
		if i < len(existingLines) {
			existingLine = existingLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if existingLine != newLine {
			if existingLine != "" {
				fmt.Printf("-%s\n", existingLine)
			}
			if newLine != "" {
				fmt.Printf("+%s\n", newLine)
			}
			diffCount++
		}
	}

	if diffCount >= 10 {
		fmt.Println("... (showing first 10 differences)")
	}

	fmt.Printf("\nOptions:\n")
	fmt.Printf("  y - Overwrite with template output (recommended)\n")
	fmt.Printf("  n - Keep existing file\n")
	fmt.Printf("  d - Show full diff\n")
	fmt.Printf("Choice [y/n/d]: ")

	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))

	switch response {
	case "d":
		// Show full diff and ask again
		fmt.Println("\n=== FULL DIFF ===")
		fmt.Println("--- Existing file")
		for i, line := range existingLines {
			fmt.Printf("%3d: %s\n", i+1, line)
		}
		fmt.Println("\n+++ Template output")
		for i, line := range newLines {
			fmt.Printf("%3d: %s\n", i+1, line)
		}
		fmt.Printf("\nOverwrite with template output? [y/n]: ")
		fmt.Scanln(&response)
		return strings.ToLower(strings.TrimSpace(response)) == "y", nil
	case "n":
		return false, nil
	case "y", "":
		return true, nil
	default:
		fmt.Printf("Invalid choice '%s', defaulting to 'y'\n", response)
		return true, nil
	}
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
	return dm.processPackageTemplatesWithOptions(packageDir, dryRun, false)
}

func (dm *DotfilesManager) processPackageTemplatesWithOptions(packageDir string, dryRun bool, interactive bool) error {
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

			// Process the template with interactive option
			if err := dm.processTemplateWithOptions(path, outputPath, interactive); err != nil {
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

				// Check for template-related conflicts
				templateConflicts, err := dm.detectTemplateMergeConflicts()
				if err != nil {
					fmt.Printf("Error detecting template conflicts: %v\n", err)
					fmt.Printf("Please resolve conflicts manually and run 'dotctl sync' again.\n")
					return fmt.Errorf("merge conflicts detected - manual resolution required")
				}

				if len(templateConflicts) > 0 {
					fmt.Printf("Found %d template-related conflicts that can be resolved interactively.\n", len(templateConflicts))
					fmt.Print("Resolve conflicts interactively? [Y/n]: ")
					var response string
					fmt.Scanln(&response)

					if strings.ToLower(strings.TrimSpace(response)) != "n" {
						// Resolve template conflicts interactively
						if err := dm.resolveTemplateConflicts(templateConflicts); err != nil {
							return fmt.Errorf("failed to resolve template conflicts: %w", err)
						}
					} else {
						fmt.Printf("Please resolve conflicts manually and run 'dotctl sync' again.\n")
						return fmt.Errorf("merge conflicts detected - manual resolution required")
					}
				} else {
					fmt.Printf("Please resolve conflicts manually and run 'dotctl sync' again.\n")
					fmt.Printf("Conflicted files can be found with: git status\n")
					return fmt.Errorf("merge conflicts detected - manual resolution required")
				}
			}
			return fmt.Errorf("failed to restore stashed changes: %w", err)
		}
		fmt.Printf("✓ Successfully restored local changes\n")
	}

	// Step 6.5: Check for template conflicts even without git merge conflicts
	// This handles cases where template and base file diverged locally
	fmt.Printf("Checking for template conflicts...\n")
	templateConflicts, err := dm.detectTemplateMergeConflicts()
	if err != nil {
		fmt.Printf("Warning: Error checking for template conflicts: %v\n", err)
	} else if len(templateConflicts) > 0 {
		fmt.Printf("Found %d template files with local modifications.\n", len(templateConflicts))
		fmt.Print("Review and resolve template conflicts? [Y/n]: ")
		var response string
		fmt.Scanln(&response)

		if strings.ToLower(strings.TrimSpace(response)) != "n" {
			if err := dm.resolveTemplateConflicts(templateConflicts); err != nil {
				return fmt.Errorf("failed to resolve template conflicts: %w", err)
			}
		}
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

	// Add template overwrite information if any templates were processed
	templateMsg := dm.getTemplateOverwriteMessage()
	if templateMsg != "" {
		commitMsg += templateMsg
	}

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
	return dm.deployShellPackageWithOptions(packageDir, homeDir, dryRun, false)
}

func (dm *DotfilesManager) deployShellPackageWithOptions(packageDir, homeDir string, dryRun bool, interactive bool) error {
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

			// Process template with interactive option
			if err := dm.processTemplateWithOptions(sourcePath, targetPath, interactive); err != nil {
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
  template-history        Show commits where template files were overwritten
  merge-check             Check for template merge conflicts without syncing
  merge-resolve           Interactively resolve template merge conflicts
  github-repo <owner/repo> [branch] Set GitHub repository for sync
  sync                    Sync dotfiles to GitHub repository (with merge detection)
  pull                    Pull dotfiles from GitHub repository

Options:
  --dotfiles-dir <path>   Path to dotfiles directory (default: ~/.dotfiles)
  --dry-run              Show what would be done without executing
  --interactive, -i      Prompt before overwriting template output files
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
  dotctl template-history          # Show commits with template overwrites
  dotctl merge-check               # Check for template conflicts
  dotctl merge-resolve             # Resolve template conflicts interactively
  dotctl github-repo user/dotfiles # Set GitHub repository
  dotctl sync                      # Push dotfiles to GitHub (auto-detects merges)
  dotctl pull                      # Pull dotfiles from GitHub
  dotctl --dry-run deploy          # Show what would be deployed
  dotctl --interactive deploy      # Deploy with prompts for template conflicts

Template Merging:
  When base config files are manually edited and template files are updated,
  dotctl can detect conflicts and help you merge changes interactively:
  
  - Use 'merge-check' to see if there are any conflicts
  - Use 'merge-resolve' to interactively resolve conflicts
  - During 'sync', conflicts are automatically detected and can be resolved
  
  The interactive merge provides options to:
  1. Keep local changes in base file only (don't update template)
  2. Use template output (discard local changes)
  3. Smart merge into template (analyzes changes, preserves conditionals, auto-merge)
  4. Merge base changes into template interactively (manual editing with assistance)
  5. View diffs between versions
  6. Perform three-way merge (local | remote | template)
  7. Manual edit base file with merge markers
  8. Skip individual files
  
  Option 3 intelligently:
  - Parses template sections (conditional blocks vs common sections)
  - Detects added/modified/removed lines
  - Determines where each change belongs
  - Can auto-merge or let you review placements
  - Preserves all {{#if system}} conditional blocks`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var dotfilesDir string
	var dryRun bool
	var interactive bool
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
		case arg == "--interactive" || arg == "-i":
			interactive = true
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
		manager.deployAllWithOptions(commandArgs, dryRun, interactive)

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

	case "template-history":
		if err := manager.showTemplateHistory(); err != nil {
			fmt.Printf("Error showing template history: %v\n", err)
			os.Exit(1)
		}

	case "merge-check":
		conflicts, err := manager.detectTemplateMergeConflicts()
		if err != nil {
			fmt.Printf("Error checking for template conflicts: %v\n", err)
			os.Exit(1)
		}

		if len(conflicts) == 0 {
			fmt.Println("✓ No template merge conflicts detected")
		} else {
			fmt.Printf("Found %d template merge conflict(s):\n\n", len(conflicts))
			for i, conflict := range conflicts {
				fmt.Printf("%d. %s\n", i+1, conflict.BasePath)
				fmt.Printf("   Template: %s\n", conflict.TemplatePath)

				// Show brief summary of differences
				templateContent, _ := os.ReadFile(conflict.TemplatePath)
				templateOutput := manager.processTemplateContent(string(templateContent))

				localLines := len(strings.Split(conflict.LocalContent, "\n"))
				templateLines := len(strings.Split(templateOutput, "\n"))
				fmt.Printf("   Local: %d lines, Template would generate: %d lines\n\n", localLines, templateLines)
			}

			fmt.Printf("Run 'dotctl merge-resolve' to interactively resolve these conflicts\n")
		}

	case "merge-resolve":
		conflicts, err := manager.detectTemplateMergeConflicts()
		if err != nil {
			fmt.Printf("Error detecting template conflicts: %v\n", err)
			os.Exit(1)
		}

		if len(conflicts) == 0 {
			fmt.Println("✓ No template merge conflicts to resolve")
		} else {
			if err := manager.resolveTemplateConflicts(conflicts); err != nil {
				fmt.Printf("Error resolving template conflicts: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("\n✓ Template conflicts resolved")
			fmt.Println("Run 'git status' to see staged changes")
			fmt.Println("Run 'dotctl sync' to commit and push changes")
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
