"""Gzip tar archive creation for Entware-compatible opkg packages.

This module creates IPK packages using the Entware/OpenEmbedded outer gzip tar format,
as required by AsusWRT-Merlin Entware opkg.
"""

import gzip
import io
import tarfile
from typing import Dict


def create_gzip_tar_ipk(output_path: str, members: Dict[str, bytes]) -> None:
    """Create an IPK package with outer gzip tar (Entware-compatible format).
    
    The outer archive is a gzip-compressed tar containing:
      ./debian-binary
      ./data.tar.gz
      ./control.tar.gz
    
    This matches the format produced by Entware's ipkg-build.
    """
    tar_buffer = io.BytesIO()
    with tarfile.open(fileobj=tar_buffer, mode='w') as tf:
        for name, data in members.items():
            info = tarfile.TarInfo(name=name)
            info.size = len(data)
            tf.addfile(info, io.BytesIO(data))
    
    tar_content = tar_buffer.getvalue()
    
    with open(output_path, 'wb') as f:
        gzip_file = gzip.GzipFile(fileobj=f, mode='wb')
        gzip_file.write(tar_content)
        gzip_file.close()


def parse_gzip_tar_members(f) -> Dict[str, bytes]:
    """Parse a gzip tar archive and return a dict of member_name -> content.
    
    Args:
        f: A file-like object opened in binary mode
        
    Returns:
        Dict mapping member names to their byte content
    """
    members: Dict[str, bytes] = {}
    
    # Decompress gzip
    try:
        gzip_file = gzip.GzipFile(fileobj=f)
        content = gzip_file.read()
    except Exception as e:
        raise ValueError(f"Not a valid gzip archive: {e}")
    
    # Parse inner tar
    tar_buffer = io.BytesIO(content)
    with tarfile.open(fileobj=tar_buffer, mode='r') as tf:
        for member in tf.getmembers():
            if member.isfile():
                file_obj = tf.extractfile(member)
                if file_obj is not None:
                    members[member.name] = file_obj.read()
    
    return members
