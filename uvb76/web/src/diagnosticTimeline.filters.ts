// Diagnostic Timeline Filters - Filter state and filter application logic
import type { TimelineEvent, ProbeKind, CaptureStatusDisplay, Severity } from './diagnosticTimeline.model';

// ---------------------------------------------------------------------------
// Filter State
// ---------------------------------------------------------------------------

/** Filter state for the timeline */
export interface TimelineFilters {
  probeKind: ProbeKind | 'all';
  captureStatus: CaptureStatusDisplay | 'all';
  severity: Severity | 'all';
}

/** Default filter state - show all events */
export const defaultFilters: TimelineFilters = {
  probeKind: 'all',
  captureStatus: 'all',
  severity: 'all',
};

/** Create a new filter state with partial overrides */
export function createFilters(overrides?: Partial<TimelineFilters>): TimelineFilters {
  return {
    probeKind: overrides?.probeKind ?? defaultFilters.probeKind,
    captureStatus: overrides?.captureStatus ?? defaultFilters.captureStatus,
    severity: overrides?.severity ?? defaultFilters.severity,
  };
}

// ---------------------------------------------------------------------------
// Filter Predicates
// ---------------------------------------------------------------------------

/** Check if an event matches the probe kind filter */
export function matchesProbeKind(event: TimelineEvent, filter: ProbeKind | 'all'): boolean {
  if (filter === 'all') return true;
  return event.probeKind === filter;
}

/** Check if an event matches the capture status filter */
export function matchesCaptureStatus(event: TimelineEvent, filter: CaptureStatusDisplay | 'all'): boolean {
  if (filter === 'all') return true;
  return event.captureStatus === filter;
}

/** Check if an event matches the severity filter */
export function matchesSeverity(event: TimelineEvent, filter: Severity | 'all'): boolean {
  if (filter === 'all') return true;
  return event.severity === filter;
}

/** Check if an event matches all active filters */
export function matchesAllFilters(event: TimelineEvent, filters: TimelineFilters): boolean {
  return (
    matchesProbeKind(event, filters.probeKind) &&
    matchesCaptureStatus(event, filters.captureStatus) &&
    matchesSeverity(event, filters.severity)
  );
}

// ---------------------------------------------------------------------------
// Filter Application
// ---------------------------------------------------------------------------

/** Apply filters to a list of events */
export function applyFilters(events: TimelineEvent[], filters: TimelineFilters): TimelineEvent[] {
  return events.filter(event => matchesAllFilters(event, filters));
}

/** Count events that match the filters */
export function countFilteredEvents(events: TimelineEvent[], filters: TimelineFilters): number {
  return events.filter(event => matchesAllFilters(event, filters)).length;
}

// ---------------------------------------------------------------------------
// Filter Validation
// ---------------------------------------------------------------------------

/** Check if any filters are active (non-default) */
export function hasActiveFilters(filters: TimelineFilters): boolean {
  return (
    filters.probeKind !== 'all' ||
    filters.captureStatus !== 'all' ||
    filters.severity !== 'all'
  );
}

/** Get a summary of active filter counts */
export function getActiveFilterCount(filters: TimelineFilters): number {
  let count = 0;
  if (filters.probeKind !== 'all') count++;
  if (filters.captureStatus !== 'all') count++;
  if (filters.severity !== 'all') count++;
  return count;
}

// ---------------------------------------------------------------------------
// Filter Display Helpers
// ---------------------------------------------------------------------------

/** Get display label for a probe kind filter */
export function getProbeKindFilterLabel(filter: ProbeKind | 'all'): string {
  switch (filter) {
    case 'http': return 'HTTP';
    case 'icmp': return 'ICMP';
    case 'all': return 'All probes';
    default: return String(filter);
  }
}

/** Get display label for a capture status filter */
export function getCaptureStatusFilterLabel(filter: CaptureStatusDisplay | 'all'): string {
  switch (filter) {
    case 'captured': return 'Captured';
    case 'suppressed': return 'Suppressed';
    case 'failed': return 'Failed';
    case 'not_attempted': return 'Not attempted';
    case 'all': return 'All statuses';
    default: return String(filter);
  }
}

/** Get display label for a severity filter */
export function getSeverityFilterLabel(filter: Severity | 'all'): string {
  switch (filter) {
    case 'warning': return 'Warning';
    case 'critical': return 'Critical';
    case 'all': return 'All severities';
    default: return String(filter);
  }
}

// ---------------------------------------------------------------------------
// URL State Sync (optional - for bookmarking/shareable filter states)
// ---------------------------------------------------------------------------

/** Serialize filters to URL search params */
export function filtersToQueryParams(filters: TimelineFilters): URLSearchParams {
  const params = new URLSearchParams();
  if (filters.probeKind !== 'all') params.set('probe', filters.probeKind);
  if (filters.captureStatus !== 'all') params.set('status', filters.captureStatus);
  if (filters.severity !== 'all') params.set('severity', filters.severity);
  return params;
}

/** Parse filters from URL search params */
export function filtersFromQueryParams(params: URLSearchParams): TimelineFilters {
  const probeKind = params.get('probe') as ProbeKind | 'all' | null;
  const captureStatus = params.get('status') as CaptureStatusDisplay | 'all' | null;
  const severity = params.get('severity') as Severity | 'all' | null;
  
  return createFilters({
    probeKind: probeKind === 'http' || probeKind === 'icmp' ? probeKind : undefined,
    captureStatus: captureStatus === 'captured' || captureStatus === 'suppressed' || 
                   captureStatus === 'failed' || captureStatus === 'not_attempted' 
                   ? captureStatus : undefined,
    severity: severity === 'warning' || severity === 'critical' ? severity : undefined,
  });
}
