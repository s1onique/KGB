"""Fixture builders for IPK self-testing."""

import hashlib
import io
import os
import tarfile

from ipk_tar_builder import create_gzip_tar_ipk


def _build_control_tar(control_content: str, postinst: str = None,
                       prerm: str = None) -> bytes:
    """Build a control.tar.gz with given content."""
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='control')
        info.size = len(control_content.encode())
        tf.addfile(info, io.BytesIO(control_content.encode()))

        if postinst:
            info = tarfile.TarInfo(name='postinst')
            info.size = len(postinst.encode())
            info.mode = 0o755
            tf.addfile(info, io.BytesIO(postinst.encode()))

        if prerm:
            info = tarfile.TarInfo(name='prerm')
            info.size = len(prerm.encode())
            info.mode = 0o755
            tf.addfile(info, io.BytesIO(prerm.encode()))
    return buf.getvalue()


def _build_data_tar(extra_files=None, init_mode=0o755, init_content=None) -> bytes:
    """Build a data.tar.gz with required files plus extras."""
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode='w:gz') as tf:
        info = tarfile.TarInfo(name='opt/bin/uvb76')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))

        info = tarfile.TarInfo(name='opt/etc/uvb76/uvb76.json.example')
        info.size = 0
        tf.addfile(info, io.BytesIO(b''))

        if init_content is None:
            init_content = "#!/bin/sh\necho 'init'\n"
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = init_mode
        tf.addfile(info, io.BytesIO(init_content.encode()))

        if extra_files:
            for fname, fcontent in extra_files.items():
                info = tarfile.TarInfo(name=fname)
                info.size = len(fcontent) if fcontent else 0
                tf.addfile(info, io.BytesIO(fcontent.encode() if fcontent else b''))
    return buf.getvalue()


def create_good_fixture(work_dir: str) -> str:
    """Create a valid ipk fixture using gzip tar format. Returns the fixture path."""
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

        # Valid rc.func contract init script
        init_content = """#!/bin/sh
# Test init script
ENABLED=no
PROCS=uvb76
ARGS="-config /opt/etc/uvb76/uvb76.json"
PREARGS=""
DESC=$PROCS
PATH=/opt/bin:/opt/sbin:/usr/bin:/usr/sbin:/bin:/sbin
. /opt/etc/init.d/rc.func
"""
        info = tarfile.TarInfo(name='opt/etc/init.d/S76uvb76')
        info.size = len(init_content.encode())
        info.mode = 0o755
        tf.addfile(info, io.BytesIO(init_content.encode()))

    data_tar = data_tar_buffer.getvalue()

    # Use Entware-compatible outer gzip tar format
    members = {
        './debian-binary': b'2.0\n',
        './control.tar.gz': control_tar,
        './data.tar.gz': data_tar
    }
    create_gzip_tar_ipk(fixture_path, members)

    with open(fixture_path, 'rb') as f:
        sha256 = hashlib.sha256(f.read()).hexdigest()
    with open(fixture_path + '.sha256', 'w') as f:
        f.write(sha256)

    return fixture_path
