// runtime/allocation_tracker.zig — public surface for the bounded-memory
// instrumentation. The implementation, concrete state struct, and all
// mutation/commit helpers live in `allocation_tracker_internal.zig` so
// importing files cannot reach into the accounting fields directly.
//
// `ReconnectMemoryState` is exposed as an opaque forward declaration. The
// only legal way to obtain a pointer to it is through
// `init(allocator)` (paired with `deinit`); the only legal way to obtain
// a classified allocator is through `trackingAllocator`; and the only
// legal way to commit a generation boundary is through
// `finishReconnectBoundary`.
//
// `ReconnectMemoryState` is the SINGLE AUTHORITY for connector-handle
// accounting. Connectors only produce a `ReconnectHandle` token. Every
// acquire MUST be routed through `adoptHandle(state, handle)` and every
// release through `releaseHandle(state, handle)`. There is no parallel
// `ConnectorProbe`, `HandleLedger`, or `HandleOracle` struct that can
// shadow the state's counters.
//
// The public surface intentionally does NOT re-export:
//   * `AllocationMetrics`, `OwnerMetrics`             — accounting shapes.
//   * `StateImpl`                                     — concrete state layout.
//   * `TrackingAllocator`                              — producer-side struct.
//
// The native repository gate (`make verify-allocation-tracker-imports`)
// rejects private or non-literal imports outside the `runtime/` package.
//
// Sibling files in this directory carry the larger components to keep this
// file under the LLM-friendliness hard limit:
//   * `allocation_tracker_internal.zig`            — opaque state + accessors
//                                                   + `trackingAllocator`
//                                                   + `adoptHandle`/
//                                                   `releaseHandle`/
//                                                   `finishReconnectBoundary`.
//   * `allocation_tracker_destroy.zig`             — `DestroyError` +
//                                                   `hasLiveLifetime` +
//                                                   `validateForDestroy`.
//   * `allocation_tracker_tracking_allocator.zig`  — producer-side
//                                                   `TrackingAllocator`
//                                                   (private; not re-exported).
//   * `allocation_tracker_connector_probe.zig`     — `ReconnectHandle` and
//                                                   `reconnect_handle_release_fn`.

const std = @import("std");
const internal = @import("allocation_tracker_internal.zig");
const destroy = @import("allocation_tracker_destroy.zig");
const snapshots = @import("allocation_tracker_snapshots.zig");
const connector_probe = @import("allocation_tracker_connector_probe.zig");

// ---------------------------------------------------------------------------
// Classification.
// ---------------------------------------------------------------------------

pub const AllocationOwner = internal.AllocationOwner;
pub const AllocationLifetime = internal.AllocationLifetime;
pub const num_owners = internal.num_owners;
pub const num_lifetimes = internal.num_lifetimes;

// ---------------------------------------------------------------------------
// Constants surfaced for the proof harness.
// ---------------------------------------------------------------------------

pub const warmup_generation_count = internal.AllocationMetrics.warmup_generation_count;

// ---------------------------------------------------------------------------
// Errors.
// ---------------------------------------------------------------------------

pub const LeakCheckError = internal.LeakCheckError;
pub const BoundaryError = internal.BoundaryError;
pub const ReconnectGenerationError = internal.ReconnectGenerationError;
pub const BackingAllocatorError = internal.BackingAllocatorError;
pub const HandleError = internal.HandleError;

// ---------------------------------------------------------------------------
// Bounded resource counters (kept public for targeted tests).
// ---------------------------------------------------------------------------

pub const BoundedResourceCounters = internal.BoundedResourceCounters;

// ---------------------------------------------------------------------------
// Opaque state. The only legal way to obtain a pointer is through
// `init(allocator)`; the only legal way to release it is through
// `deinit(state, allocator)`. The state owns its own handle oracle
// (`active_handle`, `handles_acquired`, `handles_released`) so external
// probes/ledgers cannot shadow the authoritative counters.
// ---------------------------------------------------------------------------

pub const ReconnectMemoryState = internal.ReconnectMemoryState;

// `validateForDestroy` and `DestroyError` live in
// `allocation_tracker_destroy.zig` (FA-3 final file split). The
// destroy-time contract is its own small module so the state
// file stays focused on mutation/commit helpers.
pub const validateForDestroy = destroy.validateForDestroy;
pub const DestroyError = destroy.DestroyError;

pub const init = internal.init;
pub const deinit = internal.deinit;

// ---------------------------------------------------------------------------
// Snapshot projections.
// ---------------------------------------------------------------------------

pub const MemorySnapshot = snapshots.MemorySnapshot;
pub const ResourceSnapshot = snapshots.ResourceSnapshot;
pub const HandleSnapshot = snapshots.HandleSnapshot;

// ---------------------------------------------------------------------------
// Connector seam: handles are produced by connectors and disposed of via
// the state's `adoptHandle` / `releaseHandle`.
// ---------------------------------------------------------------------------

pub const ReconnectHandle = connector_probe.ReconnectHandle;
pub const reconnect_handle_release_fn = connector_probe.reconnect_handle_release_fn;

// ---------------------------------------------------------------------------
// Bounded accessor functions on the opaque state. Snapshot/read-side
// accessors live in `allocation_tracker_snapshots.zig`; the boundary
// coordinator lives in `allocation_tracker_internal.zig`.
// ---------------------------------------------------------------------------

pub const finishReconnectBoundary = internal.finishReconnectBoundary;
pub const recordSocketOpen = snapshots.recordSocketOpen;
pub const recordSocketClose = snapshots.recordSocketClose;
pub const recordTimerStart = snapshots.recordTimerStart;
pub const recordTimerStop = snapshots.recordTimerStop;
pub const generation = snapshots.generation;
pub const liveBytesForLifetime = snapshots.liveBytesForLifetime;
pub const currentAllocationsForLifetime = snapshots.currentAllocationsForLifetime;
pub const baselineLiveBytes = snapshots.baselineLiveBytes;
pub const totalPeakBytes = snapshots.totalPeakBytes;
pub const memorySnapshot = snapshots.memorySnapshot;
pub const resourceSnapshot = snapshots.resourceSnapshot;
pub const handleSnapshot = snapshots.handleSnapshot;

// ---------------------------------------------------------------------------
// Authoritative handle lifecycle. These are the ONLY functions that may
// mutate the state's handle counters. Connectors do not write here.
// ---------------------------------------------------------------------------

pub const adoptHandle = internal.adoptHandle;
pub const releaseHandle = internal.releaseHandle;

// ---------------------------------------------------------------------------
// Factory: obtain a classified allocator through the opaque state.
// External code never sees the `TrackingAllocator` struct.
// ---------------------------------------------------------------------------

pub const trackingAllocator = internal.trackingAllocator;
