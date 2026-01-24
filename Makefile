.PHONY: build build-plugins install clean test lint run

# Build variables
BINARY_NAME=reorg
BUILD_DIR=bin
CMD_DIR=cmd/reorg
PLUGIN_APPLE_NOTES=reorg-plugin-apple-notes
PLUGIN_OBSIDIAN=reorg-plugin-obsidian

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

# Build plugins
build-plugins:
	@echo "Building plugins..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(PLUGIN_APPLE_NOTES) ./cmd/reorg-plugin-apple-notes
	$(GOBUILD) -o $(BUILD_DIR)/$(PLUGIN_OBSIDIAN) ./cmd/reorg-plugin-obsidian
	@echo "Built $(BUILD_DIR)/$(PLUGIN_APPLE_NOTES)"
	@echo "Built $(BUILD_DIR)/$(PLUGIN_OBSIDIAN)"

# Build everything
build-all: build build-plugins

# Install to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOCMD) install ./$(CMD_DIR)
	@echo "Installed to $$(go env GOPATH)/bin/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "Cleaned"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && brew install golangci-lint)
	golangci-lint run ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Run the application
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Development: build and run
dev: build
	./$(BUILD_DIR)/$(BINARY_NAME) status

# Quick test: init and create sample data
quicktest: build
	@rm -rf /tmp/reorg-quicktest
	./$(BUILD_DIR)/$(BINARY_NAME) init --data-dir /tmp/reorg-quicktest --skip-wizard
	./$(BUILD_DIR)/$(BINARY_NAME) --data-dir /tmp/reorg-quicktest area create "Work"
	./$(BUILD_DIR)/$(BINARY_NAME) --data-dir /tmp/reorg-quicktest area create "Personal"
	./$(BUILD_DIR)/$(BINARY_NAME) --data-dir /tmp/reorg-quicktest project create "Test Project" --area work
	./$(BUILD_DIR)/$(BINARY_NAME) --data-dir /tmp/reorg-quicktest task create "First Task" --project test-project
	./$(BUILD_DIR)/$(BINARY_NAME) --data-dir /tmp/reorg-quicktest status

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  build-plugins - Build plugin binaries"
	@echo "  build-all     - Build application and plugins"
	@echo "  install       - Install to GOPATH/bin"
	@echo "  clean         - Clean build artifacts"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  lint          - Run golangci-lint"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  run           - Build and run"
	@echo "  dev           - Build and run status command"
	@echo "  quicktest     - Run quick functionality test"
	@echo "  help          - Show this help"
