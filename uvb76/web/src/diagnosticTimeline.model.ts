// Diagnostic Timeline Model - Fetch and normalize API responses
import { api, type SpikeResponseWithCaptures, type SpikeEventWithCaptures, type DiagCapture, type SpikeRetentionStats, type CaptureCooldownInfo } from './api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Probe kind types */
export type ProbeKind = 'http' | 'icmp';

/** Severity levels */
export type Severity = 'warning' | 'critical';

/** Capture status for display - maps backend statuses to operator-friendly categories */
export type CaptureStatusDisplay = 'captured' | 'suppressed' | 'failed' | 'not_attempted';

/** Unified timeline event combining HTTP and ICMP spike events */
export interface TimelineEvent {
  // Source data
  eventId: string;
  targetId: string;
  probeKind: ProbeKind;
  severity: Severity;
  latencyMs: number;
  sampleTs: string;
  collectedAt: string;
  reasons: string[];
  rollingMedianMs: number;
  thresholds: {
    warningMs: number;
    criticalMs: number;
    relativeMultiplier: number;
  };
  
  // Capture data
  captures: DiagCapture[];
  primaryCapture: DiagCapture | null;
  captureStatus: CaptureStatusDisplay;
  
  // Canonical event time - deterministic selector for sorting
  canonicalTime: Date;
  
  // Stable sort keys
  sortProbeKind: number;  // http=0, icmp=1
  sortSeverity: number;    // warning=0, critical=1
  sortEventId: string;     // event id for stable tie-break
}

/** Summary card for a probe kind */
export interface ProbeKindSummary {
  probeKind: ProbeKind;
  totalEvents: number;
  capturedCount: number;
  suppressedCount: number;
  failedCount: number;
  criticalCount: number;
  warningCount: number;
}

/** Timeline state after fetching and normalizing */
export interface TimelineState {
  httpResponse: SpikeResponseWithCaptures | null;
  icmpResponse: SpikeResponseWithCaptures | null;
  httpEvents: TimelineEvent[];
  icmpEvents: TimelineEvent[];
  mergedEvents: TimelineEvent[];
  httpSummary: ProbeKindSummary;
  icmpSummary: ProbeKindSummary;
  isLoading: boolean;
  error: string | null;
}

// ---------------------------------------------------------------------------
// Canonical Time Selector
// ---------------------------------------------------------------------------

/** 
 * Get canonical event time using deterministic selector:
 * 1. sample timestamp if present
 * 2. collected/created timestamp if present
 * 3. capture started timestamp if present
 * 4. capture finished timestamp if present
 * 5. stable fallback (epoch)
 */
function getCanonicalTime(spike: SpikeEventWithCaptures): Date {
  // 1. sample timestamp
  if (spike.sample_ts) {
    const d = new Date(spike.sample_ts);
    if (!isNaN(d.getTime())) return d;
  }
  
  // 2. collected timestamp
  if (spike.collected_at) {
    const d = new Date(spike.collected_at);
    if (!isNaN(d.getTime())) return d;
  }
  
  // 3. capture started timestamp
  if (spike.captures && spike.captures.length > 0) {
    const capture = spike.captures[0];
    if (capture.capture_started_at) {
      const d = new Date(capture.capture_started_at);
      if (!isNaN(d.getTime())) return d;
    }
    
    // 4. capture finished timestamp
    if (capture.capture_finished_at) {
      const d = new Date(capture.capture_finished_at);
      if (!isNaN(d.getTime())) return d;
    }
  }
  
  // 5. stable fallback
  return new Date(0);
}

// ---------------------------------------------------------------------------
// Capture Status Mapping
// ---------------------------------------------------------------------------

/** Map backend capture status to display status */
function mapCaptureStatus(capture: DiagCapture): CaptureStatusDisplay {
  // First check explicit capture_status from backend
  if (capture.capture_status) {
    switch (capture.capture_status) {
      case 'captured':
        return 'captured';
      case 'skipped_cooldown':
      case 'suppressed':
        return 'suppressed';
      case 'failed':
      case 'error':
      case 'timeout':
        return 'failed';
      case 'disabled':
      case 'not_configured':
      case 'not_attempted':
      case 'missing':
      case 'none':
      case 'in_progress':
      default:
        return 'not_attempted';
    }
  }
  
  // Fallback to legacy suppressed_by_cooldown flag
  if (capture.suppressed_by_cooldown) {
    return 'suppressed';
  }
  
  switch (capture.status) {
    case 'ok':
      return 'captured';
    case 'error':
    case 'timeout':
      return 'failed';
    case 'disabled':
    case 'no_peer_mapping':
      return 'not_attempted';
    default:
      return 'not_attempted';
  }
}

// ---------------------------------------------------------------------------
// Normalization
// ---------------------------------------------------------------------------

/** Normalize a spike event to a timeline event */
function normalizeSpikeEvent(spike: SpikeEventWithCaptures): TimelineEvent {
  const canonicalTime = getCanonicalTime(spike);
  
  // Sort captures: prefer 'captured', then 'suppressed', then others
  const sortedCaptures = [...(spike.captures || [])].sort((a, b) => {
    const statusOrder: Record<string, number> = { captured: 0, suppressed: 1, failed: 2, not_attempted: 3 };
    return (statusOrder[mapCaptureStatus(a)] || 99) - (statusOrder[mapCaptureStatus(b)] || 99);
  });
  
  const primaryCapture = sortedCaptures[0] || null;
  const captureStatus = primaryCapture ? mapCaptureStatus(primaryCapture) : 'not_attempted';
  
  return {
    eventId: spike.event_id,
    targetId: spike.target_id,
    probeKind: spike.kind as ProbeKind,
    severity: spike.severity as Severity,
    latencyMs: spike.latency_ms,
    sampleTs: spike.sample_ts,
    collectedAt: spike.collected_at,
    reasons: spike.reasons,
    rollingMedianMs: spike.rolling_median_ms,
    thresholds: {
      warningMs: spike.thresholds?.warning_ms ?? 0,
      criticalMs: spike.thresholds?.critical_ms ?? 0,
      relativeMultiplier: spike.thresholds?.relative_multiplier ?? 0,
    },
    captures: sortedCaptures,
    primaryCapture,
    captureStatus,
    canonicalTime,
    sortProbeKind: spike.kind === 'http' ? 0 : 1,
    sortSeverity: spike.severity === 'warning' ? 0 : 1,
    sortEventId: spike.event_id,
  };
}

/** Normalize HTTP response */
function normalizeHttpResponse(response: SpikeResponseWithCaptures | null): TimelineEvent[] {
  if (!response?.spikes) return [];
  return response.spikes.map(normalizeSpikeEvent);
}

/** Normalize ICMP response */
function normalizeIcmpResponse(response: SpikeResponseWithCaptures | null): TimelineEvent[] {
  if (!response?.spikes) return [];
  return response.spikes.map(normalizeSpikeEvent);
}

// ---------------------------------------------------------------------------
// Sorting
// ---------------------------------------------------------------------------

/** Sort timeline events newest-first with stable tie-breaks */
function sortTimelineEvents(events: TimelineEvent[]): TimelineEvent[] {
  return [...events].sort((a, b) => {
    // Primary: newest-first canonical time
    const timeDiff = b.canonicalTime.getTime() - a.canonicalTime.getTime();
    if (timeDiff !== 0) return timeDiff;
    
    // Stable tie-break 1: probe kind (http before icmp)
    if (a.sortProbeKind !== b.sortProbeKind) {
      return a.sortProbeKind - b.sortProbeKind;
    }
    
    // Stable tie-break 2: severity (warning before critical)
    if (a.sortSeverity !== b.sortSeverity) {
      return a.sortSeverity - b.sortSeverity;
    }
    
    // Stable tie-break 3: event id
    return a.sortEventId.localeCompare(b.sortEventId);
  });
}

/** Merge HTTP and ICMP events into one chronological list */
function mergeEvents(httpEvents: TimelineEvent[], icmpEvents: TimelineEvent[]): TimelineEvent[] {
  const merged = [...httpEvents, ...icmpEvents];
  return sortTimelineEvents(merged);
}

// ---------------------------------------------------------------------------
// Summary Computation
// ---------------------------------------------------------------------------

/** Compute summary for a probe kind */
function computeProbeKindSummary(
  probeKind: ProbeKind,
  events: TimelineEvent[]
): ProbeKindSummary {
  const filtered = events.filter(e => e.probeKind === probeKind);
  
  return {
    probeKind,
    totalEvents: filtered.length,
    capturedCount: filtered.filter(e => e.captureStatus === 'captured').length,
    suppressedCount: filtered.filter(e => e.captureStatus === 'suppressed').length,
    failedCount: filtered.filter(e => e.captureStatus === 'failed').length,
    criticalCount: filtered.filter(e => e.severity === 'critical').length,
    warningCount: filtered.filter(e => e.severity === 'warning').length,
  };
}

// ---------------------------------------------------------------------------
// Fetching
// ---------------------------------------------------------------------------

const DEFAULT_LIMIT = 10;

/** Fetch both HTTP and ICMP responses in parallel */
export async function fetchTimelineResponses(
  targetId: string,
  limit: number = DEFAULT_LIMIT
): Promise<{ http: SpikeResponseWithCaptures | null; icmp: SpikeResponseWithCaptures | null }> {
  const [httpResponse, icmpResponse] = await Promise.allSettled([
    api.getLatencySpikesWithCaptures(targetId, 'http', limit),
    api.getLatencySpikesWithCaptures(targetId, 'icmp', limit),
  ]);
  
  const httpResult = httpResponse.status === 'fulfilled' ? httpResponse.value : null;
  const icmpResult = icmpResponse.status === 'fulfilled' ? icmpResponse.value : null;
  
  // If both fetches failed, throw an error to trigger error state
  if (httpResult === null && icmpResult === null) {
    throw new Error('Failed to load HTTP and ICMP diagnostic timelines');
  }
  
  return {
    http: httpResult,
    icmp: icmpResult,
  };
}

// ---------------------------------------------------------------------------
// State Building
// ---------------------------------------------------------------------------

/** Build complete timeline state from API responses */
export function buildTimelineState(
  httpResponse: SpikeResponseWithCaptures | null,
  icmpResponse: SpikeResponseWithCaptures | null
): TimelineState {
  const httpEvents = normalizeHttpResponse(httpResponse);
  const icmpEvents = normalizeIcmpResponse(icmpResponse);
  const mergedEvents = mergeEvents(httpEvents, icmpEvents);
  
  return {
    httpResponse,
    icmpResponse,
    httpEvents,
    icmpEvents,
    mergedEvents,
    httpSummary: computeProbeKindSummary('http', [...httpEvents, ...icmpEvents]),
    icmpSummary: computeProbeKindSummary('icmp', [...httpEvents, ...icmpEvents]),
    isLoading: false,
    error: null,
  };
}

/** Build empty timeline state */
export function buildEmptyTimelineState(): TimelineState {
  return {
    httpResponse: null,
    icmpResponse: null,
    httpEvents: [],
    icmpEvents: [],
    mergedEvents: [],
    httpSummary: computeProbeKindSummary('http', []),
    icmpSummary: computeProbeKindSummary('icmp', []),
    isLoading: false,
    error: null,
  };
}

/** Build loading timeline state */
export function buildLoadingTimelineState(): TimelineState {
  return {
    httpResponse: null,
    icmpResponse: null,
    httpEvents: [],
    icmpEvents: [],
    mergedEvents: [],
    httpSummary: computeProbeKindSummary('http', []),
    icmpSummary: computeProbeKindSummary('icmp', []),
    isLoading: true,
    error: null,
  };
}

/** Build error timeline state */
export function buildErrorTimelineState(error: string): TimelineState {
  return {
    httpResponse: null,
    icmpResponse: null,
    httpEvents: [],
    icmpEvents: [],
    mergedEvents: [],
    httpSummary: computeProbeKindSummary('http', []),
    icmpSummary: computeProbeKindSummary('icmp', []),
    isLoading: false,
    error,
  };
}

// ---------------------------------------------------------------------------
// Cooldown Info Helpers
// ---------------------------------------------------------------------------

/** Check if an event has cross-probe suppression */
export function hasCrossProbeSuppression(event: TimelineEvent): boolean {
  const capture = event.primaryCapture;
  if (!capture?.cooldown_info) return false;
  return capture.cooldown_info.is_cross_probe_suppression === true;
}

/** Get cooldown info for an event */
export function getCooldownInfo(event: TimelineEvent): CaptureCooldownInfo | null {
  return event.primaryCapture?.cooldown_info || null;
}

/** Get anchor probe kind from cooldown info */
export function getAnchorProbeKind(event: TimelineEvent): string | null {
  const info = getCooldownInfo(event);
  return info?.anchor_probe_kind || null;
}

/** Get suppressed probe kind from cooldown info */
export function getSuppressedProbeKind(event: TimelineEvent): string | null {
  const info = getCooldownInfo(event);
  return info?.suppressed_probe_kind || null;
}
