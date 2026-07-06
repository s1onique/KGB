#!/usr/bin/env python3
"""
Tovarisch Status RSS Canary - Implementation Library

Runtime smoke test for tovarisch /status endpoint memory behavior.
Detects request-driven memory growth by sampling /proc/<pid>/smaps_rollup
or /proc/<pid>/status before and after repeated requests.
"""

import argparse, json, os, platform, sys, time, urllib.error, urllib.request


def parse_memory_size_kib(value: str) -> int:
    """Parse a memory size value and return kibibytes."""
    value = value.strip()
    if value.endswith("kB"):
        return int(value[:-2].strip())
    elif value.endswith("KB"):
        return int(value[:-2].strip())
    elif value.endswith("mB"):
        return int(float(value[:-2].strip()) * 1024)
    elif value.endswith("MB"):
        return int(float(value[:-2].strip()) * 1024)
    elif value.endswith("gB"):
        return int(float(value[:-2].strip()) * 1024 * 1024)
    elif value.endswith("GB"):
        return int(float(value[:-2].strip()) * 1024 * 1024)
    else:
        return int(value.strip())


def parse_smaps_rollup(smaps_path: str) -> dict | None:
    """Parse /proc/<pid>/smaps_rollup and return memory metrics dict or None."""
    try:
        with open(smaps_path, "r") as f:
            content = f.read()
    except (FileNotFoundError, PermissionError, OSError):
        return None

    result = {
        "Rss": None,
        "Pss": None,
        "Private_Clean": None,
        "Private_Dirty": None,
        "Shared_Clean": None,
        "Shared_Dirty": None,
    }

    for line in content.split("\n"):
        if not line.strip():
            continue
        if ":" not in line:
            continue
        key, value = line.split(":", 1)
        key = key.strip()
        value = value.strip()
        if key in result:
            result[key] = parse_memory_size_kib(value)

    # Verify we got the required fields
    if any(v is None for v in result.values()):
        return None

    # Compute private_kib
    result["private_kib"] = result["Private_Clean"] + result["Private_Dirty"]
    return result


def parse_proc_status(status_path: str) -> dict | None:
    """Parse /proc/<pid>/status and return memory metrics dict or None."""
    try:
        with open(status_path, "r") as f:
            content = f.read()
    except (FileNotFoundError, PermissionError, OSError):
        return None

    result = {
        "VmRSS": None,
        "RssAnon": None,
        "RssFile": None,
        "RssShmem": None,
        "VmData": None,
    }

    for line in content.split("\n"):
        if not line.strip():
            continue
        if ":" not in line:
            continue
        key, value = line.split(":", 1)
        key = key.strip()
        value = value.strip()
        if key in result:
            result[key] = parse_memory_size_kib(value)

    # Fallback uses VmRSS and computes private as RssAnon
    if result["VmRSS"] is None:
        return None

    # If RssAnon is available, use it as private; otherwise VmRSS is an upper bound
    if result["RssAnon"] is not None:
        result["private_kib"] = result["RssAnon"]
    else:
        result["private_kib"] = result["VmRSS"]

    return result


def get_memory_source(pid: int, allow_missing_smaps_rollup: bool = False) -> tuple[str | None, dict | None, str]:
    """
    Determine memory source and read memory metrics.

    Returns: (source_name, metrics_dict, error_message)
    """
    # Check Linux platform
    if platform.system() != "Linux":
        return None, None, "not_linux"

    # Verify PID exists via /proc
    proc_path = f"/proc/{pid}"
    if not os.path.isdir(proc_path):
        return None, None, "pid_not_found"

    # Try smaps_rollup first
    smaps_rollup_path = f"{proc_path}/smaps_rollup"
    if os.path.isfile(smaps_rollup_path):
        metrics = parse_smaps_rollup(smaps_rollup_path)
        if metrics is not None:
            return "smaps_rollup", metrics, ""

    # Fallback to /proc/<pid>/status
    status_path = f"{proc_path}/status"
    metrics = parse_proc_status(status_path)
    if metrics is not None:
        return "status", metrics, ""

    if allow_missing_smaps_rollup:
        return None, None, "proc_files_missing"

    return None, None, "proc_files_missing"


def http_get(url: str, timeout: float) -> tuple[bool, str]:
    """
    Perform HTTP GET request.
    Returns: (success, body_or_error_message)
    """
    try:
        with urllib.request.urlopen(url, timeout=timeout) as response:
            body = response.read()
            if not body:
                return False, "empty_body"
            if response.status < 200 or response.status >= 300:
                return False, f"http_{response.status}"
            return True, body.decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        return False, f"http_{e.code}"
    except urllib.error.URLError as e:
        return False, f"url_error_{e.reason}"
    except TimeoutError:
        return False, "timeout"
    except Exception as e:
        return False, f"error_{type(e).__name__}"


def run_canary(
    url: str,
    pid: int,
    warmup_requests: int,
    sample_requests: int,
    interval_seconds: float,
    timeout_seconds: float,
    max_rss_kib_growth: int,
    max_private_kib_growth: int,
    allow_missing_smaps_rollup: bool,
    verbose: bool,
) -> dict:
    """
    Run the memory canary test.

    Returns a dict with:
      - status: "pass", "fail", "skip", or "error"
      - reason: failure reason if status is "fail"
      - memory_source: source used
      - rss_kib_before, rss_kib_after, rss_kib_delta
      - private_kib_before, private_kib_after, private_kib_delta
      - thresholds: dict with max values
      - other relevant fields
    """
    result = {
        "status": "error",
        "reason": "",
        "pid": pid,
        "url": url,
        "memory_source": None,
        "warmup_requests": warmup_requests,
        "sample_requests": sample_requests,
        "rss_kib_before": None,
        "rss_kib_after": None,
        "rss_kib_delta": None,
        "private_kib_before": None,
        "private_kib_after": None,
        "private_kib_delta": None,
        "thresholds": {
            "max_rss_kib_growth": max_rss_kib_growth,
            "max_private_kib_growth": max_private_kib_growth,
        },
    }

    # Step 1: Verify Linux /proc support
    if platform.system() != "Linux":
        result["status"] = "skip"
        result["reason"] = "not_linux"
        return result

    # Step 2: Verify PID exists
    proc_path = f"/proc/{pid}"
    if not os.path.isdir(proc_path):
        result["status"] = "skip"
        result["reason"] = "pid_not_found"
        return result

    # Step 3: Verify endpoint is reachable (preflight)
    success, msg = http_get(url, timeout_seconds)
    if not success:
        result["status"] = "fail"
        result["reason"] = f"endpoint_unreachable_{msg}"
        return result

    if verbose:
        print(f"[canary] endpoint preflight: OK", file=sys.stderr)

    # Step 4: Warm up the endpoint
    for i in range(warmup_requests):
        success, msg = http_get(url, timeout_seconds)
        if not success:
            result["status"] = "fail"
            result["reason"] = f"warmup_request_failed_{msg}"
            return result
        if interval_seconds > 0:
            time.sleep(interval_seconds)

    if verbose:
        print(f"[canary] warmup complete: {warmup_requests} requests", file=sys.stderr)

    # Step 5: Sleep 0.1s then capture baseline memory
    time.sleep(0.1)

    source, metrics_before, _ = get_memory_source(pid, allow_missing_smaps_rollup)
    if metrics_before is None:
        result["status"] = "skip"
        result["reason"] = "proc_files_missing"
        return result

    result["memory_source"] = source
    result["rss_kib_before"] = metrics_before.get("Rss") or metrics_before.get("VmRSS")
    result["private_kib_before"] = metrics_before.get("private_kib")

    if verbose:
        print(f"[canary] baseline memory: RSS={result['rss_kib_before']} KiB, "
              f"private={result['private_kib_before']} KiB", file=sys.stderr)

    # Step 6: Send sample requests
    for i in range(sample_requests):
        success, msg = http_get(url, timeout_seconds)
        if not success:
            result["status"] = "fail"
            result["reason"] = f"sample_request_failed_{msg}"
            return result
        if interval_seconds > 0:
            time.sleep(interval_seconds)

    if verbose:
        print(f"[canary] sample phase complete: {sample_requests} requests", file=sys.stderr)

    # Step 7: Sleep 0.1s then capture final memory
    time.sleep(0.1)

    _, metrics_after, _ = get_memory_source(pid, allow_missing_smaps_rollup)
    if metrics_after is None:
        result["status"] = "skip"
        result["reason"] = "proc_files_missing"
        return result

    result["rss_kib_after"] = metrics_after.get("Rss") or metrics_after.get("VmRSS")
    result["private_kib_after"] = metrics_after.get("private_kib")

    if verbose:
        print(f"[canary] final memory: RSS={result['rss_kib_after']} KiB, "
              f"private={result['private_kib_after']} KiB", file=sys.stderr)

    # Step 8: Compute deltas
    result["rss_kib_delta"] = result["rss_kib_after"] - result["rss_kib_before"]
    result["private_kib_delta"] = result["private_kib_after"] - result["private_kib_before"]

    if verbose:
        print(f"[canary] deltas: RSS={result['rss_kib_delta']} KiB, "
              f"private={result['private_kib_delta']} KiB", file=sys.stderr)

    # Step 9: Evaluate thresholds
    rss_ok = result["rss_kib_delta"] <= max_rss_kib_growth
    private_ok = result["private_kib_delta"] <= max_private_kib_growth

    if rss_ok and private_ok:
        result["status"] = "pass"
    else:
        result["status"] = "fail"
        if not rss_ok:
            result["reason"] = "rss_kib_delta_exceeded"
        else:
            result["reason"] = "private_kib_delta_exceeded"

    return result


def format_text_output(result: dict) -> str:
    """Format result as human-readable text."""
    if result["status"] == "pass":
        lines = [
            "TOVARISCH STATUS RSS CANARY: PASS",
            f"pid={result['pid']}",
            f"url={result['url']}",
            f"memory_source={result['memory_source']}",
            f"warmup_requests={result['warmup_requests']}",
            f"sample_requests={result['sample_requests']}",
            f"rss_kib_before={result['rss_kib_before']}",
            f"rss_kib_after={result['rss_kib_after']}",
            f"rss_kib_delta={result['rss_kib_delta']}",
            f"private_kib_before={result['private_kib_before']}",
            f"private_kib_after={result['private_kib_after']}",
            f"private_kib_delta={result['private_kib_delta']}",
            f"max_rss_kib_growth={result['thresholds']['max_rss_kib_growth']}",
            f"max_private_kib_growth={result['thresholds']['max_private_kib_growth']}",
        ]
    elif result["status"] == "fail":
        lines = [
            "TOVARISCH STATUS RSS CANARY: FAIL",
            f"reason={result['reason']}",
            f"private_kib_delta={result['private_kib_delta']}",
            f"max_private_kib_growth={result['thresholds']['max_private_kib_growth']}",
            f"rss_kib_delta={result['rss_kib_delta']}",
            f"sample_requests={result['sample_requests']}",
        ]
    elif result["status"] == "skip":
        lines = [
            "TOVARISCH STATUS RSS CANARY: SKIP",
            f"reason={result['reason']}",
        ]
    else:
        lines = [
            "TOVARISCH STATUS RSS CANARY: ERROR",
            f"reason={result.get('reason', 'unknown')}",
        ]

    return "\n".join(lines) + "\n"


def format_json_output(result: dict) -> str:
    """Format result as JSON."""
    return json.dumps(result, indent=2) + "\n"


def build_parser() -> argparse.ArgumentParser:
    """Build and return the argument parser."""
    parser = argparse.ArgumentParser(
        description="Tovarisch /status endpoint runtime RSS memory canary",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 tovarisch_status_rss_canary.py --url http://10.149.149.1:8317/status --pid 2174927

  python3 tovarisch_status_rss_canary.py \\
      --url http://127.0.0.1:8317/status \\
      --pid 12345 \\
      --warmup-requests 50 \\
      --sample-requests 500 \\
      --max-rss-kib-growth 8192 \\
      --max-private-kib-growth 2048
        """,
    )

    parser.add_argument(
        "--url", required=True,
        help="URL of the tovarisch /status endpoint"
    )
    parser.add_argument(
        "--pid", type=int, required=True,
        help="PID of the running tovarisch process"
    )
    parser.add_argument(
        "--warmup-requests", type=int, default=25,
        help="Number of warmup requests (default: 25)"
    )
    parser.add_argument(
        "--sample-requests", type=int, default=200,
        help="Number of sample requests (default: 200)"
    )
    parser.add_argument(
        "--interval-seconds", type=float, default=0.0,
        help="Interval between requests in seconds (default: 0.0)"
    )
    parser.add_argument(
        "--timeout-seconds", type=float, default=2.0,
        help="HTTP request timeout in seconds (default: 2.0)"
    )
    parser.add_argument(
        "--max-rss-kib-growth", type=int, default=4096,
        help="Maximum allowed RSS growth in KiB (default: 4096)"
    )
    parser.add_argument(
        "--max-private-kib-growth", type=int, default=1024,
        help="Maximum allowed private memory growth in KiB (default: 1024)"
    )
    parser.add_argument(
        "--output", choices=["text", "json"], default="text",
        help="Output format (default: text)"
    )
    parser.add_argument(
        "--allow-missing-smaps-rollup", action="store_true",
        help="Allow fallback to /proc/PID/status when smaps_rollup is unavailable"
    )
    parser.add_argument(
        "--verbose", action="store_true",
        help="Print verbose progress to stderr"
    )

    return parser


def validate_args(args: argparse.Namespace) -> str | None:
    """
    Validate parsed arguments.
    Returns error message if invalid, None if valid.
    """
    if args.warmup_requests < 0:
        return "warmup-requests must be non-negative"
    if args.sample_requests < 0:
        return "sample-requests must be non-negative"
    if args.interval_seconds < 0:
        return "interval-seconds must be non-negative"
    if args.timeout_seconds <= 0:
        return "timeout-seconds must be positive"
    if args.max_rss_kib_growth < 0:
        return "max-rss-kib-growth must be non-negative"
    if args.max_private_kib_growth < 0:
        return "max-private-kib-growth must be non-negative"
    return None
