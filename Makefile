.PHONY: gate digest llm-friendliness tovarisch-build tovarisch-test tovarisch-bounded-memory-reconnect-proof tovarisch-run tovarisch-status tovarisch-serve-liveness tovarisch-compile-linux cross-platform-gate coverage coverage-report verify-structured-logs verify-plaintext-logs health-audit lab-bgp-bfd lab-bgp-bfd-reconnect lab-bgp-bfd-reconnect-bgp-reset install-git-safety-hooks verify-git-history-safety verify-github-ruleset uvb76-build uvb76-build-linux-arm64 uvb76-test uvb76-polling-build uvb76-polling-test lab-uvb76-capture-url verify-memory-budgets verify-memory-lab-artifacts verify-memory-ownership memory-gate memory-lab memory-lab-test lab-tovarisch-memory lab-uvb76-memory lab-uvb76-memory-attribution verify-uvb76-memory-attribution hulk-uvb76-gate tovarisch-test-base-fingerprint verify-script-doctrine hulk-uvb76-artifact-secret-gate hulk-uvb76-artifact-producer-gate hulk-uvb76-capture-gate hulk-uvb76-latency-gate hulk-uvb76-reachability-gate tovarisch-memory-lab-matrix tovarisch-memory-lab-verify-matrix

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
	python3 scripts/verify_tovarisch_status_contract.py

# === Coverage Gate (local only) ===

coverage:
	@echo "=== Coverage Gate ==="
	@./scripts/coverage_gate.sh

coverage-report:
	@echo "=== Coverage Report ==="
	@./scripts/coverage_report.sh

# === Combined Gate (local default) ===

gate: verify-script-doctrine hulk-uvb76-artifact-secret-gate
	./scripts/quality_gate.sh

# === Individual Targets ===

llm-friendliness:
	./scripts/check_llm_friendliness.sh

.PHONY: verify-allocation-tracker-imports
verify-allocation-tracker-imports:
	go run ./cmd/verify-allocation-tracker-imports --self-test
	go run ./cmd/verify-allocation-tracker-imports

digest:
	./scripts/make_targeted_digest.sh --dirty --output digest.txt

tovarisch-build:
	cd tovarisch && zig build

tovarisch-test:
	cd tovarisch && zig build test

tovarisch-bounded-memory-reconnect-proof:
	cd tovarisch && zig build bounded-memory-reconnect-proof --summary all

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
	python3 scripts/verify_tovarisch_status_contract.py

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

# === UVB-76 Hulk Gate ===
# ACT-UVB76-HULK01-CONCURRENCY-RUNTIME-CONTRACT-GATE
# Runtime contract tests for latency, spike, and runtime read paths.
# Tests concurrent sample writes, latency series reads, spike detector median/window reads,
# ring-buffer boundary correctness, NaN/Inf latency inputs, overlapping probe execution,
# and impossible percentile output.

hulk-uvb76-gate:
	@echo "=== UVB-76 Hulk Gate: Runtime Contract Tests ==="
	@cd uvb76 && go test -race -v ./state/... ./server/... ./probe/...
	@echo "=== UVB-76 Hulk Gate: Verifying Runtime Contract Inventory ==="
	@python3 scripts/verify_uvb76_runtime_contracts.py
	@echo "=== UVB-76 Hulk Gate: Verifier Self-Test ==="
	@python3 scripts/verify_uvb76_runtime_contracts.py --self-test

# === UVB-76 Capture Hulk Gate ===
# ACT-UVB76-HULK02-DIAGNOSTIC-CAPTURE-STATE-MACHINE
# Diagnostic capture state machine contract tests for canonical statuses:
# captured, skipped_cooldown, failed, disabled, not_configured, not_attempted,
# in_progress, missing

hulk-uvb76-capture-gate:
	@echo "=== UVB-76 Hulk Gate: Diagnostic Capture State Machine ==="
	@cd uvb76 && go test -race -v ./state/... ./server/... ./diag/...
	@echo "=== UVB-76 Hulk Gate: Verifying Capture State Contract Inventory ==="
	@python3 scripts/verify_uvb76_capture_state_contracts.py
	@echo "=== UVB-76 Hulk Gate: Capture Contract Verifier Self-Test ==="
	@python3 scripts/verify_uvb76_capture_state_contracts.py --self-test

# === UVB-76 Capture Netns Polling ===

# Build and test the polling binary (Go port of shell/JQ polling logic)
uvb76-polling-build:
	cd uvb76/cmd/uvb76-capture-netns-polling && go build -o uvb76-capture-netns-polling .

uvb76-polling-test:
	cd uvb76/cmd/uvb76-capture-netns-polling && go test -v ./...

# === UVB-76 Capture URL Lab ===
# Hermetic regression test for diagnostic capture URL construction.
# Verifies that diagnostic capture uses canonical /status.json?include=network_diag.

lab-uvb76-capture-url:
	@echo "=== UVB-76 Capture URL Lab ==="
	@./scripts/verify_uvb76_capture_url_lab.sh

# === UVB-76 Capture Netns Lab ===
# Runtime lab that runs real UVB-76 and tovarisch in Linux network namespaces
# with network impairment injection to test diagnostic capture behavior.
# Primary execution: GitHub Actions (workflow_dispatch)
# NOT part of make gate

# Go orchestrator (ACT-UVB76-FP07: port from Bash to typed Go)
uvb76-capture-netns-lab:
	cd uvb76/cmd/uvb76-capture-netns-lab && go build -o uvb76-capture-netns-lab .

# Thin wrapper for compatibility; delegates to Go orchestrator
lab-uvb76-capture-netns: uvb76-polling-build uvb76-capture-netns-lab
	@./uvb76/cmd/uvb76-capture-netns-lab/uvb76-capture-netns-lab \
		--artifact-dir ./artifacts/uvb76-capture-netns-lab/$$(date +%Y%m%d-%H%M%S) \
		--uvb76-bin ./uvb76/uvb76 \
		--tovarisch-bin ./tovarisch/zig-out/bin/tovarisch

# === UVB-76 Latency Crash Lab ===
# Canonical Golang daemon crash/soak lab for LatencyTracker SIGSEGV.
# Primary execution: GitHub Actions (workflow_dispatch)
# NOT part of make gate

BASH ?= bash

lab-uvb76-latency-crash:
	@$(MAKE) uvb76-build
	@chmod +x uvb76/uvb76
	@$(BASH) ./scripts/lab_uvb76_latency_crash.sh

# === UVB-76 ICMP OS Ping Soak Lab ===
# Soak lab proving UVB-76 stays alive under continuous ICMP ping probe runs.
# Result honestly shows icmp_probe_exercised=false until daemon counters are exposed.
# Primary execution: GitHub Actions (workflow_dispatch)
# NOT part of make gate

BASH ?= bash

lab-uvb76-icmp-os-ping-soak: uvb76-build
	@UVB76_BINARY="$(CURDIR)/uvb76/uvb76" $(BASH) ./scripts/lab_uvb76_icmp_os_ping_soak.sh

# === UVB-76 Targets Crash Lab ===
# Crash/soak lab proving /api/v1/targets HTTPS surface does not crash under handler churn.
# Tests with runtime-generated TLS certs (no inline blobs).
# Primary execution: GitHub Actions (workflow_dispatch) or `make lab-uvb76-targets-crash`
# NOT part of make gate

BASH ?= bash

lab-uvb76-targets-crash:
	@$(MAKE) uvb76-build
	@chmod +x uvb76/uvb76
	@$(BASH) ./scripts/lab_uvb76_targets_crash.sh

# === UVB-76 TCP Diagnostic Telemetry Lab ===
# Proves TCP telemetry is collected in diagnostic packets using hermetic diagnostic peer.
# Artifact-backed proof: verifies underlay_tcp in captured-diagnostic-packet.json.
# Primary execution: `make lab-uvb76-tcp-diag-telemetry`
# NOT part of make gate

BASH ?= bash

lab-uvb76-tcp-diag-telemetry:
	@$(MAKE) -C uvb76 lab-tcp-diag-telemetry

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

# === Memory Frugality Gates ===
# Fast local gates for memory hygiene and budgets.

verify-memory-budgets:
	@echo "=== Memory Budget Verifier ==="
	python3 scripts/verify_memory_budgets.py

verify-memory-lab-artifacts:
	@echo "=== Memory Lab Artifact Verifier ==="
	python3 scripts/verify_memory_lab_artifact.py

verify-memory-lab-config:
	@echo "=== Memory Lab Config Verifier ==="
	python3 scripts/verify_memory_lab_config.py --self-test
	python3 scripts/verify_memory_lab_config.py

verify-memory-ownership:
	@echo "=== Memory Ownership Hygiene Gate ==="
	bash scripts/check_memory_ownership.sh

memory-gate: verify-memory-budgets verify-memory-lab-artifacts verify-memory-lab-config verify-memory-ownership memory-lab-test
	@echo "=== memory-gate passed ==="

# === Go Memory Lab Runner ===
# Native Go memory lab for real evidence generation.
# Replaces Bash-based scripts under native-owned-critical-paths doctrine.

memory-lab: tools/memory-lab/memory-lab

tools/memory-lab/memory-lab: tools/memory-lab/*.go
	cd tools/memory-lab && go build -o memory-lab .

memory-lab-test:
	cd tools/memory-lab && go test -v ./...

# === Real Memory Labs (Linux only, requires /proc) ===
# These labs measure real memory footprint under controlled workloads.
# Artifacts go to: artifacts/memory-labs/{service}/
# NOT part of local gate; run manually or via CI workflow_dispatch.

BASH ?= bash

# Tovarisch memory lab (Go-based)
lab-tovarisch-memory: memory-lab
	@echo "=== Tovarisch Memory Lab ==="
	@if [ "$$(uname -s)" != "Linux" ]; then \
		echo "[SKIP] Memory labs require Linux (needs /proc)"; \
		exit 0; \
	fi
	@./tools/memory-lab/memory-lab --service tovarisch --workload tovarisch-idle-warmup --warmup-secs 60

# UVB-76 memory lab (Go-based)
lab-uvb76-memory: memory-lab
	@echo "=== UVB-76 Memory Lab ==="
	@if [ "$$(uname -s)" != "Linux" ]; then \
		echo "[SKIP] Memory labs require Linux (needs /proc)"; \
		exit 0; \
	fi
	@./tools/memory-lab/memory-lab \
		--service uvb76 \
		--workload uvb76-idle-warmup \
		--config ./uvb76/uvb76.memory-lab.json \
		--warmup-secs 120

# === UVB-76 Memory Attribution Lab ===
# Long-running memory attribution lab that captures forced-GC memstats,
# heap profiles, goroutine dumps, and RSS/PSS samples over time.
# NOT part of make gate - manual CI target for 30-60 minute soak tests.
#
# Short smoke (default 10 min):
#   make lab-uvb76-memory-attribution
#
# 30-minute soak:
#   make lab-uvb76-memory-attribution ATTRIBUTION_DURATION=1800
#
# 60-minute soak:
#   make lab-uvb76-memory-attribution ATTRIBUTION_DURATION=3600

ATTRIBUTION_DURATION ?= 600
ATTRIBUTION_SAMPLE_MS ?= 5000

lab-uvb76-memory-attribution: memory-lab
	@echo "=== UVB-76 Memory Attribution Lab ==="
	@echo "Duration: $(ATTRIBUTION_DURATION)s ($(shell echo $$(( $(ATTRIBUTION_DURATION) / 60 ))) min)"
	@echo "Sample interval: $(ATTRIBUTION_SAMPLE_MS)ms"
	@if [ "$$(uname -s)" != "Linux" ]; then \
		echo "[SKIP] Attribution labs require Linux (needs /proc)"; \
		exit 0; \
	fi
	@./tools/memory-lab/memory-lab \
		--service uvb76 \
		--workload uvb76-attribution \
		--config ./uvb76/uvb76.memory-lab.json \
		--attribution-duration $(ATTRIBUTION_DURATION) \
		--attribution-sample-ms $(ATTRIBUTION_SAMPLE_MS) \
		--artifacts-dir ./artifacts/memory-labs/uvb76/attribution

# === UVB-76 Memory Attribution Verifier ===
# Verifies attribution lab artifacts (self-test + fixture validation).
# Self-tests run in make gate; real evidence validation is manual.

verify-uvb76-memory-attribution:
	@echo "=== UVB-76 Memory Attribution Artifact Verifier ==="
	@python3 scripts/verify_uvb76_memory_attribution_artifacts.py --self-test

# === Tovarisch Idle Staircase Memory Lab ===
# ACT: Attribute and fix tovarisch idle/background staircase memory growth
#
# This lab runs tovarisch in idle mode and samples RSS/VmData to detect
# stepwise memory growth patterns.
#
# Usage:
#   make lab-tovarisch-idle-memory                    # 10 min idle (default)
#   make lab-tovarisch-idle-memory DURATION=1800     # 30 min idle
#   make lab-tovarisch-idle-memory STATUS_BURST=true # Include /status burst test
#   make lab-tovarisch-idle-memory STRACE=true       # Include syscall tracing
#
# Artifacts: artifacts/memory-labs/tovarisch/idle-staircase/<run-id>/
#
# NOT part of make gate - manual execution only.
# Requires Linux with /proc filesystem.

DURATION ?= 600
STATUS_BURST ?= false
STRACE ?= false

lab-tovarisch-idle-memory:
	@echo "=== Tovarisch Idle Staircase Memory Lab ==="
	@echo "Duration: ${DURATION}s ($(shell echo $$(( ${DURATION} / 60 ))) min)"
	@if [ "$$(uname -s)" != "Linux" ]; then \
		echo "[SKIP] Idle staircase lab requires Linux (needs /proc)"; \
		exit 0; \
	fi
	@chmod +x scripts/lab_tovarisch_idle_memory.sh
	@./scripts/lab_tovarisch_idle_memory.sh \
		--duration ${DURATION} \
		--run-id "idle-$$(date +%Y%m%d-%H%M%S)" \
		$(if $(filter true,${STATUS_BURST}),--status-burst,) \
		$(if $(filter true,${STRACE}),--strace,)

# === Idle Staircase Artifact Verifier ===
# Verifies idle staircase memory lab artifacts (self-test + validation).

verify-idle-staircase-artifact:
	@echo "=== Idle Staircase Artifact Verifier ==="
	@python3 scripts/verify_idle_staircase_artifact.py --self-test

# === Tovarisch Idle Memory Attribution Matrix ===
# Long-window memory attribution matrix that runs multiple lab variants to compare
# native runtime toggles and determine whether idle RSS/PSS growth is caused by a
# specific subsystem or is only bounded allocator/warmup behavior.
#
# Variants run:
#   - all_enabled:     All subsystems (heartbeat, WG checks, BGP, BFD) enabled
#   - heartbeat_disabled:  Heartbeat disabled
#   - wg_disabled:        WG checks disabled
#   - bgp_disabled:       BGP disabled
#   - bfd_disabled:       BFD disabled
#   - bgp_bfd_disabled:   BGP+BFD disabled
#   - no_periodic:        All optional periodic subsystems disabled
#
# Usage:
#   make lab-memory-attribution-matrix                          # 10 min per variant (default)
#   make lab-memory-attribution-matrix MATRIX_DURATION=1800     # 30 min per variant
#   make lab-memory-attribution-matrix MATRIX_INTERVAL=10       # 10s sample interval
#   make lab-memory-attribution-matrix MATRIX_RUN_ID=my-test    # Custom run ID
#
# Artifacts: artifacts/memory-labs/tovarisch/idle-matrix/<run-id>/
#
# NOT part of make gate - manual execution only.
# Requires Linux with /proc filesystem.

MATRIX_DURATION ?= 600
MATRIX_INTERVAL ?= 5
MATRIX_RUN_ID ?= ""

lab-memory-attribution-matrix:
	@echo "=== Tovarisch Idle Memory Attribution Matrix ==="
	@echo "Duration: ${MATRIX_DURATION}s per variant ($(shell echo $$(( ${MATRIX_DURATION} / 60 ))) min)"
	@echo "Interval: ${MATRIX_INTERVAL}s"
	@if [ "$$(uname -s)" != "Linux" ]; then \
		echo "[SKIP] Memory attribution matrix requires Linux (needs /proc)"; \
		exit 0; \
	fi
	@chmod +x scripts/lab_memory_attribution_matrix.sh
	@./scripts/lab_memory_attribution_matrix.sh \
		--duration ${MATRIX_DURATION} \
		--interval ${MATRIX_INTERVAL} \
		$(if $(MATRIX_RUN_ID),--run-id ${MATRIX_RUN_ID},)

# === Memory Attribution Matrix Verifier ===
# Verifies matrix artifacts (self-test + validation).

verify-memory-attribution-matrix:
	@echo "=== Memory Attribution Matrix Verifier ==="
	@python3 scripts/verify_memory_attribution_matrix.py --self-test

# === WireGuard Generic-Netlink Lab ===
# Runtime proof harness for GenericNetlinkBackend against real WireGuard kernel interface.
#
# Prerequisites:
#   - Linux kernel
#   - WireGuard kernel module (wireguard.ko)
#   - CAP_NET_ADMIN or root
#
# NOT part of make gate - manual CI target for privileged runners.
# Artifacts: artifacts/wg-netlink-lab/

WG_NETLINK_LAB := ./tools/wg-netlink-lab/wg-netlink-lab
WG_NETLINK_PROOF := ./tovarisch/zig-out/bin/wg_netlink_proof

# Build the Zig proof binary for GenericNetlinkBackend
wg-netlink-proof:
	cd tovarisch && zig build wg-netlink-proof

# Build the Go lab harness
$(WG_NETLINK_LAB):
	cd tools/wg-netlink-lab && go build -o wg-netlink-lab .

# Full WireGuard generic-netlink lab: build harness, prepare deps, then run full proof on Linux
.PHONY: lab-wg-netlink
lab-wg-netlink: $(WG_NETLINK_LAB)
	@echo "=== WireGuard Generic-Netlink Lab ==="
	@if [ "$$(uname -s)" != "Linux" ]; then \
		echo "[SKIP] WireGuard netlink lab requires Linux"; \
		echo "[INFO] Preflight only:"; \
		$(WG_NETLINK_LAB) preflight || true; \
		exit 0; \
	fi
	@echo "=== Preparing Linux network lab dependencies ==="
	@chmod +x ./scripts/install_linux_net_lab_deps.sh
	@./scripts/install_linux_net_lab_deps.sh
	@$(MAKE) wg-netlink-proof
	@$(WG_NETLINK_LAB) full

# === Tovarisch Status RSS Canary ===
# Runtime smoke test for /status endpoint request-driven memory growth.
# NOT part of make gate - requires live tovarisch and Linux /proc.
# Usage:
#   TOVARISCH_STATUS_URL=http://10.149.149.1:8317/status TOVARISCH_PID=2174927 make tovarisch-status-rss-canary
#   TOVARISCH_PID=2174927 make tovarisch-status-rss-canary-local

.PHONY: tovarisch-status-rss-canary
tovarisch-status-rss-canary:
	@echo "=== Tovarisch Status RSS Canary ==="
	python3 scripts/tovarisch_status_rss_canary.py \
		--url "$${TOVARISCH_STATUS_URL:?set TOVARISCH_STATUS_URL}" \
		--pid "$${TOVARISCH_PID:?set TOVARISCH_PID}"

.PHONY: tovarisch-status-rss-canary-local
tovarisch-status-rss-canary-local:
	@echo "=== Tovarisch Status RSS Canary (local) ==="
	python3 scripts/tovarisch_status_rss_canary.py \
		--url "$${TOVARISCH_STATUS_URL:-http://127.0.0.1:8317/status}" \
		--pid "$${TOVARISCH_PID:?set TOVARISCH_PID}"

# === Tovarisch Test-Base Fingerprint ===
# ACT-HULK29R-ZIG016-TEST-BASE-SKIP-PROFILE-FINGERPRINT
# Diagnostic fingerprint for `zig build test-base` to capture environment context
# and explain pass/skip/fail profile divergence between CI and local runs.
# NOT part of make gate - diagnostic ACT first.
#
# Usage:
#   make tovarisch-test-base-fingerprint
#   make tovarisch-test-base-fingerprint SEED=0xa710199f

SEED ?= 0xa710199f

.PHONY: tovarisch-test-base-fingerprint
tovarisch-test-base-fingerprint:
	@echo "=== Tovarisch Test-Base Fingerprint ==="
	@echo "Seed: $(SEED)"
	python3 scripts/tovarisch_test_base_fingerprint.py --seed $(SEED)


# === UVB-76 Latency Series Query Boundary Hulk Gate ===
# ACT-UVB76-HULK03-LATENCY-SERIES-QUERY-BOUNDARY-FUZZ
hulk-uvb76-latency-gate:
	@echo "=== UVB-76 Hulk Gate: Latency Series Query Boundary ==="
	@cd uvb76 && go test -race -v ./state/... ./server/... -run 'LatencySeries|FuzzLatencySeries'
	@echo "=== UVB-76 Hulk Gate: Running Bounded Fuzz Tests ==="
	@cd uvb76 && go test ./server/... -run '^$$' -fuzz FuzzLatencySeriesQueryParams -fuzztime=10s
	@cd uvb76 && go test ./server/... -run '^$$' -fuzz FuzzLatencySeriesWindowStepRange -fuzztime=10s
	@echo "=== UVB-76 Hulk Gate: Verifying Latency Series Contract Inventory ==="
	@python3 scripts/verify_uvb76_latency_series_contracts.py
	@echo "=== UVB-76 Hulk Gate: Verifier Self-Test ==="
	@python3 scripts/verify_uvb76_latency_series_contracts.py --self-test

# === UVB-76 Probe Reachability Semantics Hulk Gate ===
# ACT-UVB76-HULK04-PROBE-REACHABILITY-SEMANTICS
# Explicit reachability vocabulary contract tests for HTTP/ICMP probe independence.
# Canonical statuses: target_reachable, service_reachable, partially_reachable,
# service_unreachable, network_unreachable, unknown.
hulk-uvb76-reachability-gate:
	@echo "=== UVB-76 Hulk Gate: Probe Reachability Semantics ==="
	@cd uvb76 && go test -race -v ./probe/... ./state/... ./server/... -run 'Reachability|ProbeSemantics'
	@echo "=== UVB-76 Hulk Gate: Verifying Reachability Contract Inventory ==="
	@python3 scripts/verify_uvb76_reachability_contracts.py
	@echo "=== UVB-76 Hulk Gate: Verifier Self-Test ==="
	@python3 scripts/verify_uvb76_reachability_contracts.py --self-test

# === UVB-76 Artifact Secret Hygiene Hulk Gate (R4 composition) ===
# ACT-UVB76-HULK05-ARTIFACT-SECRET-HYGIENE
# Repository-wide deterministic artifact-secret-hygiene contract.
# Verifies tracked artifacts do not contain prohibited secret classes.
# Implements two-tier scanning: universal critical rules + artifact-context rules.
# Diagnostics never expose detected values.
#
# R4 wiring: this rule MUST depend on hulk-uvb76-artifact-producer-gate so that
# ValidateCatalog runs from the real gate before this scanner runs.
# Composition is asserted by uvb76/cmd/uvb76-makefile-composition-check.
hulk-uvb76-artifact-secret-gate: hulk-uvb76-artifact-producer-gate
	@echo "=== UVB-76 Hulk Gate: Artifact Secret Hygiene (R4 composition) ==="
	@echo "=== Verifier Self-Test ==="
	@python3 scripts/verify_uvb76_artifact_secret_hygiene.py --self-test
	@echo "=== Go Redaction Tests ==="
	@cd uvb76 && go test ./internal/redact -v
	@cd uvb76 && go test ./internal/redact -race
	@echo "=== Makefile Composition Self-Test ==="
	@cd uvb76 && go run ./cmd/uvb76-makefile-composition-check -makefile ../Makefile -repo ..
	@echo "=== Scanning Artifact Surfaces ==="
	@python3 scripts/verify_uvb76_artifact_secret_hygiene.py
	@echo "=== UVB-76 Hulk Gate: Artifact Secret Hygiene PASSED ==="

# === Script Doctrine Verification ===
# ACT-UVB76-GO-TOOLING-DOCTRINE01
# Verifies repository tooling follows Go-first doctrine:
# - No Python files
# - No Python invocations in Makefiles or shell
# - Shell scripts within LOC limits
# - All scripts in inventory
# - No risky shell patterns

verify-script-doctrine:
	@echo "=== Script Doctrine Verification ==="
	go run ./cmd/verify-script-doctrine --bootstrap


# === UVB-76 Artifact Producer Enforcement Hulk Gate ===
# ACT-UVB76-HULK05R4-ARTIFACT-PRODUCER-ENFORCEMENT
# Enforces executable producer contracts: every registered active producer
# must sanitize before persistence, must pass typed Go AST bypass
# detection, and must have a real serializer-level test.
hulk-uvb76-artifact-producer-gate:
	@echo "=== UVB-76 Hulk Gate: Artifact Producer Enforcement ==="
	@echo "=== Canonical catalog metrics ==="
	@cd uvb76 && go run ./cmd/uvb76-artifact-writer-verify -self-test
	@echo "=== Artifactio boundary tests ==="
	@cd uvb76 && go test -count=1 ./internal/artifactio
	@cd uvb76 && go test -count=1 -race ./internal/artifactio
	@echo "=== Producer contract tests (no global mutation) ==="
	@cd uvb76 && go test -count=50 ./internal/producer
	@cd uvb76 && go test -race ./internal/producer
	@echo "=== Producer registry self-test ==="
	@cd uvb76 && go run ./cmd/uvb76-artifact-writer-verify -self-test
	@echo "=== Go AST writer-bypass verifier ==="
	@cd uvb76 && go run ./cmd/uvb76-artifact-writer-verify
	@echo "=== UVB-76 Hulk Gate: Artifact Producer Enforcement PASSED ==="

# === Tovarisch Memory Lab Docker Targets ===
# ACT-TOVARISCH-GO-MEMORY-LAB01-CORRECTION07: Docker Canary Acceptance
#
# Go-based Docker laboratory for Tovarisch memory investigation.
# Uses Docker Engine Go client directly; no os/exec docker commands.
# Binary installed to: .factory/bin/tovarisch-memory-lab
#
# Exact classification matrix:
#   canary-growing:    overall=growth, memory=growing, scenario_valid=true, canaries_valid=true
#   canary-bounded:    overall=stable, memory=stable, scenario_valid=true, canaries_valid=true
#   canary-descriptor: overall=resource_growth, resource=resource_growth, scenario_valid=true, canaries_valid=true

MEMORY_LAB := .factory/bin/tovarisch-memory-lab


# Build extract-image-metadata helper
# CORRECTION23: Replaces Python JSON parsing in canary image build.
extract-image-metadata:
	@mkdir -p .factory/bin
	cd tovarisch/labs/memory && go build -o ../../.factory/bin/extract-image-metadata ./cmd/extract-image-metadata

# Build the canary image with immutable OCI + kgb.dev labels
# CORRECTION02 §7: bind the canary binary to the tested source tree.
# CORRECTION23: Requires extract-image-metadata helper.
tovarisch-memory-lab-canary-image: extract-image-metadata
	@bash scripts/build_tovarisch_canary_image.sh

tovarisch-memory-lab-build:
	@mkdir -p .factory/bin
	cd tovarisch/labs/memory && go build -o ../../../.factory/bin/tovarisch-memory-lab ./cmd/tovarisch-memory-lab

# Short growing probe - runs BEFORE the full matrix to verify deterministic phase tests
tovarisch-memory-lab-growing-probe: tovarisch-memory-lab-build
	@echo "=== Memory Lab: Growing Probe (short semantic test) ==="
	"$(MEMORY_LAB)" run \
		--scenario canary-growing \
		--duration 32 \
		--artifacts-dir .factory/tovarisch-memory-lab

tovarisch-memory-lab-test:
	cd tovarisch/labs/memory && go test -v ./...

tovarisch-memory-lab-test-race:
	cd tovarisch/labs/memory && go test -race -v ./...

tovarisch-memory-lab-canary-growing:
	@echo "=== Memory Lab: Canary Growing ==="
	"$(MEMORY_LAB)" run \
		--scenario canary-growing \
		--duration 60 \
		--artifacts-dir .factory/tovarisch-memory-lab

tovarisch-memory-lab-canary-bounded:
	@echo "=== Memory Lab: Canary Bounded ==="
	"$(MEMORY_LAB)" run \
		--scenario canary-bounded \
		--duration 60 \
		--artifacts-dir .factory/tovarisch-memory-lab

tovarisch-memory-lab-canary-descriptor:
	@echo "=== Memory Lab: Canary Descriptor ==="
	"$(MEMORY_LAB)" run \
		--scenario canary-descriptor \
		--duration 60 \
		--artifacts-dir .factory/tovarisch-memory-lab

# Full canary suite: all three Docker canaries
tovarisch-memory-lab-canary-suite: tovarisch-memory-lab-build
	@echo "=== Memory Lab: Canary Suite ==="
	"$(MEMORY_LAB)" run \
		--scenario canary-growing \
		--duration 60 \
		--artifacts-dir .factory/tovarisch-memory-lab
	"$(MEMORY_LAB)" run \
		--scenario canary-bounded \
		--duration 60 \
		--artifacts-dir .factory/tovarisch-memory-lab
	"$(MEMORY_LAB)" run \
		--scenario canary-descriptor \
		--duration 60 \
		--artifacts-dir .factory/tovarisch-memory-lab

tovarisch-memory-lab-verify-evidence:
	@test -n "$(RUN_ID)" || { echo "RUN_ID is required, e.g. make tovarisch-memory-lab-verify-evidence RUN_ID=lab-canary-growing-1234567890"; exit 2; }
	@echo "=== Memory Lab Evidence Verifier ==="
	"$(MEMORY_LAB)" verify \
		--artifacts-dir .factory/tovarisch-memory-lab \
		--run-id "$(RUN_ID)"

tovarisch-memory-lab-clean:
	rm -rf .factory/tovarisch-memory-lab

# === ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01 ===
# Matrix command: execute all three scenarios with frozen execution identity

tovarisch-memory-lab-matrix: tovarisch-memory-lab-build
	@echo "=== Memory Lab: Matrix (all three scenarios with frozen identity) ==="
	@mkdir -p .factory/tovarisch-memory-lab
	"$(MEMORY_LAB)" matrix \
		--duration 60 \
		--artifacts-dir .factory/tovarisch-memory-lab

tovarisch-memory-lab-verify-matrix:
	@if [ -z "$(MATRIX_DIR)" ]; then \
		echo "MATRIX_DIR is required, e.g. make tovarisch-memory-lab-verify-matrix MATRIX_DIR=.factory/tovarisch-memory-lab/matrix-1234567890"; \
		exit 1; \
	fi
	@echo "=== Memory Lab Matrix Verifier ==="
	"$(MEMORY_LAB)" verify-matrix \
		--matrix-dir "$(MATRIX_DIR)"
	cd tovarisch/labs/memory && go clean
