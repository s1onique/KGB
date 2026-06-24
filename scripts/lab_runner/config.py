# lab_runner/config.py - Configuration dataclass and argument parsing
import argparse
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional


class LabError(Exception):
    """Lab execution error with actionable diagnostics."""
    pass


@dataclass(frozen=True)
class LabRunConfig:
    """Configuration for a single idle staircase memory lab run."""
    duration: int
    interval: int
    run_id: str
    native_events: bool
    native_events_path: Optional[Path]
    disable_heartbeat: bool
    disable_wg_checks: bool
    disable_bgp: bool
    disable_bfd: bool
    strace: bool
    status_burst: bool
    repo_root: Path
    script_dir: Path
    artifact_root: Path
    artifact_path: Path
    tovarisch_binary: Path
    lab_bind: str
    lab_port: int
    heartbeat_enabled: bool
    wg_check_enabled: bool
    bgp_bfd_enabled: bool


def parse_args(argv: list[str]) -> LabRunConfig:
    """Parse command-line arguments into LabRunConfig."""
    parser = argparse.ArgumentParser(
        description="Idle staircase memory lab for tovarisch",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Shell-side synthetic event toggles (do NOT disable actual tovarisch paths):
  --heartbeat-only  Emit only heartbeat synthetic events
  --wg-only         Emit only WG check synthetic events
  --bgp-bfd-only    Emit only BGP/BFD synthetic events
  --no-subsystems   Suppress all synthetic events

Tovarisch-native event flags (emit events from real runtime paths):
  --native-events          Enable native event emission
  --native-events-path     Path for native_event_timeline.tsv (default: artifact dir)

Native runtime toggles (disable actual tovarisch periodic paths):
  --disable-heartbeat      Disable heartbeat runtime loop
  --disable-wg-checks      Disable WireGuard periodic checks
  --disable-bgp            Disable BGP maintenance/reconnect loop
  --disable-bfd            Disable BFD timer/tick loop

Native isolation modes (combinations of disable flags):
  --native-heartbeat-only  Enable heartbeat only, disable all other periodic paths
  --native-wg-only         Enable WG checks only, disable all other periodic paths
  --native-bgp-bfd-only    Enable BGP/BFD only, disable heartbeat and WG
  --native-no-periodic     Disable all periodic paths for baseline measurement
        """
    )
    parser.add_argument("--duration", type=int, default=600,
                        help="Lab duration in seconds (default: 600)")
    parser.add_argument("--interval", type=int, default=5,
                        help="Memory sample interval in seconds (default: 5)")
    parser.add_argument("--status-burst", action="store_true",
                        help="Run /status burst test after idle window")
    parser.add_argument("--strace", action="store_true",
                        help="Enable strace syscall tracing (Linux only)")
    parser.add_argument("--run-id", default="",
                        help="Custom run identifier (default: auto-generated)")
    parser.add_argument("--heartbeat-only", action="store_true",
                        help="Emit only heartbeat synthetic events")
    parser.add_argument("--wg-only", action="store_true",
                        help="Emit only WG check synthetic events")
    parser.add_argument("--bgp-bfd-only", action="store_true",
                        help="Emit only BGP/BFD synthetic events")
    parser.add_argument("--no-subsystems", action="store_true",
                        help="Suppress all synthetic events")
    parser.add_argument("--native-events", action="store_true",
                        help="Enable native event emission")
    parser.add_argument("--native-events-path",
                        help="Path for native_event_timeline.tsv")
    parser.add_argument("--native-no-periodic", action="store_true",
                        help="Disable all periodic paths for baseline measurement")
    parser.add_argument("--native-heartbeat-only", action="store_true",
                        help="Enable heartbeat only, disable all other periodic paths")
    parser.add_argument("--native-wg-only", action="store_true",
                        help="Enable WG checks only, disable all other periodic paths")
    parser.add_argument("--native-bgp-bfd-only", action="store_true",
                        help="Enable BGP/BFD only, disable heartbeat and WG")
    parser.add_argument("--disable-heartbeat", action="store_true",
                        help="Disable heartbeat runtime loop")
    parser.add_argument("--disable-wg-checks", action="store_true",
                        help="Disable WireGuard periodic checks")
    parser.add_argument("--disable-bgp", action="store_true",
                        help="Disable BGP maintenance/reconnect loop")
    parser.add_argument("--disable-bfd", action="store_true",
                        help="Disable BFD timer/tick loop")
    parser.add_argument("--self-test", action="store_true",
                        help="Run self-tests")

    args = parser.parse_args(argv)

    # Resolve paths - __file__ is scripts/lab_runner/config.py
    # package_dir = scripts/lab_runner
    # script_dir = scripts
    # repo_root = repo root
    package_dir = Path(__file__).resolve().parent
    script_dir = package_dir.parent
    repo_root = script_dir.parent
    artifact_root = repo_root / "artifacts" / "memory-labs" / "tovarisch" / "idle-staircase"
    tovarisch_binary = repo_root / "tovarisch" / "zig-out" / "bin" / "tovarisch"

    # Handle native isolation modes
    if args.native_heartbeat_only:
        native_events = True
        disable_heartbeat = False
        disable_wg_checks = True
        disable_bgp = True
        disable_bfd = True
    elif args.native_wg_only:
        native_events = True
        disable_heartbeat = True
        disable_wg_checks = False
        disable_bgp = True
        disable_bfd = True
    elif args.native_bgp_bfd_only:
        native_events = True
        disable_heartbeat = True
        disable_wg_checks = True
        disable_bgp = False
        disable_bfd = False
    elif args.native_no_periodic:
        native_events = True
        disable_heartbeat = True
        disable_wg_checks = True
        disable_bgp = True
        disable_bfd = True
    else:
        native_events = args.native_events
        disable_heartbeat = args.disable_heartbeat
        disable_wg_checks = args.disable_wg_checks
        disable_bgp = args.disable_bgp
        disable_bfd = args.disable_bfd

    # Handle synthetic event toggles
    if args.no_subsystems:
        heartbeat_enabled = False
        wg_check_enabled = False
        bgp_bfd_enabled = False
    elif args.heartbeat_only:
        heartbeat_enabled = True
        wg_check_enabled = False
        bgp_bfd_enabled = False
    elif args.wg_only:
        heartbeat_enabled = False
        wg_check_enabled = True
        bgp_bfd_enabled = False
    elif args.bgp_bfd_only:
        heartbeat_enabled = False
        wg_check_enabled = False
        bgp_bfd_enabled = True
    else:
        heartbeat_enabled = True
        wg_check_enabled = True
        bgp_bfd_enabled = True

    # Validate run_id
    run_id = args.run_id
    if not run_id:
        run_id = f"idle-{datetime.now(timezone.utc).strftime('%Y%m%d-%H%M%S')}-{__import__('os').getpid()}"
    if "/" in run_id or ".." in run_id or run_id == "":
        raise LabError(f"Invalid run_id: {run_id!r} (must not contain /, .., or be empty)")

    # Build native_events_path
    native_events_path = None
    if native_events:
        if args.native_events_path:
            native_events_path = Path(args.native_events_path)

    # Generate artifact path
    artifact_path = artifact_root / run_id

    # Set native_events_path to default artifact location if needed
    if native_events and native_events_path is None:
        native_events_path = artifact_path / "native_event_timeline.tsv"

    return LabRunConfig(
        duration=args.duration,
        interval=args.interval,
        run_id=run_id,
        native_events=native_events,
        native_events_path=native_events_path,
        disable_heartbeat=disable_heartbeat,
        disable_wg_checks=disable_wg_checks,
        disable_bgp=disable_bgp,
        disable_bfd=disable_bfd,
        strace=args.strace,
        status_burst=args.status_burst,
        repo_root=repo_root,
        script_dir=script_dir,
        artifact_root=artifact_root,
        artifact_path=artifact_path,
        tovarisch_binary=tovarisch_binary,
        lab_bind="127.0.0.1",
        lab_port=8317,
        heartbeat_enabled=heartbeat_enabled,
        wg_check_enabled=wg_check_enabled,
        bgp_bfd_enabled=bgp_bfd_enabled,
    )
