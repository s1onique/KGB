"""IPK self-test runner and utilities."""

import hashlib
import io
import os
import shutil
import sys
import tempfile
from contextlib import redirect_stderr, redirect_stdout

from ipk_fixture_builders import create_good_fixture
from ipk_fixture_mutators import (
    mod_missing_debian_binary,
    mod_wrong_debian_binary,
    mod_missing_control,
    mod_legacy_control,
    mod_missing_postinst,
    mod_missing_prerm,
    mod_bad_package_name,
    mod_bad_architecture,
    mod_outside_opt,
    mod_absolute_path,
    mod_dotdot_path,
    mod_non_exec_init,
    mod_missing_config,
    mod_rc_unslung,
    mod_sha256_mismatch,
    mod_ar_outer_format,
)
from ipk_verifier import verify_ipk, log_info, log_pass, log_fail


def run_verify_capture(path: str):
    """Run verify_ipk and capture stdout/stderr."""
    stdout = io.StringIO()
    stderr = io.StringIO()
    with redirect_stdout(stdout), redirect_stderr(stderr):
        ok = verify_ipk(path, verbose=False)
    return ok, stdout.getvalue() + stderr.getvalue()


def run_self_tests() -> bool:
    """Run self-tests. Returns True if all pass."""
    log_info("Running self-tests...")
    print()

    with tempfile.TemporaryDirectory() as work_dir:
        passed = 0
        failed = 0
        total = 0

        test_cases = [
            ("valid package passes", None, "Package verification passed"),
            ("missing debian-binary fails", mod_missing_debian_binary, "debian-binary"),
            ("wrong debian-binary fails", mod_wrong_debian_binary, "debian-binary must be"),
            ("missing control fails", mod_missing_control, "control.tar.gz missing"),
            ("legacy CONTROL/control fails", mod_legacy_control, "legacy"),
            ("missing postinst fails", mod_missing_postinst, "missing ./postinst"),
            ("missing prerm fails", mod_missing_prerm, "missing ./prerm"),
            ("bad package name fails", mod_bad_package_name, "Package name must be"),
            ("unsupported architecture fails", mod_bad_architecture, "Unsupported architecture"),
            ("payload outside opt fails", mod_outside_opt, "writes outside"),
            ("absolute payload path fails", mod_absolute_path, "writes outside"),
            ("dotdot traversal fails", mod_dotdot_path, "writes outside"),
            ("non-exec init fails", mod_non_exec_init, "not executable"),
            ("missing config fails", mod_missing_config, "Missing in data payload"),
            ("rc.unslung sourcing fails", mod_rc_unslung, "rc.unslung"),
            ("sha256 mismatch fails", mod_sha256_mismatch, "SHA256 mismatch"),
            ("ar outer format fails", mod_ar_outer_format, "outer package must be gzip tar"),
        ]

        print("=== Self-Test Results ===")
        print()

        good_fixture_path = create_good_fixture(work_dir)
        shutil.copy(good_fixture_path, os.path.join(work_dir, '_good.ipk'))
        shutil.copy(good_fixture_path + '.sha256', os.path.join(work_dir, '_good.ipk.sha256'))

        for name, modifier, expected in test_cases:
            total += 1
            fixture_name = name.split()[0]

            if modifier is None:
                fixture_path = good_fixture_path
            else:
                fixture_path = os.path.join(work_dir, f'{fixture_name}.ipk')
                modifier(os.path.join(work_dir, '_good.ipk'), fixture_path)
                if name != "sha256 mismatch fails":
                    with open(fixture_path, 'rb') as f:
                        sha256 = hashlib.sha256(f.read()).hexdigest()
                    with open(fixture_path + '.sha256', 'w') as f:
                        f.write(sha256)

            log_info(f"Test {total}: {name}")

            ok, output = run_verify_capture(fixture_path)

            if modifier is None:
                if ok and expected in output:
                    log_pass(f"Test {total} passed")
                    passed += 1
                else:
                    log_fail(f"Test {total} failed")
                    print(f"Expected diagnostic: {expected}", file=sys.stderr)
                    print(output, file=sys.stderr)
                    failed += 1
            else:
                if (not ok) and expected in output:
                    log_pass(f"Test {total} passed")
                    passed += 1
                else:
                    log_fail(f"Test {total} failed")
                    print(f"Expected diagnostic: {expected}", file=sys.stderr)
                    print(output, file=sys.stderr)
                    failed += 1

            print()

        print("=== Self-Test Summary ===")
        print(f"Total: {total}")
        print(f"Passed: {passed}")
        print(f"Failed: {failed}")

        if failed > 0:
            log_fail("Self-tests failed")
            return False

        log_pass("All self-tests passed")
        return True
