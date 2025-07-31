package main

import (
	"encoding/json"
	"fmt"
	// "io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type PackageConfig struct {
	Systems     []string `json:"systems,omitempty"`
	Description string   `json:"description,omitempty"`
}

type Config struct {
	Packages       map[string]interface{} `json:"packages"`
	GlobalExcludes []string               `json:"global_excludes"`
	StowOptions    []string               `json:"stow_options"`
}

type DotfilesManager struct {
	DotfilesDir string
	ConfigFile  string
	System      string
	Config      *Config
}

func NewDotfilesManager(dotfilesDir string) (*DotfilesManager, error) {
	if dotfilesDir == "" {
		usr, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("failed to get current user: %w", err)
		}
		dotfilesDir = filepath.Join(usr.HomeDir, ".dotfiles")
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
		fmt.Printf("Creating default config at %s\n", dm.ConfigFile)
		return defaultConfig, dm.saveConfig(defaultConfig)
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
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && entry.Name() != "__pycache__" {
			packages = append(packages, entry.Name())
		}
	}

	sort.Strings(packages)
	return packages, nil
}

func (dm *DotfilesManager) deployPackage(packageName string, dryRun bool) error {
	packageDir := filepath.Join(dm.DotfilesDir, packageName)
	if _, err := os.Stat(packageDir); os.IsNotExist(err) {
		return fmt.Errorf("package '%s' not found in %s", packageName, dm.DotfilesDir)
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
	args = append(args, packageName)

	if dryRun {
		args = append(args, "--no")
		fmt.Printf("DRY RUN: Would execute: %s\n", strings.Join(args, " "))
	} else {
		fmt.Printf("Deploying %s...\n", packageName)
	}

	cmd := exec.Command("stow", args[1:]...)
	cmd.Dir = dm.DotfilesDir
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

	args := []string{"--delete"}
	args = append(args, dm.Config.StowOptions...)
	args = append(args, packageName)

	if dryRun {
		args = append(args, "--no")
		fmt.Printf("DRY RUN: Would execute: stow %s\n", strings.Join(args, " "))
	} else {
		fmt.Printf("Undeploying %s...\n", packageName)
	}

	cmd := exec.Command("stow", args...)
	cmd.Dir = dm.DotfilesDir
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
  deploy [packages...]    Deploy packages (default: all for current system)
  undeploy [packages...]  Undeploy packages (default: all for current system)
  status                  Show current status
  add <package> [systems...] Add package to configuration
  remove <package>        Remove package from configuration

Options:
  --dotfiles-dir <path>   Path to dotfiles directory (default: ~/.dotfiles)
  --dry-run              Show what would be done without executing
  --help                 Show this help message

Examples:
  dotctl deploy                    # Deploy all packages for current system
  dotctl deploy vim tmux           # Deploy specific packages
  dotctl undeploy shell            # Undeploy specific package
  dotctl status                    # Show current status
  dotctl add vim linux macos      # Add vim package for Linux and macOS
  dotctl add shell all             # Add shell package for all systems
  dotctl remove vim                # Remove vim from configuration
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

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}
