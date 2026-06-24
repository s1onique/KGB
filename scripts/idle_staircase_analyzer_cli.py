#!/usr/bin/env python3
# idle_staircase_analyzer_cli.py — CLI wrapper for idle_staircase_analyzer
from pathlib import Path
from idle_staircase_analyzer import analyze

if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description="Analyze idle staircase memory lab artifacts")
    parser.add_argument("memory_samples", help="Path to memory_samples.tsv")
    parser.add_argument("artifact_dir", help="Path to artifact directory")
    parser.add_argument("event_timeline", help="Path to event_timeline.tsv")
    parser.add_argument("--duration", type=int, default=600, help="Lab duration in seconds")
    parser.add_argument("--heartbeat-enabled", default="true", help="Heartbeat enabled")
    parser.add_argument("--wg-enabled", default="true", help="WG check enabled")
    parser.add_argument("--bgp-bfd-enabled", default="true", help="BGP/BFD enabled")
    parser.add_argument("--native-events", default="false", help="Native events enabled")
    parser.add_argument("--native-event-timeline", help="Path to native_event_timeline.tsv")
    
    args = parser.parse_args()
    
    verdict = analyze(
        Path(args.memory_samples),
        Path(args.artifact_dir),
        Path(args.event_timeline),
        args.duration,
        args.heartbeat_enabled.lower() == "true",
        args.wg_enabled.lower() == "true",
        args.bgp_bfd_enabled.lower() == "true",
        args.native_events.lower() == "true",
        Path(args.native_event_timeline) if args.native_event_timeline else None,
    )
    print(verdict)
