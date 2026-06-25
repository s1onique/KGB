# verdict.py — Matrix verdict determination

from typing import Tuple, Dict, Any, List


def determine_matrix_verdict(results: dict) -> Tuple[str, str, dict]:
    """
    Determine the overall matrix verdict based on all variant results.
    
    Returns: (verdict, reason, details)
    
    Verdicts:
      - no_growth: No significant growth in any variant
      - bounded_warmup_or_allocator_highwater: Growth present but bounded across all variants
      - subsystem_correlated_growth: Specific subsystem correlates with growth
      - inconclusive: Cannot determine
    """
    variant_metrics = []
    
    for name, data in results.items():
        if not data.get("success"):
            continue
        
        verdict_data = data.get("verdict", {})
        native_counts = data.get("native_counts", {})
        
        growth = verdict_data.get("total_growth_kib", 0)
        steps = verdict_data.get("steps_detected", 0)
        rate = verdict_data.get("growth_rate_kib_per_min", 0)
        samples = verdict_data.get("samples_count", 0)
        
        variant_metrics.append({
            "name": name,
            "growth": growth,
            "steps": steps,
            "rate": rate,
            "samples": samples,
            "verdict": verdict_data.get("verdict", "unknown"),
            "owner": verdict_data.get("owner", ""),
            "native_counts": native_counts,
        })
    
    if not variant_metrics:
        return "inconclusive", "No successful variant runs", {}
    
    all_growths = [m["growth"] for m in variant_metrics]
    max_growth = max(all_growths) if all_growths else 0
    
    all_steps = [m["steps"] for m in variant_metrics]
    max_steps = max(all_steps) if all_steps else 0
    
    all_enabled_metric = next((m for m in variant_metrics if m["name"] == "all_enabled"), None)
    no_periodic_metric = next((m for m in variant_metrics if m["name"] == "no_periodic"), None)
    
    details = {
        "variants": variant_metrics,
        "max_growth_kib": max_growth,
        "max_steps": max_steps,
        "all_enabled_growth": all_enabled_metric["growth"] if all_enabled_metric else None,
        "no_periodic_growth": no_periodic_metric["growth"] if no_periodic_metric else None,
    }
    
    # Case 1: No growth anywhere
    if max_growth < 100 and max_steps < 3:
        return "no_growth", "No significant memory growth detected in any variant", details
    
    # Case 2: Growth in all_enabled but NOT in no_periodic
    if all_enabled_metric and no_periodic_metric:
        all_enabled_growth = all_enabled_metric["growth"]
        no_periodic_growth = no_periodic_metric["growth"]
        
        if all_enabled_growth > 200 and no_periodic_growth < 100:
            suspects = []
            
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "heartbeat_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("heartbeat")
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "wg_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("wireguard")
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "bgp_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("bgp")
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "bfd_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("bfd")
            
            if suspects:
                return "subsystem_correlated_growth", f"Growth correlates with subsystem(s): {', '.join(suspects)}", details
            else:
                return "subsystem_correlated_growth", "Growth correlates with periodic subsystems but specific owner unclear", details
        
        if no_periodic_growth > 200:
            return "bounded_warmup_or_allocator_highwater", f"Growth persists with all periodic paths disabled ({no_periodic_growth} KiB). Likely allocator warmup or other source.", details
    
    # Case 3: Growth in no_periodic but bounded
    if no_periodic_metric and no_periodic_metric["growth"] < 500 and no_periodic_metric["steps"] < 5:
        return "bounded_warmup_or_allocator_highwater", f"Growth is bounded even with all periodic paths disabled ({no_periodic_metric['growth']} KiB). Consistent with allocator warmup settling.", details
    
    # Case 4: Inconclusive
    return "inconclusive", "Cannot determine growth attribution from available variants", details
