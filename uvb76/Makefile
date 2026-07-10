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

# Race detector tests for state, probe, and server packages
# This target runs tests with -race to detect data races in concurrent code.
# Key targets tested: LatencyTracker (uvb76/state), ICMPClient (uvb76/probe), server handlers.
race:
	$(GOTEST) -race -timeout=60s ./state/...
	$(GOTEST) -race -timeout=60s ./probe/...
	$(GOTEST) -race -timeout=60s ./server/...

# Short race test (skips slow stress tests)
race-short:
	$(GOTEST) -race -timeout=30s -short ./state/...
	$(GOTEST) -race -timeout=30s -short ./probe/...
	$(GOTEST) -race -timeout=30s -short ./server/...

# === Latency Crash Lab ===
# Builds the Go lab command and verifier.
lab-build:
	$(GOBUILD) -o uvb76-latency-crash-lab ./cmd/uvb76-latency-crash-lab
	$(GOBUILD) -o uvb76-latency-crash-verify ./cmd/uvb76-latency-crash-verify

# Run the latency crash lab (requires Go toolchain and uvb76 binary built)
lab-run: build lab-build
	./uvb76-latency-crash-lab

# Verify an artifact directory (pass artifact dir as argument)
lab-verify: lab-build
	./uvb76-latency-crash-verify $(ARTIFACT_DIR)

# === UVB-76 TCP Diagnostic Telemetry Lab ===
# Builds and runs the Go lab that proves TCP telemetry is collected in diagnostic packets.
# Primary execution: `make lab-uvb76-tcp-diag-telemetry` or `make -C uvb76 lab-tcp-diag-telemetry`
# NOT part of make gate (requires hermetic diagnostic peer)

lab-tcp-diag-telemetry-build:
	cd cmd/uvb76-tcp-diag-telemetry-lab && go build -o ../../../uvb76-tcp-diag-telemetry-lab .

# Run the TCP diagnostic telemetry lab
lab-tcp-diag-telemetry: lab-tcp-diag-telemetry-build
	./uvb76-tcp-diag-telemetry-lab

# Run the verifier on an artifact directory
lab-tcp-diag-telemetry-verify: lab-tcp-diag-telemetry-build
	./uvb76-tcp-diag-telemetry-lab --verify --artifact-dir "$(ARTIFACT_DIR)"

# === Memory Lab Foundation ===
# Tests for pprof config and memory lab artifact/verdict model.
# These tests run the pure logic without spawning real processes.
test-memory-lab-collector:
	$(GOTEST) -v ./config/... -run "PProf"
	$(GOTEST) -v ./tools/memory-lab/...
	cd cmd/uvb76-memory-lab && $(GOTEST) -v ./...

# Build the memory lab collector command
memory-lab-build:
	cd cmd/uvb76-memory-lab && go build -o ../../uvb76-memory-lab .

# === UVB-76 Memory Leak pprof Lab (LAB01C) ===
# Localhost 10-minute lab for UVB-76/tovarisch pprof memory leak analysis.
# Uses fake-tovarisch status server, does not require netns.
# NOT part of make gate - manual execution only.

# Build the pprof memory lab command
lab-uvb76-memleak-pprof-build:
	cd cmd/uvb76-memleak-pprof-lab && go build -o ../../uvb76-memleak-pprof-lab .

# Build the pprof memory lab verifier
lab-uvb76-memleak-pprof-verify-build:
	cd cmd/uvb76-memleak-pprof-lab/verify && go build -o ../../../uvb76-memleak-pprof-verify .

# Run the localhost pprof memory leak lab
# Usage: make lab-uvb76-memleak-pprof ARTIFACT_DIR=artifacts/uvb76-memleak-pprof-lab
# Or with custom duration: make lab-uvb76-memleak-pprof DURATION=5m
lab-uvb76-memleak-pprof: lab-uvb76-memleak-pprof-build memory-lab-build
	@echo "=== UVB-76 Memory Leak pprof Lab ==="
	@if [ "$$(uname -s)" != "Linux" ]; then \
		echo "[SKIP] Memory labs require Linux (needs /proc)"; \
		exit 0; \
	fi
	@mkdir -p $(ARTIFACT_DIR)
	@./uvb76-memleak-pprof-lab \
		--duration $(DURATION) \
		--artifact-dir $(ARTIFACT_DIR)

# Verify pprof memory lab artifacts
# Usage: make verify-uvb76-memleak-pprof-artifacts ARTIFACT_DIR=artifacts/uvb76-memleak-pprof-lab
verify-uvb76-memleak-pprof-artifacts: lab-uvb76-memleak-pprof-verify-build
	@./uvb76-memleak-pprof-verify --artifact-dir $(ARTIFACT_DIR)

# Test all memory lab components (collector + pprof)
test-memory-lab: test-memory-lab-collector test-memory-lab-pprof
	@echo "=== Memory Lab Tests Complete ==="

# Tests for pprof lab components
test-memory-lab-pprof:
	cd cmd/uvb76-memleak-pprof-lab && $(GOTEST) -v ./...
	cd cmd/uvb76-memleak-pprof-lab/verify && $(GOTEST) -v ./...

# Default duration for lab (can override with DURATION= parameter)
DURATION ?= 10m

# Default artifact directory
ARTIFACT_DIR ?= artifacts/uvb76-memleak-pprof-lab
