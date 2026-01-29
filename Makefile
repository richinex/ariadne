# Davingo Makefile
# Run 'make help' to see available targets

.PHONY: all build test lint lint-nil clean install-tools help

# Default target
all: build

# Build the Go binary
build:
	@echo "Building davingo..."
	@go build -o davingo ./cmd/davingo

# Build with race detector (for development)
build-race:
	@echo "Building davingo with race detector..."
	@go build -race -o davingo ./cmd/davingo

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	@go test -race ./...

# Run tests with goroutine leak detection
# Uses go.uber.org/goleak in test files
test-leak:
	@echo "Running tests with goroutine leak detection..."
	@go test -v ./storage/... ./tools/... ./internal/dsa/...
	@echo "Leak detection complete!"

# Run all quality checks (race + leak)
test-quality: test-race test-leak
	@echo "All quality checks passed!"

# Lint the code
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	elif [ -f ~/go/bin/golangci-lint ]; then \
		~/go/bin/golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run 'make install-tools' first."; \
		go vet ./...; \
	fi

# Nil pointer detection with NilAway
lint-nil:
	@echo "Running NilAway nil pointer analysis..."
	@if command -v nilaway >/dev/null 2>&1; then \
		nilaway ./...; \
	elif [ -f ~/go/bin/nilaway ]; then \
		~/go/bin/nilaway ./...; \
	else \
		echo "NilAway not installed. Run 'make install-tools' first."; \
		exit 1; \
	fi

# Nil pointer detection on critical packages only
lint-nil-critical:
	@echo "Running NilAway on critical packages..."
	@if command -v nilaway >/dev/null 2>&1; then \
		nilaway ./storage/... ./tools/... ./cli/... ./internal/dsa/...; \
	elif [ -f ~/go/bin/nilaway ]; then \
		~/go/bin/nilaway ./storage/... ./tools/... ./cli/... ./internal/dsa/...; \
	else \
		echo "NilAway not installed. Run 'make install-tools' first."; \
		exit 1; \
	fi

# Static analysis with staticcheck
lint-static:
	@echo "Running staticcheck..."
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	elif [ -f ~/go/bin/staticcheck ]; then \
		~/go/bin/staticcheck ./...; \
	else \
		echo "staticcheck not installed. Run 'make install-tools' first."; \
		exit 1; \
	fi

# Run all linters
lint-all: lint lint-nil lint-static

# Format code
fmt:
	@echo "Formatting Go code..."
	@go fmt ./...

# Install development tools
install-tools:
	@echo "Installing development tools..."
	@echo "Installing NilAway..."
	@go install go.uber.org/nilaway/cmd/nilaway@latest
	@echo "Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Installing staticcheck..."
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "Installing goleak (for leak testing)..."
	@go get -u go.uber.org/goleak
	@echo "Tools installed to ~/go/bin/"
	@echo "Make sure ~/go/bin is in your PATH"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f davingo
	@rm -rf build
	@rm -f coverage.out coverage.html
	@rm -rf .davingo

# Clean including database
clean-all: clean
	@echo "Deep cleaning (including database)..."
	@rm -rf .davingo

# Generate (if you have go generate directives)
generate:
	@echo "Running go generate..."
	@go generate ./...

# Check for outdated dependencies
deps-check:
	@echo "Checking for outdated dependencies..."
	@go list -u -m all

# Update dependencies
deps-update:
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy

# Tidy dependencies
deps-tidy:
	@echo "Tidying dependencies..."
	@go mod tidy

# Release build (optimized binary)
release:
	@echo "Building release binary..."
	@mkdir -p build
	@go build -ldflags "-s -w" -trimpath -o build/davingo ./cmd/davingo
	@echo "Release binary: build/davingo"

# Cross-compile for multiple platforms
cross-build:
	@echo "Cross-compiling..."
	@mkdir -p build
	@GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o build/davingo-linux-amd64 ./cmd/davingo
	@GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o build/davingo-linux-arm64 ./cmd/davingo
	@GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o build/davingo-darwin-amd64 ./cmd/davingo
	@GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o build/davingo-darwin-arm64 ./cmd/davingo
	@echo "Cross-compilation complete"

# Development setup
dev-setup: install-tools
	@echo "Development environment setup complete!"

# CI target (for continuous integration)
ci: lint test build
	@echo "CI checks passed!"

# Quick check before commit
pre-commit: fmt lint-nil-critical test-race
	@echo "Pre-commit checks passed!"

# Run examples
example-resultstore: build
	@echo "Running ResultStore demo..."
	@go run ./examples/resultstore_demo/

# Help
help:
	@echo "Davingo Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build targets:"
	@echo "  build          Build the Go binary"
	@echo "  build-race     Build with race detector"
	@echo "  release        Build optimized release binary"
	@echo "  cross-build    Cross-compile for Linux, macOS"
	@echo ""
	@echo "Test targets:"
	@echo "  test           Run tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  test-race      Run tests with race detector"
	@echo "  test-leak      Run tests with goroutine leak detection"
	@echo "  test-quality   Run all quality checks (race + leak)"
	@echo ""
	@echo "Lint targets:"
	@echo "  lint           Run golangci-lint (or go vet)"
	@echo "  lint-nil       Run NilAway nil pointer analysis (all packages)"
	@echo "  lint-nil-critical  Run NilAway on critical packages"
	@echo "  lint-static    Run staticcheck"
	@echo "  lint-all       Run all linters"
	@echo "  fmt            Format Go code"
	@echo ""
	@echo "Dependency targets:"
	@echo "  deps-check     Check for outdated dependencies"
	@echo "  deps-update    Update all dependencies"
	@echo "  deps-tidy      Tidy go.mod"
	@echo ""
	@echo "Setup targets:"
	@echo "  install-tools  Install dev tools (nilaway, golangci-lint, staticcheck)"
	@echo "  dev-setup      Full development setup"
	@echo ""
	@echo "Clean targets:"
	@echo "  clean          Remove build artifacts"
	@echo "  clean-all      Deep clean (includes .davingo database)"
	@echo ""
	@echo "Other targets:"
	@echo "  ci             Run CI checks (lint, test, build)"
	@echo "  pre-commit     Quick checks before commit"
	@echo "  example-resultstore  Run ResultStore demo"
	@echo "  help           Show this help"
