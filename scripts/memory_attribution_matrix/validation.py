# validation.py — Validation checks

from typing import Tuple


def check_disabled_subsystem_leak(
    variant_name: str, 
    variant_config: dict,
    native_counts: dict
) -> Tuple[bool, str]:
    """
    Check if a disabled subsystem still emits native events (FAIL condition).
    Returns (is_leak, description).
    """
    if variant_config.get("disable_heartbeat") and native_counts.get("heartbeat", 0) > 0:
        return True, f"Heartbeat disabled but {native_counts['heartbeat']} heartbeat events emitted"
    
    if variant_config.get("disable_wg_checks") and native_counts.get("wireguard", 0) > 0:
        return True, f"WG checks disabled but {native_counts['wireguard']} WG events emitted"
    
    if variant_config.get("disable_bgp") and native_counts.get("bgp", 0) > 0:
        return True, f"BGP disabled but {native_counts['bgp']} BGP events emitted"
    
    if variant_config.get("disable_bfd") and native_counts.get("bfd", 0) > 0:
        return True, f"BFD disabled but {native_counts['bfd']} BFD events emitted"
    
    return False, ""
