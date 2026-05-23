.PHONY: gate digest llm-friendliness tovarisch-build tovarisch-test tovarisch-run tovarisch-status tovarisch-serve-liveness coverage coverage-report

# Coverage threshold: percentage of line coverage required to pass
COVERAGE_THRESHOLD ?= 60

# === Split Gates ===

# hygiene-gate: source-policy checks (non-coverage subset of quality_gate.sh)
# Includes: LLM-friendliness, final newlines, required docs, forbidden naming,
# privacy doctrine, AGENTS/.clinerules content, coverage ledger commands.
# Does NOT include: Zig build/test (see test-gate), coverage (see make coverage).
hygiene-gate:
	@./scripts/quality_gate.sh --hygiene-only
	@./scripts/check_final_newlines_regression.sh

# test-gate: build + test + status contract
test-gate:
	cd tovarisch && zig build
	cd tovarisch && zig build test
	./scripts/verify_tovarisch_status_contract.sh

# === Coverage Gate (local only) ===

coverage:
	@echo "=== Coverage Gate ==="
	@./scripts/coverage_gate.sh

coverage-report:
	@echo "=== Coverage Report ==="
	@./scripts/coverage_report.sh

# === Combined Gate (local default) ===

gate:
	./scripts/quality_gate.sh

# === Individual Targets ===

llm-friendliness:
	./scripts/check_llm_friendliness.sh

digest:
	./scripts/make_targeted_digest.sh --dirty --output digest.txt

tovarisch-build:
	cd tovarisch && zig build

tovarisch-test:
	cd tovarisch && zig build test

tovarisch-run:
	cd tovarisch && zig build run -- --version

tovarisch-status:
	cd tovarisch && zig build run -- status --json

tovarisch-serve-liveness: tovarisch-build
	./scripts/check_tovarisch_serve_liveness.sh ./tovarisch/zig-out/bin/tovarisch

verify-status-contract:
	./scripts/verify_tovarisch_status_contract.sh

test-final-newlines-regression:
	./scripts/check_final_newlines_regression.sh

# === Debian Package ===

package-deb:
	./scripts/package_deb.sh

release-artifacts: package-deb
	ls -lh dist/*.deb
