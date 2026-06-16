"""Fixture mutators for IPK self-testing."""

import io
import tarfile

from ar_parser import parse_ar_members, create_ar_archive
from ipk_fixture_builders import _build_control_tar, _build_data_tar


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

    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='CONTROL/control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))

    members['control.tar.gz'] = buf.getvalue()
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

    members['control.tar.gz'] = _build_control_tar(control_content, prerm=prerm_content)
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

    members['control.tar.gz'] = _build_control_tar(control_content, postinst=postinst_content)
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

    members['control.tar.gz'] = _build_control_tar(control_content,
        postinst="#!/bin/sh\necho 'installed'\n",
        prerm="#!/bin/sh\necho 'removing'\n")
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

    members['control.tar.gz'] = _build_control_tar(control_content,
        postinst="#!/bin/sh\necho 'installed'\n",
        prerm="#!/bin/sh\necho 'removing'\n")
    create_ar_archive(dst, members)


def mod_outside_opt(src: str, dst: str) -> None:
    """Modify fixture to have payload outside /opt."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)

    members['data.tar.gz'] = _build_data_tar({'etc/uvb76.conf': ''})
    create_ar_archive(dst, members)


def mod_absolute_path(src: str, dst: str) -> None:
    """Modify fixture to have absolute path in payload."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)

    members['data.tar.gz'] = _build_data_tar({'/opt/etc/uvb76.conf': ''})
    create_ar_archive(dst, members)


def mod_dotdot_path(src: str, dst: str) -> None:
    """Modify fixture to have .. traversal in payload."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)

    members['data.tar.gz'] = _build_data_tar({'opt/../etc/uvb76.conf': ''})
    create_ar_archive(dst, members)


def mod_non_exec_init(src: str, dst: str) -> None:
    """Modify fixture to have non-executable init script."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)

    members['data.tar.gz'] = _build_data_tar(init_mode=0o644)
    create_ar_archive(dst, members)


def mod_missing_config(src: str, dst: str) -> None:
    """Modify fixture to be missing config file."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)

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
    create_ar_archive(dst, members)


def mod_rc_unslung(src: str, dst: str) -> None:
    """Modify fixture to have init script sourcing rc.unslung."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)

    init_content = "#!/bin/sh\n[ -f /opt/etc/init.d/rc.unslung ] && . /opt/etc/init.d/rc.unslung\n"
    members['data.tar.gz'] = _build_data_tar({'opt/etc/uvb76/uvb76.json.example': ''},
                                             init_content=init_content)
    create_ar_archive(dst, members)


def mod_sha256_mismatch(src: str, dst: str) -> None:
    """Modify fixture to have mismatched SHA256."""
    with open(src, 'rb') as f:
        members = parse_ar_members(f)

    create_ar_archive(dst, members)

    with open(dst + '.sha256', 'w') as f:
        f.write('0' * 64)
