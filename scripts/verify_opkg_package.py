#!/usr/bin/env python3
"""
verify_opkg_package.py - Verify Entware/opkg package structure

Validates an .ipk package against opkg package requirements:
  - Valid gzip tar archive with debian-binary, control.tar.gz, data.tar.gz
  - Required control fields present and valid
  - Control archive contains ./control, ./postinst, ./prerm
  - Package payload contained only under /opt
  - No prohibited files (source tree, .git, etc.)
  - SHA256 sidecar present
  - Config file uvb76.json.example present
  - Init script does not source rc.unslung

Supports --self-test mode with good/bad fixtures.
"""

import argparse
import sys

from ipk_fixtures import run_self_tests as ipk_run_self_tests
from ipk_verifier import verify_ipk


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Verify Entware/opkg package structure",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s dist/opkg/uvb76_1.0.0-1_aarch64-3.10.ipk
  %(prog)s --self-test
  %(prog)s --verbose dist/opkg/uvb76_1.0.0-1_aarch64-3.10.ipk
"""
    )
    
    parser.add_argument('--self-test', action='store_true',
                        help='Run self-test with good/bad fixtures')
    parser.add_argument('--verbose', '-v', action='store_true',
                        help='Enable verbose output')
    parser.add_argument('ipk_file', nargs='?',
                        help='Path to .ipk file to verify')
    
    args = parser.parse_args()
    
    if args.self_test:
        if ipk_run_self_tests():
            return 0
        return 1
    elif args.ipk_file:
        if verify_ipk(args.ipk_file, verbose=args.verbose):
            return 0
        return 1
    else:
        parser.print_help()
        return 1


if __name__ == '__main__':
    sys.exit(main())
