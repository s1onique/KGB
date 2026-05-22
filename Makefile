.PHONY: gate digest llm-friendliness tovarisch-build tovarisch-test tovarisch-run tovarisch-status coverage coverage-report

# Coverage threshold: percentage of line coverage required to pass
COVERAGE_THRESHOLD ?= 60

gate:
	./scripts/quality_gate.sh

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

coverage:
	@echo "=== Coverage Gate ==="
	@./scripts/coverage_gate.sh

coverage-report:
	@echo "=== Coverage Report ==="
	@./scripts/coverage_report.sh

.PHONY: verify-status-contract

verify-status-contract:
	./scripts/verify_tovarisch_status_contract.sh
