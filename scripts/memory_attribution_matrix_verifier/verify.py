# verify.py — Matrix verification logic

import re
from pathlib import Path
from typing import Tuple

from .checks import CANONICAL_VARIANTS, verify_variant, read_file


def read_matrix_manifest(matrix_root):
    manifest_path = matrix_root / "matrix-manifest.yaml"
    if not manifest_path.exists():
        return None
    content = manifest_path.read_text()
    variants = []
    for line in content.split('\n'):
        line = line.strip()
        if line.startswith('- '):
            variant = line[2:].strip()
            if variant:
                variants.append(variant)
    return variants if variants else CANONICAL_VARIANTS


def verify_matrix(matrix_root):
    errors = []
    results = {}
    
    summary = read_file(matrix_root / "matrix-summary.md")
    if not summary:
        errors.append("matrix-summary.md missing")
    
    declared_variants = read_matrix_manifest(matrix_root)
    
    missing_variants = []
    for variant in declared_variants:
        variant_path = matrix_root / variant
        if not variant_path.exists():
            missing_variants.append(variant)
    
    if missing_variants:
        errors.append(f"Missing declared variant directories: {', '.join(missing_variants)}")
    
    if errors:
        return False, "; ".join(errors), results
    
    all_valid = True
    for variant in declared_variants:
        variant_path = matrix_root / variant
        valid, error, data = verify_variant(variant_path, variant)
        results[variant] = {"valid": valid, "error": error, "data": data}
        if not valid:
            all_valid = False
    
    if not all_valid:
        failed = [v for v, r in results.items() if not r["valid"]]
        return False, f"Variant verification failed: {', '.join(failed)}", results
    
    if summary:
        verdict_match = re.search(r'\*\*Overall Verdict\*\*\s*\n\s*\*\*(\w+)\*\*', summary)
        if verdict_match:
            verdict = verdict_match.group(1).strip().lower()
            valid_verdicts = ["no_growth", "bounded_warmup_or_allocator_highwater", 
                             "subsystem_correlated_growth", "inconclusive"]
            if verdict not in valid_verdicts:
                return False, f"Invalid matrix verdict: {verdict}", results
    
    return True, "", results
