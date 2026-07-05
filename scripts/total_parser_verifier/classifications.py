# classifications.py -- Module classifications for total parser register
"""Classifications for tovarisch external-input parser modules.

Classification categories:
- TOTAL: Parser modules with total public API (no panics, no unreachable)
- BOUNDARY_TOTAL: Boundary adapters with total external API
- STATEFUL_ADAPTER: Stateful protocol adapters
- DEFERRED: Known partial behavior, cannot fix in current ACT
- TEST: Test-only files, excluded from production checks
"""

from enum import Enum
from typing import List


class Classification(Enum):
    """Classification categories for parser modules."""
    TOTAL = "TOTAL"
    BOUNDARY_TOTAL = "BOUNDARY_TOTAL"
    STATEFUL_ADAPTER = "STATEFUL_ADAPTER"
    DEFERRED = "DEFERRED"
    TEST = "TEST"


# TOTAL modules: All public functions are total
# No @panic, unreachable, .? unwrap, or @enumFromInt without bounds
# Note: Some TOTAL modules have accepted .? patterns after null checks
TOTAL_MODULES = {
    "status_query.zig",
    "config_parse_helpers.zig",
    "bgp/frame_decode.zig",
    "bgp/notification_decode.zig",
    "net/private_ip.zig",
    "net/interface_filter.zig",
}


# BOUNDARY_TOTAL modules: External API is total, internal may vary
BOUNDARY_TOTAL_MODULES = {
    "config.zig",
    "config_lab.zig",
    "net/linux_read.zig",
    "net/linux_stats.zig",
    "net/linux_interface_stats.zig",
    "net/safe_command.zig",
    "net/wg_dump_collector.zig",
    "net/wg_show_collector.zig",
    "cli.zig",
    "http/routes.zig",
}


# STATEFUL_ADAPTER modules: Stateful protocol adapters
STATEFUL_ADAPTER_MODULES = {
    "bgp/session.zig",
    "bfd/session.zig",
    "bfd/status.zig",
}


# DEFERRED modules: Known partial behavior, reports but doesn't fail
# bgp/runtime.zig: unreachable in formatPeerAddr (line 56) is defensive
#   - Formats fixed-size [4]u8 peer address to fixed 32-byte buffer
#   - IPv4 needs at most 16 bytes; buffer is 32 bytes
#   - catch unreachable handles theoretically impossible fmt error
#   - This is logging infrastructure, not external input parsing
DEFERRED_MODULES = {
    "bgp/runtime.zig",
}


# Combined registry: all modules that should be checked
ALL_PARSER_MODULES = (
    TOTAL_MODULES |
    BOUNDARY_TOTAL_MODULES |
    STATEFUL_ADAPTER_MODULES |
    DEFERRED_MODULES
)


def get_module_classification(module: str) -> Classification:
    """Get the classification for a module."""
    if module in TOTAL_MODULES:
        return Classification.TOTAL
    if module in BOUNDARY_TOTAL_MODULES:
        return Classification.BOUNDARY_TOTAL
    if module in STATEFUL_ADAPTER_MODULES:
        return Classification.STATEFUL_ADAPTER
    if module in DEFERRED_MODULES:
        return Classification.DEFERRED
    raise ValueError(f"Unknown module: {module}")


def get_all_registered_modules() -> List[str]:
    """Get all registered module names."""
    return sorted(ALL_PARSER_MODULES)


def is_total_module(module: str) -> bool:
    """Check if module is classified as TOTAL."""
    return module in TOTAL_MODULES


def is_boundary_total_module(module: str) -> bool:
    """Check if module is classified as BOUNDARY_TOTAL."""
    return module in BOUNDARY_TOTAL_MODULES


def is_stateful_adapter(module: str) -> bool:
    """Check if module is classified as STATEFUL_ADAPTER."""
    return module in STATEFUL_ADAPTER_MODULES


def is_deferred(module: str) -> bool:
    """Check if module is classified as DEFERRED."""
    return module in DEFERRED_MODULES


def requires_strict_check(module: str) -> bool:
    """Check if module requires strict (fail on violations) checking."""
    return is_total_module(module) or is_boundary_total_module(module)


def get_module_description(module: str) -> str:
    """Get a description for a module."""
    descriptions = {
        "status_query.zig": "HTTP query parameter parsing",
        "config_parse_helpers.zig": "Config value parsing helpers",
        "bgp/config_parse.zig": "BGP configuration parsing",
        "bgp/frame_decode.zig": "BGP wire frame decoding",
        "bgp/notification_decode.zig": "BGP NOTIFICATION decoding",
        "bfd/packet.zig": "BFD packet encode/decode",
        "net/ss_parser.zig": "ss command output parsing",
        "net/wg_show_parser.zig": "wg show output parsing",
        "net/linux_addr_parse.zig": "rtnetlink address parsing",
        "net/private_ip.zig": "IPv4 address classification",
        "net/interface_filter.zig": "Interface filtering",
        "config.zig": "INI config file parsing",
        "config_lab.zig": "Lab config parsing",
        "net/linux_read.zig": "Linux sysfs/procfs read boundary",
        "net/linux_stats.zig": "Linux interface statistics",
        "net/linux_interface_stats.zig": "Interface stats collection",
        "net/linux_interfaces.zig": "Interface enumeration",
        "net/safe_command.zig": "Safe command execution",
        "net/wg_dump_collector.zig": "wg dump collection",
        "net/wg_show_collector.zig": "wg show collection",
        "cli.zig": "CLI argument parsing",
        "http/routes.zig": "HTTP route handling",
        "bgp/session.zig": "BGP session FSM",
        "bgp/runtime.zig": "BGP runtime",
        "bfd/session.zig": "BFD session FSM",
        "bfd/status.zig": "BFD status handling",
    }
    return descriptions.get(module, "External input parser")
