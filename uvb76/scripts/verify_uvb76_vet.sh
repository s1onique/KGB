#!/usr/bin/env bash
# ShellRole: thin launcher - wraps Go vet binary
# ShellJustification: minimal shell glue; delegates to typed Go analyzer
# MigrationPlan: not applicable - thin wrapper with no risky operations

# verify_uvb76_vet.sh runs the custom UVB-76 static analyzers.
#
# This script builds the vet tool and runs it against the codebase to enforce
# latency ring ownership boundaries and other static checks.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

# Use system Go or override via environment
GO="${GO:-/run/current-system/sw/bin/go}"

# Build the vet binary to a temp location
echo "Building uvb76-vet..."
"$GO" build -o ./uvb76-vet ./cmd/uvb76-vet

# Run the analyzer and clean up
echo "Running uvb76-vet..."
"$GO" vet -vettool=./uvb76-vet ./...

# Clean up
rm -f ./uvb76-vet

echo "uvb76-vet: all checks passed"
