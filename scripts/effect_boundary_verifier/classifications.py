"""
Classifications module for effect boundary verifier.

Contains module classification sets and helper functions.
"""

import re
from typing import Set, Tuple


# =============================================================================
# Classification Table (hardcoded first version, can be extended to parse register)
# =============================================================================

# PURE modules: no effects allowed
PURE_MODULES: Set[str] = {
    "bgp/snapshot.zig",
    "bfd/snapshot.zig",
    "bgp/types.zig",
    "bgp/message.zig",
    "bgp/frame_decode.zig",
    "bgp/notification_decode.zig",
    "bgp/config_parse.zig",
    "bgp/validation.zig",
    "bgp/status.zig",
    "bgp/session_status.zig",
    "bfd/packet.zig",
    "bfd/config.zig",
    "status_query.zig",
    "status_bgp_diagnostics.zig",
    "net/linux_addr_parse.zig",
    "net/private_ip.zig",
    "net/rates.zig",
    "net/stat_formatter.zig",
    "net/ss_parser.zig",
    "net/wg_show_parser.zig",
    "net/interface_filter.zig",
    "net/network_diag_config.zig",
    "metrics_dto.zig",
    "config_parse_helpers.zig",
}

# BOUNDARY modules: effects allowed but must be documented
# These modules perform I/O but the effects are documented and bounded.
BOUNDARY_MODULES: Set[str] = {
    # Filesystem access - PURE-looking but actually touches sysfs
    "tunnel_check.zig",
    # Netlink socket operations - network I/O
    "net/linux_addr.zig",
    # Standard entry points
    "main.zig",
    "cli.zig",
    "http/routes.zig",
    "http/response.zig",
    "http/server.zig",
    "net/linux_read.zig",
    "net/safe_command.zig",
    "net/iptables.zig",
    "net/wg_status_boundary.zig",
    "net/wg_dump_collector.zig",
    "net/wg_show_collector.zig",
    "net/linux_interface_stats.zig",
    "net/linux_interfaces.zig",
    "net/linux_stats.zig",
    "net/inotify.zig",
    "status.zig",
    "status_network_diag.zig",
    "status_network_diag_tcp.zig",
    "logging.zig",
    "config.zig",
    "config_lab.zig",
    "metrics.zig",
    "metrics_state.zig",
    "build_info.zig",
}

# STATEFUL modules: own long-lived state
STATEFUL_MODULES: Set[str] = {
    "bgp/session.zig",
    "bgp/runtime.zig",
    "bgp/serve_integration.zig",
    "bgp/passive_listener.zig",
    "bgp/prefix_watch.zig",
    "bgp/prefix_file_loader.zig",
    "bfd/session.zig",
    "bfd/serve_integration.zig",
    "net/diag_event_ring.zig",
    "net/interface_sampler.zig",
    "runtime/uvb76_capture.zig",
}

# DEFERRED modules: classification unclear, report only
DEFERRED_MODULES: Set[str] = {
    "status_response.zig",
    "http/status_route_contract.zig",
}

# TEST module patterns
TEST_PATTERNS: list = [
    re.compile(r"_tests\.zig$"),
    re.compile(r"^test_"),
    re.compile(r"_test\.zig$"),
]


def is_test_file(path) -> bool:
    """
    Check if a file is a test file based on naming patterns.
    
    Args:
        path: Path object or path string
        
    Returns:
        True if the file matches test naming patterns
    """
    name = path.name if hasattr(path, 'name') else str(path)
    for pattern in TEST_PATTERNS:
        if pattern.search(name):
            return True
    return False


def classify_module(
    module_path: str,
    pure_set: Set[str],
    boundary_set: Set[str],
    stateful_set: Set[str],
    deferred_set: Set[str],
) -> str:
    """
    Classify a module based on the classification tables.
    
    Args:
        module_path: Path to the module
        pure_set: Set of PURE module paths
        boundary_set: Set of BOUNDARY module paths
        stateful_set: Set of STATEFUL module paths
        deferred_set: Set of DEFERRED module paths
        
    Returns:
        Classification string: PURE, BOUNDARY, STATEFUL, DEFERRED, or UNKNOWN
    """
    # Normalize path
    module_path = module_path.replace('tovarisch/src/', '')
    
    if module_path in pure_set:
        return "PURE"
    elif module_path in boundary_set:
        return "BOUNDARY"
    elif module_path in stateful_set:
        return "STATEFUL"
    elif module_path in deferred_set:
        return "DEFERRED"
    else:
        # Unknown modules are treated as DEFERRED for safety
        return "UNKNOWN"
