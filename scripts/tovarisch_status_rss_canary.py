#!/usr/bin/env python3
"""
Tovarisch Status RSS Canary - CLI Wrapper

Runtime smoke test for tovarisch /status endpoint memory behavior.
Detects request-driven memory growth by sampling /proc/<pid>/smaps_rollup
or /proc/<pid>/status before and after repeated requests.

Exit codes:
  0 = PASS (memory growth within thresholds)
  1 = FAIL (threshold exceeded or endpoint error)
  2 = SKIP (unsupported platform or missing /proc)
  3 = internal/tooling error
"""

import sys

from tovarisch_status_rss_canary_lib import (
    build_parser,
    format_json_output,
    format_text_output,
    run_canary,
    validate_args,
)

EXIT_CODES = {
    "pass": 0,
    "fail": 1,
    "skip": 2,
    "error": 3,
}


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    validation_error = validate_args(args)
    if validation_error:
        print(f"ERROR: {validation_error}", file=sys.stderr)
        return 3

    result = run_canary(
        url=args.url,
        pid=args.pid,
        warmup_requests=args.warmup_requests,
        sample_requests=args.sample_requests,
        interval_seconds=args.interval_seconds,
        timeout_seconds=args.timeout_seconds,
        max_rss_kib_growth=args.max_rss_kib_growth,
        max_private_kib_growth=args.max_private_kib_growth,
        allow_missing_smaps_rollup=args.allow_missing_smaps_rollup,
        verbose=args.verbose,
    )

    if args.output == "json":
        print(format_json_output(result), end="")
    else:
        print(format_text_output(result), end="")

    return EXIT_CODES.get(result.get("status"), 3)


if __name__ == "__main__":
    raise SystemExit(main())
