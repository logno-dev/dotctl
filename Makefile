# dotctl Makefile

BINARY_NAME=dotctl
BUILD_DIR=build
VERSION?=dev
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# Default target
.PHONY: all
all: build

# Build for current platform
.PHONY: build
build:
	@echo "Building $(BINARY_NAME) for current platform..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build for all major platforms
.PHONY: build-all
build-all: clean
	@echo "Building $(BINARY_NAME) for all platforms..."
	@mkdir -p $(BUILD_DIR)
	
	# Linux amd64
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	
	# Linux arm64
	@echo "Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	
	# macOS amd64
	@echo "Building for macOS amd64..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	
	# macOS arm64 (Apple Silicon)
	@echo "Building for macOS arm64..."
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	
	# Windows amd64
	@echo "Building for Windows amd64..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# Install to local system
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "✓ $(BINARY_NAME) installed successfully"

# Install to home directory (no sudo required)
.PHONY: install-user
install-user: build
	@echo "Installing $(BINARY_NAME) to ~/bin..."
	@mkdir -p ~/bin
	cp $(BUILD_DIR)/$(BINARY_NAME) ~/bin/
	chmod +x ~/bin/$(BINARY_NAME)
	@echo "✓ $(BINARY_NAME) installed to ~/bin"
	@echo "Make sure ~/bin is in your PATH"

# Uninstall from system
.PHONY: uninstall
uninstall:
	@echo "Removing $(BINARY_NAME) from /usr/local/bin..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "✓ $(BINARY_NAME) uninstalled"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

# Run tests
.PHONY: test
test:
	go test -v ./...

# Format code
.PHONY: fmt
fmt:
	go fmt ./...

# Lint code
.PHONY: lint
lint:
	golangci-lint run

# Run in development mode
.PHONY: run
run:
	go run . $(ARGS)

# Create release archive
.PHONY: release
release: build-all
	@echo "Creating release archives..."
	@mkdir -p $(BUILD_DIR)/releases
	
	# Linux
	tar -czf $(BUILD_DIR)/releases/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64
	tar -czf $(BUILD_DIR)/releases/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-arm64
	
	# macOS
	tar -czf $(BUILD_DIR)/releases/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64
	tar -czf $(BUILD_DIR)/releases/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-arm64
	
	# Windows
	cd $(BUILD_DIR) && zip releases/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe
	
	@echo "✓ Release archives created in $(BUILD_DIR)/releases/"

# Development helpers
.PHONY: dev-deploy
dev-deploy: build
	./$(BUILD_DIR)/$(BINARY_NAME) deploy --dry-run

.PHONY: dev-status
dev-status: build
	./$(BUILD_DIR)/$(BINARY_NAME) status

# Show help
.PHONY: help
help:
	@echo "dotctl Build System"
	@echo ""
	@echo "Targets:"
	@echo "  build         Build for current platform"
	@echo "  build-all     Build for all platforms"
	@echo "  install       Install to /usr/local/bin (requires sudo)"
	@echo "  install-user  Install to ~/bin (no sudo required)"
	@echo "  uninstall     Remove from /usr/local/bin"
	@echo "  clean         Remove build artifacts"
	@echo "  test          Run tests"
	@echo "  fmt           Format code"
	@echo "  lint          Lint code"
	@echo "  run           Run in development mode"
	@echo "  release       Create release archives"
	@echo "  help          Show this help"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION       Version string (default: dev)"
	@echo "  ARGS          Arguments for 'make run'"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make install"
	@echo "  make run ARGS='status'"
	@echo "  make release VERSION=v1.0.0"
