"""AR archive parsing utilities for opkg package verification."""

from typing import BinaryIO, Dict


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
