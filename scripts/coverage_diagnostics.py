"""coverage_diagnostics.py — DWARF and kcov report diagnostics

Provides diagnostic functions for coverage analysis:
- print_dwarf_diagnostics: Binary DWARF analysis for source path mapping
- print_report_diagnostics: Report file inspection for failed kcov attempts

Used by coverage_gate.py.
"""

import sys
import re
import subprocess
from pathlib import Path


def print_dwarf_diagnostics(binary: Path) -> bool:
    """Run DWARF diagnostics to check binary for project source paths.
    
    Returns True if project paths were found in DWARF, False otherwise.
    """
    print("")
    print("[DWARF-DIAGNOSTIC] === Binary analysis for source mapping ===")
    
    # File type
    result = subprocess.run(["file", str(binary)], capture_output=True, text=True)
    print(f"[DWARF-DIAGNOSTIC] file type:")
    print(f"[DWARF-DIAGNOSTIC] {result.stdout.strip()}")
    
    # Debug sections with readelf
    readelf_available = subprocess.run(["which", "readelf"], capture_output=True, text=True)
    if readelf_available.returncode == 0:
        result = subprocess.run(
            ["readelf", "-S", str(binary)],
            capture_output=True, text=True
        )
        debug_lines = [l for l in result.stdout.split("\n") 
                      if "debug" in l.lower() or "symtab" in l.lower()]
        if debug_lines:
            print(f"[DWARF-DIAGNOSTIC] debug sections:")
            for line in debug_lines[:10]:
                print(f"[DWARF-DIAGNOSTIC] {line.strip()}")
        else:
            print("[DWARF-DIAGNOSTIC] no debug/symtab sections found")
    else:
        print("[DWARF-DIAGNOSTIC] readelf not available — skipping section listing")
    
    # Check for project paths in DWARF line tables
    print("[DWARF-DIAGNOSTIC] checking for project source paths in DWARF line tables...")
    
    project_paths = []
    
    # Compile patterns once (non-capturing groups, scan line-by-line)
    patterns = [
        re.compile(r"tovari?sch/src", re.I),
        re.compile(r"src/(?:main|cli|status|http)", re.I),
        re.compile(r"commands\.zig", re.I),
        re.compile(r"status\.zig", re.I),
    ]
    
    # Try readelf first - scan line-by-line without capturing groups
    if readelf_available.returncode == 0:
        result = subprocess.run(
            ["readelf", "--debug-dump=decodedline", str(binary)],
            capture_output=True, text=True
        )
        if result.returncode == 0:
            # Scan each line for matches (not using findall with groups)
            for line in result.stdout.splitlines():
                if any(p.search(line) for p in patterns):
                    project_paths.append(line.strip())
                    if len(project_paths) >= 50:
                        break
    
    # Fallback to llvm-dwarfdump if readelf didn't find anything
    if not project_paths:
        llvm_dwarfdump = subprocess.run(["which", "llvm-dwarfdump"], capture_output=True, text=True)
        if llvm_dwarfdump.returncode == 0:
            result = subprocess.run(
                ["llvm-dwarfdump", "--debug-line", str(binary)],
                capture_output=True, text=True
            )
            if result.returncode == 0:
                for line in result.stdout.splitlines():
                    if any(p.search(line) for p in patterns):
                        project_paths.append(line.strip())
                        if len(project_paths) >= 50:
                            break
    
    if project_paths:
        print("[DWARF-DIAGNOSTIC] FOUND project paths in DWARF line tables:")
        for path in project_paths[:50]:
            print(f"[DWARF-DIAGNOSTIC] {path}")
        print(f"[DWARF-DIAGNOSTIC] project path match count: {len(project_paths)}")
        found_paths = True
    else:
        print("[DWARF-DIAGNOSTIC] WARNING: no project source paths found in DWARF line tables")
        print("[DWARF-DIAGNOSTIC] This suggests Zig compiled the tests without debug-line info")
        # Show sample of what is in the DWARF
        if readelf_available.returncode == 0:
            result = subprocess.run(
                ["readelf", "--debug-dump=decodedline", str(binary)],
                capture_output=True, text=True
            )
            if result.stdout:
                lines = result.stdout.split("\n")[:20]
                print("[DWARF-DIAGNOSTIC] Sample of actual DWARF paths (first 20 lines):")
                for line in lines:
                    print(f"[DWARF-DIAGNOSTIC] {line}")
        else:
            print("[DWARF-DIAGNOSTIC] readelf not available for sample")
        found_paths = False
    
    print("[DWARF-DIAGNOSTIC] === End binary analysis ===")
    print("")
    
    return found_paths


def check_dwarf_has_paths(binary: Path) -> bool:
    """Check if DWARF contains project source paths (quick check for main())."""
    readelf_available = subprocess.run(["which", "readelf"], capture_output=True, text=True)
    if readelf_available.returncode != 0:
        return False
    
    result = subprocess.run(
        ["readelf", "--debug-dump=decodedline", str(binary)],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        return False
    
    patterns = [
        re.compile(r"tovari?sch/src", re.I),
        re.compile(r"src/(?:main|cli|status|http)", re.I),
        re.compile(r"commands\.zig", re.I),
        re.compile(r"status\.zig", re.I),
    ]
    
    for line in result.stdout.splitlines():
        if any(p.search(line) for p in patterns):
            return True
    
    return False


def print_report_diagnostics(coverage_dir: Path, find_coverage_dir) -> None:
    """Print diagnostic info for a failed kcov attempt.
    
    Args:
        coverage_dir: The parent output directory passed to kcov
        find_coverage_dir: Function to locate the actual kcov report subdir
    """
    print("")
    
    # Find actual kcov output dir using find_coverage_dir
    kcov_dir = find_coverage_dir(str(coverage_dir))
    
    if kcov_dir is None:
        print("[coverage-debug] Report files NOT FOUND (kcov did not produce output)")
        print(f"[coverage-debug] Contents of {coverage_dir}:")
        try:
            contents = list(coverage_dir.iterdir())
            for c in contents[:20]:
                print(f"  [coverage-debug] {c.name}")
        except:
            pass
        return
    
    print(f"[coverage-debug] Found kcov output directory: {kcov_dir.name}")
    
    # Print sizes from actual report directory
    print("")
    print("[coverage-debug] Report file sizes:")
    
    reports = ["coverage.json", "cobertura.xml", "cov.xml", "codecov.json"]
    for name in reports:
        path = kcov_dir / name
        if path.exists():
            size = path.stat().st_size
            print(f"  [coverage-debug]   {name}: {size} bytes")
        else:
            print(f"  [coverage-debug]   {name}: not found")
    
    # Print sample of coverage.json
    cj = kcov_dir / "coverage.json"
    if cj.exists():
        print("")
        print("[coverage-debug] coverage.json sample (first 20 lines):")
        lines = cj.read_text().split("\n")[:20]
        for line in lines:
            print(f"[coverage-debug] {line}")
    
    # Print sample of cobertura.xml
    cx = kcov_dir / "cobertura.xml"
    if cx.exists():
        print("")
        print("[coverage-debug] cobertura.xml sample (first 20 lines):")
        lines = cx.read_text().split("\n")[:20]
        for line in lines:
            print(f"[coverage-debug] {line}")
    
    # Cap file tree output
    print("")
    print("[coverage-debug] kcov output directory contents (capped at 100 files):")
    files = list(kcov_dir.rglob("*"))
    files = [f for f in files if f.is_file()]
    fcount = len(files)
    print(f"  [coverage-debug] total files: {fcount}")
    
    # Show capped list
    to_show = sorted(files, key=lambda f: str(f))[:100]
    for f in to_show:
        rel = f.relative_to(kcov_dir)
        print(f"  [coverage-debug] file: {rel}")
    
    if fcount > 100:
        print(f"  [coverage-debug] ... and {fcount - 100} more files (output capped)")
