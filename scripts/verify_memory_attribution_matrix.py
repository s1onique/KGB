#!/usr/bin/env python3
# verify_memory_attribution_matrix.py — Verifier for memory attribution matrix artifacts
#
# Thin CLI entrypoint that delegates to memory_attribution_matrix_verifier package.

import sys
from pathlib import Path

from memory_attribution_matrix_verifier import verify, self_tests
from memory_attribution_matrix_verifier.checks import CANONICAL_VARIANTS


def main():
    import argparse
    
    parser = argparse.ArgumentParser(description="Verify memory attribution matrix artifacts")
    parser.add_argument("matrix_root", nargs="?", help="Path to matrix root directory")
    parser.add_argument("--self-test", action="store_true", help="Run self-tests")
    
    args = parser.parse_args()
    
    if args.self_test:
        success = self_tests.run_self_tests()
        sys.exit(0 if success else 1)
    
    if not args.matrix_root:
        parser.print_help()
        sys.exit(1)
    
    matrix_root = Path(args.matrix_root)
    
    if not matrix_root.exists():
        print(f"ERROR: Matrix root does not exist: {matrix_root}", file=sys.stderr)
        sys.exit(1)
    
    print("=== Memory Attribution Matrix Verifier ===")
    print(f"Matrix Root: {matrix_root}")
    
    declared = verify.read_matrix_manifest(matrix_root)
    if declared is None:
        print(f"Variants: {len(CANONICAL_VARIANTS)} (canonical full set)")
    else:
        print(f"Variants: {len(declared)} (from matrix-manifest.yaml)")
    print()
    
    valid, error, results = verify.verify_matrix(matrix_root)
    
    print("Variant Results:\n")
    
    all_ok = True
    for variant in declared or CANONICAL_VARIANTS:
        if variant not in results:
            continue
        variant_data = results[variant]
        if variant_data.get("valid"):
            print(f"  OK {variant}")
            data = variant_data.get("data", {})
            vd = data.get("verdict", {})
            nc = data.get("native_counts", {})
            print(f"    Verdict: {vd.get('verdict', 'unknown')}")
            print(f"    Growth: {vd.get('total_growth_kib', 'N/A')} KiB")
            print(f"    Native events: HB={nc.get('heartbeat', 0)}, WG={nc.get('wireguard', 0)}, BGP={nc.get('bgp', 0)}, BFD={nc.get('bfd', 0)}")
        else:
            print(f"  FAIL {variant}: {variant_data.get('error', 'Unknown error')}")
            all_ok = False
        print()
    
    if valid:
        print("VERIFICATION PASSED")
        sys.exit(0)
    else:
        print(f"VERIFICATION FAILED: {error}")
        sys.exit(1)


if __name__ == "__main__":
    main()
