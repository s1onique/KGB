.PHONY: build build-linux-arm64 test clean install-tools

# Binary name
BINARY := uvb76
VERSION := 0.1.0

# Go settings
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod

# Build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

# Default target
all: build

# Build for current platform
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY) .

# Build for Linux ARM64 (cross-compilation)
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-linux-arm64 .

# Test all packages
test:
	$(GOTEST) -v -race ./...

# Test with coverage
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
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
