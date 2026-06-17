.PHONY: build build-linux-arm64 test clean install-tools web-build web-dev web-clean verify

# Binary name
BINARY := uvb76
VERSION := 0.1.0

# Go settings
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod

# Web settings
WEB_DIR := web
WEB_DIST := $(WEB_DIR)/dist

# Build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

# Default target
all: web-build build

# Build for current platform
build: web-build
	$(GOBUILD) $(LDFLAGS) -o $(BINARY) .

# Build for Linux ARM64 (cross-compilation)
build-linux-arm64: web-build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-linux-arm64 .

# Web UI build
web-build:
	cd $(WEB_DIR) && npm ci
	cd $(WEB_DIR) && npm run build

# Web UI dev server (watch mode)
web-dev:
	cd $(WEB_DIR) && npm run dev

# Clean web build artifacts
web-clean:
	rm -rf $(WEB_DIST)

# Test all packages
test: web-build
	$(GOTEST) -v -race ./...

# Test with coverage
test-coverage: web-build
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean: web-clean
	rm -f $(BINARY) $(BINARY)-linux-arm64
	rm -f coverage.out coverage.html

# Download dependencies
tidy:
	$(GOMOD) tidy
	$(GOMOD) download

# Verify dependencies
verify:
	$(GOMOD) verify

# Run the binary (requires TLS cert/key)
run: build
	./$(BINARY) -config uvb76.json

# Build and run in dev mode (no TLS required)
dev: build
	./$(BINARY) -dev -config uvb76.json

# Verify embedded web UI build
verify-embed: web-build
	@bash scripts/verify_web_embed.sh

# Format code
fmt:
	$(GOCMD) fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run ./...

# Vet code
vet:
	$(GOCMD) vet ./...

# Generate password hash for config
genhash:
	@echo "Enter password and press Enter:"
	@read -s password; echo "$$password" | $(GOCMD) run ./tools/genhash
