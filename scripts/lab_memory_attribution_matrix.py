#!/usr/bin/env python3
# lab_memory_attribution_matrix.py — Long-window memory attribution matrix runner
#
# Thin CLI entrypoint that delegates to memory_attribution_matrix package.

import sys
from pathlib import Path

# Add scripts dir to path for lab_runner
SCRIPT_DIR = Path(__file__).parent
sys.path.insert(0, str(SCRIPT_DIR))

from memory_attribution_matrix import main, self_tests, variants


def main_entry(argv: list[str]) -> int:
    """Main entry point."""
    import argparse
    
    parser = argparse.ArgumentParser(
        description="Memory attribution matrix runner",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Run full matrix with 10-minute variants
  python3 scripts/lab_memory_attribution_matrix.py

  # Run with 30-minute variants for longer observation
  python3 scripts/lab_memory_attribution_matrix.py --duration 1800

  # Run specific variants only
  python3 scripts/lab_memory_attribution_matrix.py --variants all_enabled no_periodic
  
  # Custom run ID
  python3 scripts/lab_memory_attribution_matrix.py --run-id my-test

  # Self-test (module integrity)
  python3 scripts/lab_memory_attribution_matrix.py --self-test
        """
    )
    parser.add_argument(
        "--duration",
        type=int,
        default=600,
        help="Duration per variant in seconds (default: 600)"
    )
    parser.add_argument(
        "--interval",
        type=int,
        default=5,
        help="Memory sample interval in seconds (default: 5)"
    )
    parser.add_argument(
        "--run-id",
        default=None,
        help="Custom matrix run ID"
    )
    parser.add_argument(
        "--variants",
        nargs="+",
        default=None,
        help="Specific variants to run (default: all)"
    )
    parser.add_argument(
        "--self-test",
        action="store_true",
        help="Run self-tests for module integrity"
    )
    
    args = parser.parse_args(argv)
    
    if args.self_test:
        success = self_tests.run_self_tests()
        return 0 if success else 1
    
    # Validate variants
    skip_variants = None
    if args.variants:
        valid_variants = set(variants.MATRIX_VARIANTS.keys())
        requested = set(args.variants)
        invalid = requested - valid_variants
        if invalid:
            print(f"ERROR: Unknown variants: {', '.join(invalid)}")
            print(f"Valid variants: {', '.join(sorted(valid_variants))}")
            return 1
        skip_variants = valid_variants - requested
    
    exit_code, matrix_root = main.run_matrix(
        duration=args.duration,
        interval=args.interval,
        run_id=args.run_id,
        skip_variants=skip_variants,
    )
    
    return exit_code


if __name__ == "__main__":
    sys.exit(main_entry(sys.argv[1:]))
