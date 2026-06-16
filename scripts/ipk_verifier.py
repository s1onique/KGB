"""IPK package verification logic."""

import gzip
import hashlib
import io
import os
import re
import sys
import tarfile
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


def _parse_gzip_tar_members(ipk_path: str) -> dict[str, bytes]:
    """Parse outer gzip tar and return a dict of member_name -> content."""
    members: dict[str, bytes] = {}
    
    with open(ipk_path, 'rb') as f:
        # Decompress gzip
        try:
            gzip_file = gzip.GzipFile(fileobj=f)
            # Read the entire gzip content into memory
            content = gzip_file.read()
        except Exception as e:
            raise ValueError(f"Not a valid gzip archive: {e}")
    
    # Parse inner tar
    tar_buffer = io.BytesIO(content)
    with tarfile.open(fileobj=tar_buffer, mode='r') as tf:
        for member in tf.getmembers():
            if member.isfile():
                f = tf.extractfile(member)
                if f is not None:
                    members[member.name] = f.read()
    
    return members


def _normalize_entry(name: str) -> str:
    """Normalize tar entry name by stripping leading ./"""
    if name.startswith('./'):
        return name[2:]
    return name


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
    
    # Parse outer gzip tar
    try:
        members = _parse_gzip_tar_members(ipk_path)
    except ValueError as e:
        log_fail(f"outer package must be gzip tar: {e}")
        return False
    
    # Check for ar magic to give a better error message
    with open(ipk_path, 'rb') as f:
        magic = f.read(8)
        if magic.startswith(b"!<arch>"):
            log_fail("outer package must be gzip tar, not ar archive (Debian format)")
            return False
    
    required_members = ['debian-binary', 'control.tar.gz', 'data.tar.gz']
    for member in required_members:
        # Accept with or without leading ./
        if member not in members and f'./{member}' not in members:
            log_fail(f"Missing outer member: {member}")
            return False
    
    log_verbose("outer gzip tar: OK", verbose)
    
    # Get debian-binary (with or without ./ prefix)
    debian_binary = members.get('debian-binary') or members.get('./debian-binary')
    debian_binary = debian_binary.decode('utf-8', errors='replace').strip()
    if debian_binary != '2.0':
        log_fail(f"debian-binary must be '2.0', got: '{debian_binary}'")
        return False
    log_verbose("debian-binary: OK", verbose)
    
    # Get control.tar.gz (with or without ./ prefix)
    control_tar = members.get('control.tar.gz') or members.get('./control.tar.gz')
    try:
        with tarfile.open(fileobj=io.BytesIO(control_tar), mode='r:gz') as tf:
            control_entries = tf.getnames()
    except Exception as e:
        log_fail(f"control.tar.gz is not a valid gzip archive: {e}")
        return False
    
    log_verbose("control.tar.gz: OK", verbose)
    
    normalized_entries = {_normalize_entry(e) for e in control_entries}
    
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
    
    # Get data.tar.gz (with or without ./ prefix)
    data_tar = members.get('data.tar.gz') or members.get('./data.tar.gz')
    try:
        with tarfile.open(fileobj=io.BytesIO(data_tar), mode='r:gz') as tf:
            data_entries = tf.getnames()
    except Exception as e:
        log_fail(f"data.tar.gz is not a valid gzip archive: {e}")
        return False
    
    log_verbose("data.tar.gz: OK", verbose)
    
    normalized_data = {_normalize_entry(e) for e in data_entries}
    
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
        
        if entry.startswith('._'):
            continue
        
        normalized = _normalize_entry(entry)
        
        if normalized == 'opt':
            continue
        
        if entry.startswith('/'):
            log_fail(f"File writes outside /opt: {entry}")
            return False
        
        parts = normalized.split('/')
        if '..' in parts:
            log_fail(f"File writes outside /opt: {entry}")
            return False
        
        if not normalized.startswith('opt/'):
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
