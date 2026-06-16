"""Fixture mutators for IPK self-testing."""

import io
import tarfile

from ar_parser import parse_ar_members, create_ar_archive
from ipk_tar_builder import create_gzip_tar_ipk, parse_gzip_tar_members
from ipk_fixture_builders import _build_control_tar, _build_data_tar


def _parse_outer_members(src: str) -> dict:
    """Parse outer archive members, supporting both ar and gzip tar formats.
    
    Normalizes member names by stripping leading ./
    """
    with open(src, 'rb') as f:
        magic = f.read(8)
        f.seek(0)
        if magic.startswith(b"!<arch>"):
            # Old ar format - shouldn't happen for new fixtures
            return parse_ar_members(f)
        else:
            # Gzip tar format (current)
            members = parse_gzip_tar_members(f)
            # Normalize names: strip leading ./
            normalized = {}
            for k, v in members.items():
                normalized[k.lstrip('./')] = v
            return normalized


def _write_ipk(dst: str, members: dict) -> None:
    """Write IPK package using gzip tar format with ./ prefix."""
    # Add ./ prefix for tar members
    prefixed = {f'./{k}': v for k, v in members.items()}
    create_gzip_tar_ipk(dst, prefixed)


def mod_missing_debian_binary(src: str, dst: str) -> None:
    """Modify fixture to be missing debian-binary."""
    members = _parse_outer_members(src)

    del members['debian-binary']
    _write_ipk(dst, members)


def mod_wrong_debian_binary(src: str, dst: str) -> None:
    """Modify fixture to have wrong debian-binary."""
    members = _parse_outer_members(src)

    members['debian-binary'] = b'3.0\n'
    _write_ipk(dst, members)


def mod_missing_control(src: str, dst: str) -> None:
    """Modify fixture to be missing control."""
    members = _parse_outer_members(src)

    empty_tar = io.BytesIO()
    with tarfile.open(fileobj=empty_tar, mode='w:gz') as tf:
        pass
    
    members['control.tar.gz'] = empty_tar.getvalue()
    _write_ipk(dst, members)


def mod_legacy_control(src: str, dst: str) -> None:
    """Modify fixture to use legacy CONTROL/control layout."""
    members = _parse_outer_members(src)

    control_content = """Package: uvb76
Version: 1.0.0-1
Architecture: aarch64-3.10
Maintainer: Test <test@test.com>
Description: Test
Section: net
Priority: optional
"""

    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='CONTROL/control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))
    
    members['control.tar.gz'] = buf.getvalue()
    _write_ipk(dst, members)


def mod_missing_postinst(src: str, dst: str) -> None:
    """Modify fixture to be missing postinst."""
    members = _parse_outer_members(src)

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

    members['control.tar.gz'] = _build_control_tar(control_content, prerm=prerm_content)
    _write_ipk(dst, members)


def mod_missing_prerm(src: str, dst: str) -> None:
    """Modify fixture to be missing prerm."""
    members = _parse_outer_members(src)

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

    members['control.tar.gz'] = _build_control_tar(control_content, postinst=postinst_content)
    _write_ipk(dst, members)


def mod_bad_package_name(src: str, dst: str) -> None:
    """Modify fixture to have bad package name."""
    members = _parse_outer_members(src)

    control_content = """Package: wrong-name
Version: 1.0.0-1
Architecture: aarch64-3.10
Maintainer: Test <test@test.com>
Description: Wrong package
Section: net
Priority: optional
"""

    members['control.tar.gz'] = _build_control_tar(control_content,
        postinst="#!/bin/sh\necho 'installed'\n",
        prerm="#!/bin/sh\necho 'removing'\n")
    _write_ipk(dst, members)


def mod_bad_architecture(src: str, dst: str) -> None:
    """Modify fixture to have bad architecture."""
    members = _parse_outer_members(src)

    control_content = """Package: uvb76
Version: 1.0.0-1
Architecture: unsupported-arch
Maintainer: Test <test@test.com>
Description: Bad arch
Section: net
Priority: optional
"""

    members['control.tar.gz'] = _build_control_tar(control_content,
        postinst="#!/bin/sh\necho 'installed'\n",
        prerm="#!/bin/sh\necho 'removing'\n")
    _write_ipk(dst, members)


def mod_outside_opt(src: str, dst: str) -> None:
    """Modify fixture to have payload outside /opt."""
    members = _parse_outer_members(src)

    members['data.tar.gz'] = _build_data_tar({'etc/uvb76.conf': ''})
    _write_ipk(dst, members)


def mod_absolute_path(src: str, dst: str) -> None:
    """Modify fixture to have absolute path in payload."""
    members = _parse_outer_members(src)

    members['data.tar.gz'] = _build_data_tar({'/opt/etc/uvb76.conf': ''})
    _write_ipk(dst, members)


def mod_dotdot_path(src: str, dst: str) -> None:
    """Modify fixture to have .. traversal in payload."""
    members = _parse_outer_members(src)

    members['data.tar.gz'] = _build_data_tar({'opt/../etc/uvb76.conf': ''})
    _write_ipk(dst, members)


def mod_non_exec_init(src: str, dst: str) -> None:
    """Modify fixture to have non-executable init script."""
    members = _parse_outer_members(src)

    members['data.tar.gz'] = _build_data_tar(init_mode=0o644)
    _write_ipk(dst, members)


def mod_missing_config(src: str, dst: str) -> None:
    """Modify fixture to be missing config file."""
    members = _parse_outer_members(src)

    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))

        init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))

    members['data.tar.gz'] = buf.getvalue()
    _write_ipk(dst, members)


def mod_rc_unslung(src: str, dst: str) -> None:
    """Modify fixture to have init script sourcing rc.unslung."""
    members = _parse_outer_members(src)

    init_content = "#!/bin/sh\n[ -f /opt/etc/init.d/rc.unslung ] && . /opt/etc/init.d/rc.unslung\n"
    members['data.tar.gz'] = _build_data_tar({'opt/etc/uvb76/uvb76.json.example': ''},
                                             init_content=init_content)
    _write_ipk(dst, members)


def mod_sha256_mismatch(src: str, dst: str) -> None:
    """Modify fixture to have mismatched SHA256."""
    members = _parse_outer_members(src)

    _write_ipk(dst, members)

    with open(dst + '.sha256', 'w') as f:
        f.write('0' * 64)


def mod_ar_outer_format(src: str, dst: str) -> None:
    """Modify fixture to use Debian/ar outer format (must fail for Entware).
    
    This creates a package with ar outer archive instead of gzip tar.
    Entware opkg on AsusWRT-Merlin requires gzip tar outer format.
    """
    members = _parse_outer_members(src)
    
    # Write as ar archive instead of gzip tar
    create_ar_archive(dst, members)
