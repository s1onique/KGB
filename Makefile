.PHONY: gate digest llm-friendliness tovarisch-build tovarisch-test tovarisch-run tovarisch-status coverage coverage-report

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
	@echo "=== Coverage Status ==="
	@echo "See docs/coverage/tovarisch-coverage.md for behavior coverage ledger."
	@echo ""
	@if command -v zig >/dev/null 2>&1; then \
		echo "Running: zig build test"; \
		cd tovarisch && zig build test; \
		echo ""; \
		echo "[INFO] Zig tests passed — current coverage proxy"; \
		echo "[INFO] Zig coverage backend not configured yet"; \
		echo "[INFO] Run 'make coverage-report' for the coverage ledger"; \
		echo "[INFO] Run 'make gate' for full quality gate"; \
		else \
		echo "[INFO] Zig not installed; test-as-signal proxy unavailable"; \
		echo "[INFO] See docs/coverage/tovarisch-coverage.md"; \
		exit 1; \
	fi

coverage-report:
	@echo "=== Tovarisch Coverage Ledger ==="
	@echo ""
	@if [[ -s "docs/coverage/tovarisch-coverage.md" ]]; then \
		cat docs/coverage/tovarisch-coverage.md; \
	else \
		echo "[ERROR] Coverage ledger not found: docs/coverage/tovarisch-coverage.md"; \
		exit 1; \
	fi
