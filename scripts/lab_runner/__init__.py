# lab_runner package - modular idle staircase memory lab runner
from lab_runner.config import LabRunConfig, parse_args, LabError
from lab_runner.proc import require_linux_proc, read_proc_status
from lab_runner.artifacts import (
    write_tovarisch_config,
    print_tovarisch_config,
    write_manifest,
    write_memory_samples_header,
    write_event_timeline_header,
    append_event,
    append_memory_sample,
)
from lab_runner.tovarisch import start_tovarisch, wait_for_tovarisch_ready, terminate_process
from lab_runner.loop import run_idle_loop, run_status_burst
from lab_runner.analyzer import run_analyzer
from lab_runner.validation import verify_required_outputs, print_final_summary
from lab_runner.self_tests import run_self_tests
from lab_runner.main import main

__all__ = [
    "LabRunConfig",
    "LabError",
    "parse_args",
    "require_linux_proc",
    "read_proc_status",
    "write_tovarisch_config",
    "print_tovarisch_config",
    "write_manifest",
    "write_memory_samples_header",
    "write_event_timeline_header",
    "append_event",
    "append_memory_sample",
    "start_tovarisch",
    "wait_for_tovarisch_ready",
    "terminate_process",
    "run_idle_loop",
    "run_status_burst",
    "run_analyzer",
    "verify_required_outputs",
    "print_final_summary",
    "run_self_tests",
    "main",
]
