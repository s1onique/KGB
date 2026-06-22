// Diagnostic Timeline Model - Single source of truth for all UI state

import type { TimelineEvent } from '../diagnosticTimeline.model';
import type { ProbeKind, CaptureStatusDisplay, Severity } from '../diagnosticTimeline.model';

// ---------------------------------------------------------------------------
// Filter State
// ---------------------------------------------------------------------------

/** Filter state for the timeline */
export interface TimelineFilterState {
  probeKind: ProbeKind | 'all';
  captureStatus: CaptureStatusDisplay | 'all';
  severity: Severity | 'all';
}

/** Default filter state - show all events */
export const defaultFilters: TimelineFilterState = {
  probeKind: 'all',
  captureStatus: 'all',
  severity: 'all',
};

// ---------------------------------------------------------------------------
// Pagination State
// ---------------------------------------------------------------------------

/** Available page size options */
export const PAGE_SIZE_OPTIONS = [10, 20, 50, 100] as const;
export type PageSizeOption = typeof PAGE_SIZE_OPTIONS[number];

/** Pagination state for the timeline table */
export interface TimelinePageState {
  pageIndex: number;  // zero-based
  pageSize: number;
}

/** Default pagination state */
export const defaultPagination: TimelinePageState = {
  pageIndex: 0,
  pageSize: 20,
};

// ---------------------------------------------------------------------------
// Main Model
// ---------------------------------------------------------------------------

/** Summary for a probe kind */
export interface ProbeKindSummary {
  probeKind: ProbeKind;
  totalEvents: number;
  capturedCount: number;
  suppressedCount: number;
  failedCount: number;
  criticalCount: number;
  warningCount: number;
}

/** Complete timeline model - single source of truth */
export interface TimelineModel {
  // Server data
  mergedEvents: TimelineEvent[];
  httpSummary: ProbeKindSummary;
  icmpSummary: ProbeKindSummary;
  isLoading: boolean;
  error: string | null;
  
  // UI state
  filters: TimelineFilterState;
  pagination: TimelinePageState;
  expandedEventIds: Set<string>;
}

// ---------------------------------------------------------------------------
// Model Factory
// ---------------------------------------------------------------------------

/** Create initial empty model */
export function createInitialModel(): TimelineModel {
  return {
    mergedEvents: [],
    httpSummary: createEmptySummary('http'),
    icmpSummary: createEmptySummary('icmp'),
    isLoading: false,
    error: null,
    filters: { ...defaultFilters },
    pagination: { ...defaultPagination },
    expandedEventIds: new Set(),
  };
}

/** Create loading model */
export function createLoadingModel(): TimelineModel {
  return {
    ...createInitialModel(),
    isLoading: true,
  };
}

/** Create error model */
export function createErrorModel(error: string): TimelineModel {
  return {
    ...createInitialModel(),
    error,
  };
}

/** Create empty summary */
function createEmptySummary(probeKind: ProbeKind): ProbeKindSummary {
  return {
    probeKind,
    totalEvents: 0,
    capturedCount: 0,
    suppressedCount: 0,
    failedCount: 0,
    criticalCount: 0,
    warningCount: 0,
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
export function matchesAllFilters(event: TimelineEvent, filters: TimelineFilterState): boolean {
  return (
    matchesProbeKind(event, filters.probeKind) &&
    matchesCaptureStatus(event, filters.captureStatus) &&
    matchesSeverity(event, filters.severity)
  );
}

/** Apply filters to events */
export function applyFilters(events: TimelineEvent[], filters: TimelineFilterState): TimelineEvent[] {
  return events.filter(event => matchesAllFilters(event, filters));
}

// ---------------------------------------------------------------------------
// Pagination Computations
// ---------------------------------------------------------------------------

/** Calculate pagination values */
export function calculatePagination(
  filteredCount: number,
  pagination: TimelinePageState
): {
  totalPages: number;
  safePageIndex: number;
  start: number;
  end: number;
} {
  const totalPages = Math.max(1, Math.ceil(filteredCount / pagination.pageSize));
  const safePageIndex = Math.min(pagination.pageIndex, totalPages - 1);
  const start = safePageIndex * pagination.pageSize;
  const end = Math.min(start + pagination.pageSize, filteredCount);
  return { totalPages, safePageIndex, start, end };
}

/** Clamp page index to valid range */
export function clampPageIndex(pageIndex: number, totalPages: number): number {
  if (totalPages <= 0) return 0;
  return Math.min(Math.max(0, pageIndex), totalPages - 1);
}

/** Get the page index that would show a specific row index */
export function getPageIndexForRow(rowIndex: number, pageSize: number): number {
  return Math.floor(rowIndex / pageSize);
}

/** Get the first visible row index from a page index */
export function getFirstVisibleRow(pageIndex: number, pageSize: number): number {
  return pageIndex * pageSize;
}

// ---------------------------------------------------------------------------
// Summary Computations
// ---------------------------------------------------------------------------

/** Compute summary for a probe kind from events */
export function computeProbeKindSummary(
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
// Derived Data
// ---------------------------------------------------------------------------

/** Get filtered events from model */
export function getFilteredEvents(model: TimelineModel): TimelineEvent[] {
  return applyFilters(model.mergedEvents, model.filters);
}

/** Get paged events from model */
export function getPagedEvents(model: TimelineModel): TimelineEvent[] {
  const filtered = getFilteredEvents(model);
  const { start, end } = calculatePagination(filtered.length, model.pagination);
  return filtered.slice(start, end);
}

/** Get pagination info */
export function getPaginationInfo(model: TimelineModel): {
  totalPages: number;
  safePageIndex: number;
  start: number;
  end: number;
  filteredCount: number;
} {
  const filteredCount = getFilteredEvents(model).length;
  const { totalPages, safePageIndex, start, end } = calculatePagination(filteredCount, model.pagination);
  return { totalPages, safePageIndex, start, end, filteredCount };
}
