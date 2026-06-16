#!/usr/bin/env python3
"""
verify_opkg_package.py - Verify Entware/opkg package structure

Validates an .ipk package against opkg package requirements:
  - Valid ar archive with debian-binary, control.tar.gz, data.tar.gz
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
import hashlib
import io
import os
import shutil
import sys
import tarfile
import tempfile
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from typing import BinaryIO, Dict

# Allowlisted Entware architectures
ALLOWED_ARCHS = frozenset([
    "aarch64-3.10", "aarch64-k3.10", "armv7hf", "armebhf",
    "arm-softfloat-linux-gnueabi", "armv5tel", "mipsel", "mipselsf",
    "mips-3.4", "x86_64", "i686"
])


def log_info(msg: str) -> None:
    print(f"[INFO] {msg}")


def log_pass(msg: str) -> None:
    print(f"[PASS] {msg}")


def log_fail(msg: str) -> None:
    print(f"[FAIL] {msg}", file=sys.stderr)


def log_verbose(msg: str, verbose: bool = False) -> None:
    if verbose:
        print(f"[VERBOSE] {msg}")


# === AR Archive Parsing ===

def _ar_field(value: str, width: int) -> bytes:
    """Encode a string as a fixed-width ASCII field, padded with spaces."""
    data = value.encode("ascii")
    if len(data) > width:
        raise ValueError(f"ar field too long for width {width}: {value!r}")
    return data.ljust(width, b" ")


def parse_ar_members(f: BinaryIO) -> Dict[str, bytes]:
    """Parse an ar archive and return a dict of member_name -> content."""
    members: Dict[str, bytes] = {}

    magic = f.read(8)
    if magic != b"!<arch>\n":
        raise ValueError("Not an ar archive")

    while True:
        header = f.read(60)
        if header == b"":
            break
        if len(header) != 60:
            raise ValueError("Truncated ar member header")
        if header[58:60] != b"`\n":
            raise ValueError(f"Invalid ar header magic: {header[58:60]!r}")

        raw_name = header[0:16].decode("ascii", errors="strict")
        raw_size = header[48:58].decode("ascii", errors="strict").strip()

        try:
            size = int(raw_size)
        except ValueError as exc:
            raise ValueError(f"Invalid ar member size: {raw_size!r}") from exc

        name = raw_name.rstrip()
        if name.endswith("/"):
            name = name[:-1]

        data = f.read(size)
        if len(data) != size:
            raise ValueError(f"Truncated ar member data for {name!r}")

        members[name] = data

        if size % 2:
            pad = f.read(1)
            if pad == b"":
                raise ValueError(f"Missing ar padding byte after {name!r}")

    return members


def write_ar_member(f: BinaryIO, name: str, data: bytes) -> None:
    """Write one GNU/SVR4-style ar member with a 60-byte header."""
    if "/" in name:
        raise ValueError(f"fixture ar writer only supports simple member names: {name!r}")

    member_name = name + "/"

    header = b"".join([
        _ar_field(member_name, 16),       # ar_name
        _ar_field("0", 12),              # ar_date (12 bytes, not 10!)
        _ar_field("0", 6),                # ar_uid
        _ar_field("0", 6),                # ar_gid
        _ar_field("100644", 8),           # ar_mode
        _ar_field(str(len(data)), 10),    # ar_size
        b"`\n",                           # ar_fmag
    ])

    if len(header) != 60:
        raise AssertionError(f"internal ar header bug: {len(header)} bytes")

    f.write(header)
    f.write(data)

    if len(data) % 2:
        f.write(b"\n")


def create_ar_archive(output_path: str, members: Dict[str, bytes]) -> None:
    """Create an ar archive with the given members."""
    with open(output_path, 'wb') as f:
        f.write(b"!<arch>\n")
        for name, data in members.items():
            write_ar_member(f, name, data)


# === IPK Package Verification ===

def verify_ipk(ipk_path: str, verbose: bool = False) -> bool:
    """Verify an ipk package. Returns True if valid, False otherwise."""
    log_info(f"Verifying package: {ipk_path}")
    
    if not os.path.exists(ipk_path):
        log_fail(f"File not found: {ipk_path}")
        return False
    
    if os.path.getsize(ipk_path) == 0:
        log_fail(f"File is empty: {ipk_path}")
        return False
    
    log_verbose(f"File exists and non-empty: {os.path.getsize(ipk_path)} bytes", verbose)
    
    with open(ipk_path, 'rb') as f:
        try:
            ar_members = parse_ar_members(f)
        except ValueError as e:
            log_fail(f"Not a valid ar archive: {e}")
            return False
    
    required_ar_members = ['debian-binary', 'control.tar.gz', 'data.tar.gz']
    for member in required_ar_members:
        if member not in ar_members:
            log_fail(f"Missing ar member: {member}")
            return False
    
    log_verbose("ar archive: OK", verbose)
    
    debian_binary = ar_members['debian-binary'].decode('utf-8', errors='replace').strip()
    if debian_binary != '2.0':
        log_fail(f"debian-binary must be '2.0', got: '{debian_binary}'")
        return False
    log_verbose("debian-binary: OK", verbose)
    
    control_tar = ar_members['control.tar.gz']
    try:
        with tarfile.open(fileobj=io.BytesIO(control_tar), mode='r:gz') as tf:
            control_entries = tf.getnames()
    except Exception as e:
        log_fail(f"control.tar.gz is not a valid gzip archive: {e}")
        return False
    
    log_verbose("control.tar.gz: OK", verbose)
    
    normalized_entries = set()
    for entry in control_entries:
        if entry.startswith('./'):
            normalized_entries.add(entry[2:])
        else:
            normalized_entries.add(entry)
    
    if 'control' not in normalized_entries:
        log_fail("control.tar.gz missing ./control")
        return False
    log_verbose("control.tar.gz has ./control: OK", verbose)
    
    if 'postinst' not in normalized_entries:
        log_fail("control.tar.gz missing ./postinst")
        return False
    log_verbose("control.tar.gz has ./postinst: OK", verbose)
    
    if 'prerm' not in normalized_entries:
        log_fail("control.tar.gz missing ./prerm")
        return False
    log_verbose("control.tar.gz has ./prerm: OK", verbose)
    
    if 'CONTROL/control' in control_entries or 'CONTROL/control' in normalized_entries:
        log_fail("control.tar.gz uses legacy CONTROL/control layout")
        return False
    
    with tarfile.open(fileobj=io.BytesIO(control_tar), mode='r:gz') as tf:
        # Try both control and ./control (Python vs macOS tar)
        try:
            control_file = tf.extractfile('control')
        except KeyError:
            try:
                control_file = tf.extractfile('./control')
            except KeyError:
                log_fail("Missing control file in control.tar.gz")
                return False
        
        control_content = control_file.read().decode('utf-8', errors='replace')
    
    control_fields = {}
    for line in control_content.split('\n'):
        if ':' in line:
            key, value = line.split(':', 1)
            control_fields[key.strip()] = value.strip()
    
    required_fields = ['Package', 'Version', 'Architecture', 'Maintainer', 
                      'Description', 'Section', 'Priority']
    for field in required_fields:
        if field not in control_fields:
            log_fail(f"Missing required field in control: {field}")
            return False
    
    pkg_name = control_fields['Package']
    if pkg_name != 'uvb76':
        log_fail(f"Package name must be 'uvb76', got: '{pkg_name}'")
        return False
    
    arch = control_fields['Architecture']
    if arch not in ALLOWED_ARCHS:
        log_fail(f"Unsupported architecture: '{arch}'")
        return False
    
    log_verbose("control metadata: OK", verbose)
    
    data_tar = ar_members['data.tar.gz']
    try:
        with tarfile.open(fileobj=io.BytesIO(data_tar), mode='r:gz') as tf:
            data_entries = tf.getnames()
    except Exception as e:
        log_fail(f"data.tar.gz is not a valid gzip archive: {e}")
        return False
    
    log_verbose("data.tar.gz: OK", verbose)
    
    normalized_data = set()
    for entry in data_entries:
        if entry.startswith('./'):
            normalized_data.add(entry[2:])
        else:
            normalized_data.add(entry)
    
    required_payload = [
        'opt/bin/uvb76',
        'opt/etc/init.d/S76uvb76',
        'opt/etc/uvb76/uvb76.json.example'
    ]
    for required in required_payload:
        if required not in normalized_data:
            log_fail(f"Missing in data payload: {required}")
            return False
    
    with tarfile.open(fileobj=io.BytesIO(data_tar), mode='r:gz') as tf:
        try:
            init_info = tf.getmember('opt/etc/init.d/S76uvb76')
            if not (init_info.mode & 0o100):
                log_fail("Init script /opt/etc/init.d/S76uvb76 is not executable")
                return False
        except KeyError:
            try:
                init_info = tf.getmember('./opt/etc/init.d/S76uvb76')
                if not (init_info.mode & 0o100):
                    log_fail("Init script /opt/etc/init.d/S76uvb76 is not executable")
                    return False
            except KeyError:
                log_fail("Missing init script in data.tar.gz")
                return False
    
    log_verbose("init script executable: OK", verbose)
    
    for entry in data_entries:
        if entry.endswith('/'):
            continue
        
        # Skip macOS AppleDouble files (._ prefix for extended attributes)
        if entry.startswith('._'):
            continue
        
        if entry.startswith('./'):
            entry = entry[2:]
        
        # Skip bare directory entries (e.g., "opt" without trailing slash)
        if entry == 'opt':
            continue
        
        if entry.startswith('/'):
            log_fail(f"File writes outside /opt: {entry}")
            return False
        
        parts = entry.split('/')
        if '..' in parts:
            log_fail(f"File writes outside /opt: {entry}")
            return False
        
        if not entry.startswith('opt/'):
            log_fail(f"File writes outside /opt: {entry}")
            return False
        
        if entry == 'Makefile':
            log_fail("Package contains Makefile")
            return False
        if '_test.go' in entry or 'test_' in entry:
            log_fail("Package contains Go test files")
            return False
    
    with tarfile.open(fileobj=io.BytesIO(data_tar), mode='r:gz') as tf:
        try:
            init_file = tf.extractfile('opt/etc/init.d/S76uvb76')
            if init_file is None:
                init_file = tf.extractfile('./opt/etc/init.d/S76uvb76')
            init_content = init_file.read().decode('utf-8', errors='replace')
            # Check for actual sourcing, not just mentions in comments
            # Pattern: . or source followed by rc.unslung
            import re
            if re.search(r'[.\s]source\s+rc\.unslung|\.\s+.*rc\.unslung', init_content):
                log_fail("Init script must not source rc.unslung")
                return False
        except KeyError:
            pass
    
    log_verbose("data payload: OK", verbose)
    
    sha256_path = ipk_path + '.sha256'
    if not os.path.exists(sha256_path):
        log_fail(f"Missing SHA256 sidecar: {sha256_path}")
        return False
    
    with open(sha256_path, 'r') as f:
        sha256_content = f.read().strip()
    
    if not sha256_content:
        log_fail(f"SHA256 sidecar is empty: {sha256_path}")
        return False
    
    expected_sha = sha256_content.split()[0]
    
    with open(ipk_path, 'rb') as f:
        actual_sha = hashlib.sha256(f.read()).hexdigest()
    
    if expected_sha != actual_sha:
        log_fail(f"SHA256 mismatch: expected {expected_sha}, got {actual_sha}")
        return False
    
    log_verbose("SHA256: OK", verbose)
    
    log_pass(f"Package verification passed: {ipk_path}")
    return True


# === Self-Test ===

def run_verify_capture(path: str) -> tuple[bool, str]:
    """Run verify_ipk and capture stdout/stderr."""
    stdout = io.StringIO()
    stderr = io.StringIO()
    with redirect_stdout(stdout), redirect_stderr(stderr):
        ok = verify_ipk(path, verbose=False)
    return ok, stdout.getvalue() + stderr.getvalue()


def create_good_fixture(work_dir: str) -> str:
    """Create a valid ipk fixture. Returns the fixture path."""
    fixture_path = os.path.join(work_dir, 'good.ipk')
    
    control_content = """Package: uvb76
Version: 1.0.0-1
Architecture: aarch64-3.10
Maintainer: KGB Project <kgb@example.com>
Description: UVB-76 - KGB Control Plane Station
Section: net
Priority: optional
"""
    
    postinst_content = """#!/bin/sh
echo "installed"
"""
    
    prerm_content = """#!/bin/sh
echo "removing"
"""
    
    control_tar_buffer = io.BytesIO()
    with tarfile.open(fileobj=control_tar_buffer, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))
        
        info = tarfile.TarInfo(name='postinst')
        info.size = len(postinst_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(postinst_content.encode()))
        
        info = tarfile.TarInfo(name='prerm')
        info.size = len(prerm_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(prerm_content.encode()))
    
    control_tar = control_tar_buffer.getvalue()
    
    data_tar_buffer = io.BytesIO()
    with tarfile.open(fileobj=data_tar_buffer, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        info = tarfile.TarInfo(name='opt/etc/uvb76/uvb76.json.example')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))
    
    data_tar = data_tar_buffer.getvalue()
    
    members = {
        'debian-binary': b'2.0\n',
        'control.tar.gz': control_tar,
        'data.tar.gz': data_tar
    }
    create_ar_archive(fixture_path, members)
    
    with open(fixture_path, 'rb') as f:
        sha256 = hashlib.sha256(f.read()).hexdigest()
    with open(fixture_path + '.sha256', 'w') as f:
        f.write(sha256)
    
    return fixture_path


def mod_missing_debian_binary(src: str, dst: str) -> None:
    """Modify fixture to be missing debian-binary."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    del members['debian-binary']
    create_ar_archive(dst, members)


def mod_wrong_debian_binary(src: str, dst: str) -> None:
    """Modify fixture to have wrong debian-binary."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    members['debian-binary'] = b'3.0\n'
    create_ar_archive(dst, members)


def mod_missing_control(src: str, dst: str) -> None:
    """Modify fixture to be missing control."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    empty_tar = io.BytesIO()
    with tarfile.open(fileobj=empty_tar, mode='w:gz') as tf:
        pass
    members['control.tar.gz'] = empty_tar.getvalue()
    create_ar_archive(dst, members)


def mod_legacy_control(src: str, dst: str) -> None:
    """Modify fixture to use legacy CONTROL/control layout."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    control_content = """Package: uvb76
Version: 1.0.0-1
Architecture: aarch64-3.10
Maintainer: Test <test@test.com>
Description: Test
Section: net
Priority: optional
"""
    
    control_tar = io.BytesIO()
    with tarfile.open(fileobj=control_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='CONTROL/control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))
    
    members['control.tar.gz'] = control_tar.getvalue()
    create_ar_archive(dst, members)


def mod_missing_postinst(src: str, dst: str) -> None:
    """Modify fixture to be missing postinst."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    control_content = """Package: uvb76
Version: 1.0.0-1
Architecture: aarch64-3.10
Maintainer: KGB Project <kgb@example.com>
Description: UVB-76 - KGB Control Plane Station
Section: net
Priority: optional
"""
    
    prerm_content = """#!/bin/sh
echo "removing"
"""
    
    control_tar = io.BytesIO()
    with tarfile.open(fileobj=control_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))
        
        info = tarfile.TarInfo(name='prerm')
        info.size = len(prerm_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(prerm_content.encode()))
    
    members['control.tar.gz'] = control_tar.getvalue()
    create_ar_archive(dst, members)


def mod_missing_prerm(src: str, dst: str) -> None:
    """Modify fixture to be missing prerm."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    control_content = """Package: uvb76
Version: 1.0.0-1
Architecture: aarch64-3.10
Maintainer: KGB Project <kgb@example.com>
Description: UVB-76 - KGB Control Plane Station
Section: net
Priority: optional
"""
    
    postinst_content = """#!/bin/sh
echo "installed"
"""
    
    control_tar = io.BytesIO()
    with tarfile.open(fileobj=control_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))
        
        info = tarfile.TarInfo(name='postinst')
        info.size = len(postinst_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(postinst_content.encode()))
    
    members['control.tar.gz'] = control_tar.getvalue()
    create_ar_archive(dst, members)


def mod_bad_package_name(src: str, dst: str) -> None:
    """Modify fixture to have bad package name."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    control_content = """Package: wrong-name
Version: 1.0.0-1
Architecture: aarch64-3.10
Maintainer: Test <test@test.com>
Description: Wrong package
Section: net
Priority: optional
"""
    
    postinst_content = """#!/bin/sh
echo "installed"
"""
    
    prerm_content = """#!/bin/sh
echo "removing"
"""
    
    control_tar = io.BytesIO()
    with tarfile.open(fileobj=control_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))
        
        info = tarfile.TarInfo(name='postinst')
        info.size = len(postinst_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(postinst_content.encode()))
        
        info = tarfile.TarInfo(name='prerm')
        info.size = len(prerm_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(prerm_content.encode()))
    
    members['control.tar.gz'] = control_tar.getvalue()
    create_ar_archive(dst, members)


def mod_bad_architecture(src: str, dst: str) -> None:
    """Modify fixture to have bad architecture."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    control_content = """Package: uvb76
Version: 1.0.0-1
Architecture: unsupported-arch
Maintainer: Test <test@test.com>
Description: Bad arch
Section: net
Priority: optional
"""
    
    postinst_content = """#!/bin/sh
echo "installed"
"""
    
    prerm_content = """#!/bin/sh
echo "removing"
"""
    
    control_tar = io.BytesIO()
    with tarfile.open(fileobj=control_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))
        
        info = tarfile.TarInfo(name='postinst')
        info.size = len(postinst_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(postinst_content.encode()))
        
        info = tarfile.TarInfo(name='prerm')
        info.size = len(prerm_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(prerm_content.encode()))
    
    members['control.tar.gz'] = control_tar.getvalue()
    create_ar_archive(dst, members)


def mod_outside_opt(src: str, dst: str) -> None:
    """Modify fixture to have payload outside /opt."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    data_tar = io.BytesIO()
    with tarfile.open(fileobj=data_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        info = tarfile.TarInfo(name='opt/etc/uvb76/uvb76.json.example')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))
        
        info = tarfile.TarInfo(name='etc/uvb76.conf')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
    
    members['data.tar.gz'] = data_tar.getvalue()
    create_ar_archive(dst, members)


def mod_absolute_path(src: str, dst: str) -> None:
    """Modify fixture to have absolute path in payload."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    data_tar = io.BytesIO()
    with tarfile.open(fileobj=data_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        info = tarfile.TarInfo(name='opt/etc/uvb76/uvb76.json.example')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))
        
        info = tarfile.TarInfo(name='/opt/etc/uvb76.conf')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
    
    members['data.tar.gz'] = data_tar.getvalue()
    create_ar_archive(dst, members)


def mod_dotdot_path(src: str, dst: str) -> None:
    """Modify fixture to have .. traversal in payload."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    data_tar = io.BytesIO()
    with tarfile.open(fileobj=data_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        info = tarfile.TarInfo(name='opt/etc/uvb76/uvb76.json.example')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))
        
        info = tarfile.TarInfo(name='opt/../etc/uvb76.conf')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
    
    members['data.tar.gz'] = data_tar.getvalue()
    create_ar_archive(dst, members)


def mod_non_exec_init(src: str, dst: str) -> None:
    """Modify fixture to have non-executable init script."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    data_tar = io.BytesIO()
    with tarfile.open(fileobj=data_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        info = tarfile.TarInfo(name='opt/etc/uvb76/uvb76.json.example')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o644
        tf.addfile(info, io.BytesIO(init_content.encode()))
    
    members['data.tar.gz'] = data_tar.getvalue()
    create_ar_archive(dst, members)


def mod_missing_config(src: str, dst: str) -> None:
    """Modify fixture to be missing config file."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    data_tar = io.BytesIO()
    with tarfile.open(fileobj=data_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))
    
    members['data.tar.gz'] = data_tar.getvalue()
    create_ar_archive(dst, members)


def mod_rc_unslung(src: str, dst: str) -> None:
    """Modify fixture to have init script sourcing rc.unslung."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    data_tar = io.BytesIO()
    with tarfile.open(fileobj=data_tar, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        info = tarfile.TarInfo(name='opt/etc/uvb76/uvb76.json.example')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))
        
        init_content = "#!/bin/sh\n[ -f /opt/etc/init.d/rc.unslung ] && . /opt/etc/init.d/rc.unslung\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))
    
    members['data.tar.gz'] = data_tar.getvalue()
    create_ar_archive(dst, members)


def mod_sha256_mismatch(src: str, dst: str) -> None:
    """Modify fixture to have mismatched SHA256."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)
    
    create_ar_archive(dst, members)
    
    with open(dst + '.sha256', 'w') as f:
        f.write('0' * 64)


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
        ]
        
        print("=== Self-Test Results ===")
        print()
        
        good_fixture_path = create_good_fixture(work_dir)
        shutil.copy(good_fixture_path, os.path.join(work_dir, '_good.ipk'))
        
        for name, modifier, expected in test_cases:
            total += 1
            fixture_name = name.split()[0]
            
            if modifier is None:
                fixture_path = good_fixture_path
            else:
                fixture_path = os.path.join(work_dir, f'{fixture_name}.ipk')
                modifier(os.path.join(work_dir, '_good.ipk'), fixture_path)
                # Don't create SHA256 for sha256 mismatch test - it already has wrong SHA256
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
        if run_self_tests():
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
