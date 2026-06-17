.PHONY: gate digest llm-friendliness tovarisch-build tovarisch-test tovarisch-run tovarisch-status tovarisch-serve-liveness tovarisch-compile-linux cross-platform-gate coverage coverage-report verify-structured-logs verify-plaintext-logs health-audit lab-bgp-bfd lab-bgp-bfd-reconnect lab-bgp-bfd-reconnect-bgp-reset install-git-safety-hooks verify-git-history-safety verify-github-ruleset uvb76-build uvb76-build-linux-arm64 uvb76-test

# Coverage threshold: percentage of line coverage required to pass
COVERAGE_THRESHOLD ?= 87
export COVERAGE_THRESHOLD

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

# === BGP Sub-Suite Targets (for CI isolation) ===

tovarisch-test-bgp-protocol:
	cd tovarisch && zig build test-bgp-protocol --summary all

tovarisch-test-bgp-session:
	cd tovarisch && zig build test-bgp-session --summary all

tovarisch-test-bgp-tcp:
	cd tovarisch && zig build test-bgp-tcp --summary all

tovarisch-test-bgp-integration:
	cd tovarisch && zig build test-bgp-integration --summary all

# Combined BGP sub-suite step (runs all BGP sub-suites)
tovarisch-test-bgp-split:
	cd tovarisch && zig build test-bgp-split --summary all

tovarisch-run:
	cd tovarisch && zig build run -- --version

tovarisch-status:
	cd tovarisch && zig build run -- status --json

tovarisch-serve-liveness: tovarisch-build
	./scripts/check_tovarisch_serve_liveness.sh ./tovarisch/zig-out/bin/tovarisch

# === Cross-Platform Compile Checks ===
# These targets verify Linux-only code paths compile correctly on non-Linux hosts.
# Required because Zig does not semantically analyze Linux branches on macOS/Windows.
# NOTE: Cross-compiled tests cannot execute on non-Linux hosts; compile-only verification.

tovarisch-compile-linux:
	# Compile tovarisch for Linux target from non-Linux host.
	# This catches platform-specific API drift in @import("builtin").os.tag branches.
	cd tovarisch && zig build -Dtarget=x86_64-linux-gnu

# === Platform-Specific Semantic Analysis Gate ===

# Cross-platform compile gate: catches platform-specific API drift that local gate misses.
# On macOS, Linux-only code in @import("builtin").os.tag == .linux branches is not analyzed.
# On Linux CI, those branches become live and expose any API mismatches.
cross-platform-gate: tovarisch-compile-linux
	@echo "=== Cross-platform compile gate passed ==="

verify-status-contract:
	./scripts/verify_tovarisch_status_contract.sh

test-final-newlines-regression:
	./scripts/check_final_newlines_regression.sh

# === Structured Logs Verification ===

verify-structured-logs:
	./scripts/verify_structured_logs.sh

verify-plaintext-logs:
	./scripts/verify_plaintext_logs.sh

# === Debian Package ===

package-deb:
	./scripts/package_deb.sh

verify-deb-systemd:
	./scripts/verify_deb_systemd_package.sh

deb-gate: package-deb verify-deb-systemd
	@echo "=== deb-gate passed ==="

release-artifacts: package-deb
	ls -lh dist/*.deb

# === Scheduled Health Audit (Advisory) ===
# This target runs advisory health checks on the repository.
# It is scheduled via GitHub Actions and does NOT block development.

health-audit:
	@./scripts/audit_repo_health.sh

# === BGP/BFD Netns Lab (Manual CI) ===
# Manual lab for tovarisch BFD/BGP behavior using Linux network namespaces.
# Primary execution: GitHub Actions (workflow_dispatch)
# Local execution: optional for debugging only
# NOT part of make gate

BASH ?= bash

lab-bgp-bfd:
	@$(BASH) ./scripts/lab_bgp_bfd_netns.sh

# === BGP/BFD Reconnect Lab ===
# Proves: BGP recovers WITHOUT restarting tovarisch.
# Failure injection: BIRD restart.
# Primary execution: GitHub Actions (workflow_dispatch)
# NOT part of make gate

lab-bgp-bfd-reconnect:
	@./scripts/lab_bgp_bfd_reconnect.sh

# === BGP/BFD Reconnect Lab — BGP Protocol Reset Scenario ===
# Proves: BGP protocol reset with BFD healthy, tovarisch reconnects without restart.
# Failure injection: BIRD protocol disable/enable (peer-side BGP restart).
# Sibling scenario to lab-bgp-bfd-reconnect.sh.
# Primary execution: GitHub Actions (workflow_dispatch)
# NOT part of make gate

lab-bgp-bfd-reconnect-bgp-reset:
	@./scripts/lab_bgp_bfd_reconnect_bgp_reset.sh

# === Git History Safety ===
# Prevents force pushes, branch/tag deletions, and history rewriting.

# Install the git safety pre-push hook
install-git-safety-hooks:
	@./scripts/install_git_safety_hooks.sh

# Verify git history safety policy is properly configured
verify-git-history-safety:
	@./scripts/verify_git_history_safety_policy.sh

# Verify GitHub ruleset blocks force pushes (skips if not in CI)
verify-github-ruleset:
	@./scripts/verify_github_no_force_push_ruleset.sh

# === UVB-76 Targets ===

uvb76-web-build:
	cd uvb76/web && npm ci
	cd uvb76/web && npm run build

uvb76-verify-embed: uvb76-web-build
	cd uvb76 && bash scripts/verify_web_embed.sh

uvb76-build: uvb76-web-build
	cd uvb76 && go build -o uvb76 .

uvb76-build-linux-arm64: uvb76-web-build
	cd uvb76 && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 go build -o uvb76-linux-arm64 .

uvb76-test: uvb76-web-build
	cd uvb76 && go test -v ./...

# === opkg Package Targets (Entware/AsusWRT-Merlin) ===

# Build opkg package for Entware/AsusWRT-Merlin
opkg-package:
	@mkdir -p dist/opkg
	@bash scripts/build_opkg_package.sh

# Verify opkg package structure
# Only verifies the most recently built package (matches VERSION pattern)
verify-opkg-package:
	@bash scripts/verify_opkg_package.sh --self-test
	@LATEST_IPK=$$(ls -t dist/opkg/uvb76_*.ipk 2>/dev/null | head -1); \
	if [ -n "$$LATEST_IPK" ]; then \
		bash scripts/verify_opkg_package.sh "$$LATEST_IPK"; \
	else \
		echo "No uvb76 package found in dist/opkg/"; \
		exit 1; \
	fi

# Combined opkg gate: build, self-test, and verify all packages
opkg-gate: opkg-package verify-opkg-package
	@echo "=== opkg-gate passed ==="

