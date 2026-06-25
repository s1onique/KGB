# variants.py — Matrix variant definitions

# Matrix variants configuration
MATRIX_VARIANTS = {
    "all_enabled": {
        "description": "All subsystems enabled (baseline)",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": False,
        "disable_bfd": False,
    },
    "heartbeat_disabled": {
        "description": "Heartbeat disabled",
        "disable_heartbeat": True,
        "disable_wg_checks": False,
        "disable_bgp": False,
        "disable_bfd": False,
    },
    "wg_disabled": {
        "description": "WireGuard checks disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": True,
        "disable_bgp": False,
        "disable_bfd": False,
    },
    "bgp_disabled": {
        "description": "BGP disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": True,
        "disable_bfd": False,
    },
    "bfd_disabled": {
        "description": "BFD disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": False,
        "disable_bfd": True,
    },
    "bgp_bfd_disabled": {
        "description": "BGP+BFD disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": True,
        "disable_bfd": True,
    },
    "no_periodic": {
        "description": "All periodic subsystems disabled (baseline for allocator/warmup)",
        "disable_heartbeat": True,
        "disable_wg_checks": True,
        "disable_bgp": True,
        "disable_bfd": True,
    },
}
