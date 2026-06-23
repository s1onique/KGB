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

/** Timestamp quality status */
export type TimeStatus = 'ok' | 'missing' | 'invalid';

/** Data quality status for a timeline event */
export type TimelineEventDataStatus = 'ok' | 'malformed';

/** Unified timeline event combining HTTP and ICMP spike events */
export interface TimelineEvent {
  // Source data
  eventId: string;
  targetId: string;
  probeKind: ProbeKind | 'unknown';
  severity: Severity | 'unknown';
  latencyMs: number | null;
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
  
  // Canonical event time - numeric sort key (milliseconds since epoch)
  // null indicates missing/invalid timestamp
  canonicalTimeMs: number | null;
  
  // Timestamp quality status for degraded rendering
  timeStatus: TimeStatus;
  
  // Stable sort keys
  sortProbeKind: number;  // http=0, icmp=1, unknown=2
  sortSeverity: number;    // warning=0, critical=1, unknown=2
  sortEventId: string;     // event id for stable tie-break

  // Data quality tracking - explicit malformed-row semantics
  // Distinguishes "legitimate missing optional value" from "wrong DTO shape"
  dataStatus: TimelineEventDataStatus;
  malformedReasons: string[];
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
// Timestamp Parsing
// ---------------------------------------------------------------------------

/** Result of parsing a timestamp from API response */
interface ParsedTimestamp {
  ms: number | null;
  status: TimeStatus;
}

/**
 * Parse a timestamp value and return milliseconds with status.
 * Handles missing, empty, and invalid timestamp values.
 */
function parseTimestamp(value: unknown): ParsedTimestamp {
  // Handle missing/undefined/null
  if (value === null || value === undefined || value === '') {
    return { ms: null, status: 'missing' };
  }
  
  // Handle non-string values
  const strValue = String(value).trim();
  if (strValue === '') {
    return { ms: null, status: 'missing' };
  }
  
  // Parse the timestamp
  const ms = Date.parse(strValue);
  
  // Handle invalid date (e.g., "not-a-date", "0001-01-01T00:00:00Z" on some parsers)
  if (!Number.isFinite(ms)) {
    return { ms: null, status: 'invalid' };
  }
  
  return { ms, status: 'ok' };
}

/**
 * Get canonical event time using deterministic selector:
 * 1. sample timestamp if present
 * 2. collected/created timestamp if present
 * 3. capture started timestamp if present
 * 4. capture finished timestamp if present
 * 5. missing/invalid (based on whether we saw any invalid timestamps)
 */
function getCanonicalTime(spike: SpikeEventWithCaptures): ParsedTimestamp {
  let sawInvalid = false;
  
  const consider = (value: unknown): ParsedTimestamp | null => {
    if (value === undefined || value === null || value === '') return null;
    
    const result = parseTimestamp(value);
    if (result.status === 'ok') return result;
    if (result.status === 'invalid') sawInvalid = true;
    return null;
  };
  
  // 1. sample timestamp
  const sample = consider(spike.sample_ts);
  if (sample) return sample;
  
  // 2. collected timestamp
  const collected = consider(spike.collected_at);
  if (collected) return collected;
  
  // 3-4. capture timestamps
  const capture = spike.captures?.[0];
  if (capture) {
    const started = consider(capture.capture_started_at);
    if (started) return started;
    
    const finished = consider(capture.capture_finished_at);
    if (finished) return finished;
  }
  
  // 5. No valid timestamp found - report invalid if we saw invalid values, else missing
  return { ms: null, status: sawInvalid ? 'invalid' : 'missing' };
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
// String Normalization
// ---------------------------------------------------------------------------

/**
 * Normalize a value to a non-empty string for stable sorting.
 * Handles missing, empty, and non-string values.
 */
function normalizeSortString(value: unknown, fallback: string): string {
  if (typeof value === 'string' && value.trim() !== '') {
    return value;
  }
  if (typeof value === 'number' && Number.isFinite(value)) {
    return String(value);
  }
  return fallback;
}

/**
 * Generate a deterministic fallback event ID from stable row contents.
 */
function normalizeEventId(spike: SpikeEventWithCaptures): string {
  const eventIdFallback = [
    spike.target_id ?? 'unknown',
    spike.kind ?? 'unknown',
    spike.sample_ts ?? spike.collected_at ?? 'unknown',
  ].join(':');
  
  return normalizeSortString(spike.event_id, `missing-event-id:${eventIdFallback}`);
}

/**
 * Safe string comparison that handles undefined/null operands.
 */
function compareStringKey(a: unknown, b: unknown): number {
  return String(a ?? '').localeCompare(String(b ?? ''));
}

// ---------------------------------------------------------------------------
// Normalization
// ---------------------------------------------------------------------------

/**
 * Normalize a probe kind value to a valid ProbeKind.
 * Handles missing, invalid, or non-string values from API.
 */
function normalizeProbeKind(value: unknown): ProbeKind {
  if (value === 'icmp') return 'icmp';
  return 'http'; // Default to http for any other value
}

/**
 * Normalize a severity value to a valid Severity.
 * Handles missing, invalid, or non-string values from API.
 */
function normalizeSeverity(value: unknown): Severity {
  if (value === 'critical') return 'critical';
  return 'warning'; // Default to warning for any other value
}

/**
 * Normalize a display string value - always returns a non-empty string.
 * Handles missing, empty, null, undefined values from API.
 */
function normalizeDisplayString(value: unknown, fallback: string): string {
  if (typeof value === 'string' && value.trim() !== '') {
    return value;
  }
  if (typeof value === 'number' && Number.isFinite(value)) {
    return String(value);
  }
  return fallback;
}

/**
 * Result of normalizing a spike event with data quality tracking.
 */
interface NormalizeSpikeEventResult {
  event: TimelineEvent;
  malformedReasons: string[];
}

/**
 * Validate and normalize a spike event, tracking data quality issues.
 * 
 * Unlike previous implementations that silently defaulted missing values,
 * this function explicitly tracks when the DTO shape is wrong so the UI
 * can render honest degraded rows instead of fake data.
 */
function normalizeSpikeEvent(spike: SpikeEventWithCaptures): NormalizeSpikeEventResult {
  const malformedReasons: string[] = [];
  
  const { ms, status } = getCanonicalTime(spike);
  
  // Validate event_id - missing is a data quality issue
  const hasEventId = typeof spike.event_id === 'string' && spike.event_id.trim() !== '';
  if (!hasEventId) {
    malformedReasons.push('missing event_id');
  }
  
  // Validate kind - unknown/missing is a data quality issue (not defaulting to http)
  const hasValidKind = spike.kind === 'http' || spike.kind === 'icmp';
  if (!hasValidKind) {
    malformedReasons.push(spike.kind === undefined ? 'missing kind' : `invalid kind: ${spike.kind}`);
  }
  
  // Validate severity - unknown/missing is a data quality issue (not defaulting to warning)
  const hasValidSeverity = spike.severity === 'warning' || spike.severity === 'critical';
  if (!hasValidSeverity) {
    malformedReasons.push(spike.severity === undefined ? 'missing severity' : `invalid severity: ${spike.severity}`);
  }
  
  // Validate latency_ms
  const hasValidLatency = typeof spike.latency_ms === 'number' && Number.isFinite(spike.latency_ms);
  if (!hasValidLatency) {
    malformedReasons.push(spike.latency_ms === undefined ? 'missing latency_ms' : `invalid latency_ms: ${spike.latency_ms}`);
  }
  
  // Normalize event identity - always non-empty string
  const eventId = normalizeEventId(spike);
  
  // Determine effective probeKind and severity (with 'unknown' for malformed values)
  // This preserves type safety while allowing honest degraded rendering
  let probeKind: ProbeKind | 'unknown';
  let sortProbeKind: number;
  if (spike.kind === 'http') {
    probeKind = 'http';
    sortProbeKind = 0;
  } else if (spike.kind === 'icmp') {
    probeKind = 'icmp';
    sortProbeKind = 1;
  } else {
    probeKind = 'unknown';
    sortProbeKind = 2; // Sort unknown after known values
  }
  
  let severity: Severity | 'unknown';
  let sortSeverity: number;
  if (spike.severity === 'warning') {
    severity = 'warning';
    sortSeverity = 0;
  } else if (spike.severity === 'critical') {
    severity = 'critical';
    sortSeverity = 1;
  } else {
    severity = 'unknown';
    sortSeverity = 2; // Sort unknown after known values
  }
  
  const targetId = normalizeDisplayString(spike.target_id, 'unknown-target');
  
  // Sort captures: prefer 'captured', then 'suppressed', then others
  const sortedCaptures = [...(spike.captures || [])].sort((a, b) => {
    const statusOrder: Record<string, number> = { captured: 0, suppressed: 1, failed: 2, not_attempted: 3 };
    return (statusOrder[mapCaptureStatus(a)] || 99) - (statusOrder[mapCaptureStatus(b)] || 99);
  });
  
  const primaryCapture = sortedCaptures[0] || null;
  const captureStatus = primaryCapture ? mapCaptureStatus(primaryCapture) : 'not_attempted';
  
  const event: TimelineEvent = {
    eventId,
    targetId,
    probeKind,
    severity,
    latencyMs: hasValidLatency ? spike.latency_ms : null,
    sampleTs: spike.sample_ts || '',
    collectedAt: spike.collected_at || '',
    reasons: spike.reasons || [],
    rollingMedianMs: spike.rolling_median_ms ?? 0,
    thresholds: {
      warningMs: spike.thresholds?.warning_ms ?? 0,
      criticalMs: spike.thresholds?.critical_ms ?? 0,
      relativeMultiplier: spike.thresholds?.relative_multiplier ?? 0,
    },
    captures: sortedCaptures,
    primaryCapture,
    captureStatus,
    canonicalTimeMs: ms,
    timeStatus: status,
    sortProbeKind,
    sortSeverity,
    sortEventId: eventId,
    dataStatus: malformedReasons.length === 0 ? 'ok' : 'malformed',
    malformedReasons,
  };
  
  return { event, malformedReasons };
}

/** Normalize HTTP response - exported for use in effects.ts */
export function normalizeHttpResponse(response: SpikeResponseWithCaptures | null): TimelineEvent[] {
  if (!response?.spikes) return [];
  return response.spikes.map(spike => normalizeSpikeEvent(spike).event);
}

/** Normalize ICMP response - exported for use in effects.ts */
export function normalizeIcmpResponse(response: SpikeResponseWithCaptures | null): TimelineEvent[] {
  if (!response?.spikes) return [];
  return response.spikes.map(spike => normalizeSpikeEvent(spike).event);
}

// ---------------------------------------------------------------------------
// Sorting
// ---------------------------------------------------------------------------

/** Sort timeline events newest-first with stable tie-breaks.
 * Events with missing/invalid timestamps are sorted to the end.
 */
export function sortTimelineEvents(events: TimelineEvent[]): TimelineEvent[] {
  return [...events].sort((a, b) => {
    // Primary: newest-first canonical time
    // null timestamps sort after valid timestamps
    const aTime = a.canonicalTimeMs ?? Number.MIN_SAFE_INTEGER;
    const bTime = b.canonicalTimeMs ?? Number.MIN_SAFE_INTEGER;
    const timeDiff = bTime - aTime;
    if (timeDiff !== 0) return timeDiff;
    
    // Stable tie-break 1: probe kind (http before icmp)
    if (a.sortProbeKind !== b.sortProbeKind) {
      return a.sortProbeKind - b.sortProbeKind;
    }
    
    // Stable tie-break 2: severity (warning before critical)
    if (a.sortSeverity !== b.sortSeverity) {
      return a.sortSeverity - b.sortSeverity;
    }
    
    // Stable tie-break 3: event id (use safe comparator for defense-in-depth)
    return compareStringKey(a.sortEventId, b.sortEventId);
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
