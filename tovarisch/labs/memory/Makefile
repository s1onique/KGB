# tovarisch/labs/memory/Makefile — Memory Laboratory Build Targets
#
# Reference: kgb://factory/workflow

BINARY := memory-lab
CANARY := canary
ARTIFACTS ?= /tmp/tovarisch-memory-lab-artifacts
FACTORY := /home/kgb/Projects/KGB/.factory/bin

.PHONY: all build test vet fmt tidy clean \
	tovarisch-memory-lab \
	tovarisch-memory-lab-canary-growing \
	tovarisch-memory-lab-canary-bounded \
	tovarisch-memory-lab-canary-descriptor \
	tovarisch-memory-lab-canary-suite \
	tovarisch-memory-lab-verify-evidence \
	tovarisch-memory-lab-matrix \
	tovarisch-memory-lab-verify-matrix

all: build test

# Build the main binary
build:
	go build -o $(FACTORY)/$(BINARY) ./cmd/tovarisch-memory-lab

# Build canary
canary:
	go build -o $(FACTORY)/$(CANARY) ./cmd/canary

# Test all packages
test:
	go test -count=1 ./...

# Test with race detection
test-race:
	go test -race -count=1 ./...

# Vet all packages
vet:
	go vet ./...

# Format all packages
fmt:
	go fmt ./...

# Tidy dependencies
tidy:
	go mod tidy

# Clean
clean:
	rm -f $(FACTORY)/$(BINARY) $(FACTORY)/$(CANARY)

# Run canary-growing scenario
tovarisch-memory-lab-canary-growing:
	mkdir -p $(ARTIFACTS)
	$(FACTORY)/$(BINARY) run \
		--scenario canary-growing \
		--duration 60 \
		--artifacts-dir $(ARTIFACTS)

# Run canary-bounded scenario
tovarisch-memory-lab-canary-bounded:
	mkdir -p $(ARTIFACTS)
	$(FACTORY)/$(BINARY) run \
		--scenario canary-bounded \
		--duration 60 \
		--artifacts-dir $(ARTIFACTS)

# Run canary-descriptor scenario
tovarisch-memory-lab-canary-descriptor:
	mkdir -p $(ARTIFACTS)
	$(FACTORY)/$(BINARY) run \
		--scenario canary-descriptor \
		--duration 60 \
		--artifacts-dir $(ARTIFACTS)

# Run all canary scenarios
tovarisch-memory-lab-canary-suite: tovarisch-memory-lab-canary-growing tovarisch-memory-lab-canary-bounded tovarisch-memory-lab-canary-descriptor

# Verify evidence (requires RUN_ID and ARTIFACTS env vars or arguments)
tovarisch-memory-lab-verify-evidence:
	@if [ -z "$(RUN_ID)" ]; then \
		echo "Usage: make tovarisch-memory-lab-verify-evidence RUN_ID=<run-id> ARTIFACTS=<path>"; \
		exit 1; \
	fi
	$(FACTORY)/$(BINARY) verify \
		--run-id $(RUN_ID) \
		--artifacts-dir $(ARTIFACTS)

# === ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01 ===
# Matrix command: execute all three scenarios with frozen execution identity

tovarisch-memory-lab-matrix:
	mkdir -p $(ARTIFACTS)
	$(FACTORY)/$(BINARY) matrix \
		--duration 60 \
		--artifacts-dir $(ARTIFACTS)

# Verify matrix (requires MATRIX_DIR env var)
tovarisch-memory-lab-verify-matrix:
	@if [ -z "$(MATRIX_DIR)" ]; then \
		echo "Usage: make tovarisch-memory-lab-verify-matrix MATRIX_DIR=<path>"; \
		exit 1; \
	fi
	$(FACTORY)/$(BINARY) verify-matrix \
		--matrix-dir $(MATRIX_DIR)
