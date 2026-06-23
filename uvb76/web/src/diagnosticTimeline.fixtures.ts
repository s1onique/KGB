// Diagnostic Timeline Test Fixtures - Shared frontend test fixtures
import type { TimelineEvent, ProbeKindSummary, TimelineState } from './diagnosticTimeline.model';
import type { TimelineFilters } from './diagnosticTimeline.filters';
import type { SpikeResponseWithCaptures, DiagCapture, CaptureCooldownInfo, NetworkDiagData, TcpSocketDiagData, SpikeRetentionStats } from './api';

// ---------------------------------------------------------------------------
// Default Retention Stats
// ---------------------------------------------------------------------------

export const defaultRetention: SpikeRetentionStats = {
  retained_spike_count: 1,
  visible_spike_count: 1,
  protected_capture_count: 1,
  purge_eligible_count: 0,
  max_uncaptured_spikes: 200,
};

// ---------------------------------------------------------------------------
// Fixture Helpers
// ---------------------------------------------------------------------------

/** Create a minimal spike event */
export function createSpikeEvent(overrides: Partial<{
  event_id: string;
  target_id: string;
  kind: string;
  severity: string;
  latency_ms: number;
  sample_ts: string;
  collected_at: string;
  captures: DiagCapture[];
}> = {}): SpikeResponseWithCaptures['spikes'][0] {
  return {
    event_id: 'evt-1',
    target_id: 'test-target',
    kind: 'http',
    severity: 'warning',
    sample_ts: '2026-06-18T12:00:00Z',
    latency_ms: 1234,
    rolling_median_ms: 100,
    reasons: [],
    thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
    previous_samples: [],
    collected_at: '2026-06-18T12:00:00Z',
    ...overrides,
  };
}

/** Create a spike response */
export function createSpikeResponse(spike: ReturnType<typeof createSpikeEvent>, retention?: Partial<SpikeRetentionStats>): SpikeResponseWithCaptures {
  return {
    spikes: [spike],
    count: 1,
    retention: { ...defaultRetention, ...retention },
  };
}

/** Create an ok capture */
export function createOkCapture(overrides: Partial<DiagCapture> = {}): DiagCapture {
  return {
    source: 'peer-1',
    base_url: 'http://10.0.0.1:8080',
    capture_started_at: '2026-06-18T12:00:00Z',
    status: 'ok',
    ...overrides,
  };
}

/** Create an error capture */
export function createErrorCapture(overrides: Partial<DiagCapture> & { error: string }): DiagCapture {
  return {
    source: 'peer-1',
    base_url: 'http://10.0.0.1:8080',
    capture_started_at: '2026-06-18T12:00:00Z',
    status: 'error',
    error: 'connection refused',
    ...overrides,
  };
}

/** Create a TCP socket */
export function createTcpSocket(overrides: Partial<TcpSocketDiagData> = {}): TcpSocketDiagData {
  return {
    name: 'xray',
    state: 'ESTAB',
    local: 'redacted:443',
    remote: 'redacted:443',
    rtt_ms: 123.4,
    rto_ms: 456,
    retransmits: 7,
    unacked: 3,
    cwnd: 10,
    status: 'ok',
    ...overrides,
  };
}

/** Create network diag */
export function createNetworkDiag(overrides: Partial<NetworkDiagData> = {}): NetworkDiagData {
  return {
    started_at: '2026-06-18T12:00:00Z',
    status: 'ok',
    underlay_tcp: [],
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Cross-Probe Suppression Fixtures
// ---------------------------------------------------------------------------

/** Create cooldown info for ICMP→HTTP cross-probe suppression (hidden anchor) */
export function createIcmpToHttpCrossProbeCooldownInfo(overrides: Partial<CaptureCooldownInfo> = {}): CaptureCooldownInfo {
  return {
    scope: 'per_diagnostic_peer',
    last_successful_capture_at: '2026-06-18T11:00:00Z',
    next_capture_eligible_at: '2026-06-18T12:05:00Z',
    remaining_cooldown_ms: 300000,
    cooldown_key: 'peer-1',
    anchor_visible: false,
    anchor_artifact_visible: true,
    anchor_timeline_visible: false,
    anchor_visibility_reason: 'outside_filter_window',
    skipped_attempt_updates_cooldown: false,
    cooldown_seconds: 300,
    anchor_capture_id: 'icmp-anchor-event-001',
    anchor_target_id: 'test-target',
    anchor_probe_kind: 'icmp',
    anchor_source: 'peer-1',
    suppressed_probe_kind: 'http',
    is_cross_probe_suppression: true,
    ...overrides,
  };
}

/** Create cooldown info for ICMP→HTTP cross-probe suppression (visible anchor) */
export function createIcmpToHttpCrossProbeVisibleCooldownInfo(overrides: Partial<CaptureCooldownInfo> = {}): CaptureCooldownInfo {
  return {
    scope: 'per_diagnostic_peer',
    last_successful_capture_at: '2026-06-18T11:55:00Z',
    next_capture_eligible_at: '2026-06-18T12:00:00Z',
    remaining_cooldown_ms: 0,
    cooldown_key: 'peer-1',
    anchor_visible: true,
    anchor_artifact_visible: true,
    anchor_timeline_visible: true,
    anchor_visibility_reason: 'retained_visible',
    skipped_attempt_updates_cooldown: false,
    cooldown_seconds: 300,
    anchor_capture_id: 'icmp-anchor-event-001',
    anchor_target_id: 'test-target',
    anchor_probe_kind: 'icmp',
    anchor_source: 'peer-1',
    suppressed_probe_kind: 'http',
    is_cross_probe_suppression: true,
    ...overrides,
  };
}

/** Create a skipped cooldown capture with hidden anchor */
export function createSkippedCooldownCaptureWithHiddenAnchor(overrides: Partial<DiagCapture> = {}): DiagCapture {
  return {
    source: 'peer-1',
    base_url: 'http://10.0.0.1:8080',
    capture_started_at: '2026-06-18T12:00:00Z',
    status: 'ok',
    suppressed_by_cooldown: true,
    cooldown_info: createIcmpToHttpCrossProbeCooldownInfo(),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Spike Response Factories
// ---------------------------------------------------------------------------

/** Create spike response with captured status */
export function createCapturedSpikeResponse(overrides: Partial<{
  latency_ms?: number;
  probe_kind?: string;
  severity?: string;
}> = {}): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpikeEvent({
    kind: overrides.probe_kind ?? 'http',
    severity: overrides.severity ?? 'warning',
    latency_ms: overrides.latency_ms ?? 1234,
    captures: [createOkCapture({
      source: 'peer-1',
      duration_ms: 500,
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 50.0, rto_ms: 200, retransmits: 0 })],
      }),
    })],
  }));
}

/** Create spike response with suppressed/cooldown status */
export function createSuppressedSpikeResponse(overrides: Partial<{
  latency_ms?: number;
  probe_kind?: string;
  severity?: string;
}> = {}): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpikeEvent({
    kind: overrides.probe_kind ?? 'http',
    severity: overrides.severity ?? 'warning',
    latency_ms: overrides.latency_ms ?? 800,
    captures: [createSkippedCooldownCaptureWithHiddenAnchor()],
  }));
}

/** Create spike response with failed status */
export function createFailedSpikeResponse(overrides: Partial<{
  latency_ms?: number;
  probe_kind?: string;
  severity?: string;
  error?: string;
}> = {}): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpikeEvent({
    kind: overrides.probe_kind ?? 'http',
    severity: overrides.severity ?? 'critical',
    latency_ms: overrides.latency_ms ?? 5000,
    captures: [createErrorCapture({ error: overrides.error ?? 'connection refused' })],
  }));
}

/** Create spike response with cross-probe suppression */
export function createCrossProbeSuppressionResponse(): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpikeEvent({
    kind: 'http',
    severity: 'warning',
    latency_ms: 800,
    captures: [createSkippedCooldownCaptureWithHiddenAnchor({
      cooldown_info: createIcmpToHttpCrossProbeCooldownInfo(),
    })],
  }));
}

// ---------------------------------------------------------------------------
// Timeline Event Fixtures
// ---------------------------------------------------------------------------

/** Create a timeline event from API response */
export function createTimelineEvent(overrides: Partial<{
  eventId: string;
  targetId: string;
  probeKind: 'http' | 'icmp' | 'unknown';
  severity: 'warning' | 'critical' | 'unknown';
  latencyMs: number | null;
  sampleTs: string;
  collectedAt: string;
  captureStatus: 'captured' | 'suppressed' | 'failed' | 'not_attempted';
  canonicalTime: Date;
  dataStatus: 'ok' | 'malformed';
  malformedReasons: string[];
}> = {}): TimelineEvent {
  const eventId = overrides.eventId ?? 'evt-1';
  const probeKind = overrides.probeKind ?? 'http';
  const severity = overrides.severity ?? 'warning';
  const sampleTs = overrides.sampleTs ?? '2026-06-18T12:00:00Z';
  const canonicalTime = overrides.canonicalTime ?? new Date(sampleTs);
  
  return {
    eventId,
    targetId: overrides.targetId ?? 'test-target',
    probeKind,
    severity,
    latencyMs: overrides.latencyMs ?? 1234,
    sampleTs,
    collectedAt: overrides.collectedAt ?? sampleTs,
    reasons: [],
    rollingMedianMs: 100,
    thresholds: {
      warningMs: 500,
      criticalMs: 1000,
      relativeMultiplier: 10,
    },
    captures: [],
    primaryCapture: null,
    captureStatus: overrides.captureStatus ?? 'captured',
    canonicalTimeMs: canonicalTime.getTime(),
    timeStatus: 'ok',
    sortProbeKind: probeKind === 'http' ? 0 : probeKind === 'icmp' ? 1 : 2,
    sortSeverity: severity === 'warning' ? 0 : severity === 'critical' ? 1 : 2,
    sortEventId: eventId,
    dataStatus: overrides.dataStatus ?? 'ok',
    malformedReasons: overrides.malformedReasons ?? [],
  };
}

/** Create an HTTP timeline event */
export function createHttpTimelineEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  return createTimelineEvent({
    probeKind: 'http',
    ...overrides,
  });
}

/** Create an ICMP timeline event */
export function createIcmpTimelineEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  return createTimelineEvent({
    probeKind: 'icmp',
    ...overrides,
  });
}

/** Create a critical timeline event */
export function createCriticalTimelineEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  return createTimelineEvent({
    severity: 'critical',
    latencyMs: 5000,
    ...overrides,
  });
}

// ---------------------------------------------------------------------------
// Timeline State Fixtures
// ---------------------------------------------------------------------------

/** Create an empty timeline state */
export function createEmptyTimelineState(): TimelineState {
  return {
    httpResponse: null,
    icmpResponse: null,
    httpEvents: [],
    icmpEvents: [],
    mergedEvents: [],
    httpSummary: {
      probeKind: 'http',
      totalEvents: 0,
      capturedCount: 0,
      suppressedCount: 0,
      failedCount: 0,
      criticalCount: 0,
      warningCount: 0,
    },
    icmpSummary: {
      probeKind: 'icmp',
      totalEvents: 0,
      capturedCount: 0,
      suppressedCount: 0,
      failedCount: 0,
      criticalCount: 0,
      warningCount: 0,
    },
    isLoading: false,
    error: null,
  };
}

/** Create a loading timeline state */
export function createLoadingTimelineState(): TimelineState {
  const state = createEmptyTimelineState();
  state.isLoading = true;
  return state;
}

/** Create a timeline state with events */
export function createTimelineStateWithEvents(events: TimelineEvent[]): TimelineState {
  const httpEvents = events.filter(e => e.probeKind === 'http');
  const icmpEvents = events.filter(e => e.probeKind === 'icmp');
  
  return {
    httpResponse: null,
    icmpResponse: null,
    httpEvents,
    icmpEvents,
    mergedEvents: events,
    httpSummary: {
      probeKind: 'http',
      totalEvents: httpEvents.length,
      capturedCount: httpEvents.filter(e => e.captureStatus === 'captured').length,
      suppressedCount: httpEvents.filter(e => e.captureStatus === 'suppressed').length,
      failedCount: httpEvents.filter(e => e.captureStatus === 'failed').length,
      criticalCount: httpEvents.filter(e => e.severity === 'critical').length,
      warningCount: httpEvents.filter(e => e.severity === 'warning').length,
    },
    icmpSummary: {
      probeKind: 'icmp',
      totalEvents: icmpEvents.length,
      capturedCount: icmpEvents.filter(e => e.captureStatus === 'captured').length,
      suppressedCount: icmpEvents.filter(e => e.captureStatus === 'suppressed').length,
      failedCount: icmpEvents.filter(e => e.captureStatus === 'failed').length,
      criticalCount: icmpEvents.filter(e => e.severity === 'critical').length,
      warningCount: icmpEvents.filter(e => e.severity === 'warning').length,
    },
    isLoading: false,
    error: null,
  };
}

// ---------------------------------------------------------------------------
// Filter Fixtures
// ---------------------------------------------------------------------------

/** Default filters */
export const defaultTestFilters: TimelineFilters = {
  probeKind: 'all',
  captureStatus: 'all',
  severity: 'all',
};

/** HTTP-only filter */
export const httpOnlyFilter: TimelineFilters = {
  probeKind: 'http',
  captureStatus: 'all',
  severity: 'all',
};

/** Captured-only filter */
export const capturedOnlyFilter: TimelineFilters = {
  probeKind: 'all',
  captureStatus: 'captured',
  severity: 'all',
};

/** Critical-only filter */
export const criticalOnlyFilter: TimelineFilters = {
  probeKind: 'all',
  captureStatus: 'all',
  severity: 'critical',
};
