// Diagnostic Timeline - Re-export from Elm-ish module for backward compatibility
// New code should import from ./diagnosticTimeline/index.ts

// Re-export everything from the new Elm-ish module
export {
  DiagnosticTimelineController,
  mountDiagnosticTimeline,
} from './diagnosticTimeline/controller';

export {
  createInitialModel,
  defaultFilters,
  defaultPagination,
  PAGE_SIZE_OPTIONS,
  applyFilters,
  matchesAllFilters,
  calculatePagination,
  clampPageIndex,
  getPageIndexForRow,
  getFirstVisibleRow,
  getFilteredEvents,
  getPagedEvents,
  getPaginationInfo,
} from './diagnosticTimeline/model';

export type {
  TimelineModel,
  TimelineFilterState,
  TimelinePageState,
  ProbeKindSummary,
} from './diagnosticTimeline/model';

export type { TimelineMsg } from './diagnosticTimeline/msg';
export { update } from './diagnosticTimeline/update';
export { executeEffect } from './diagnosticTimeline/effects';
export { renderTimeline } from './diagnosticTimeline/view';

// Re-export types from the base model
export type {
  TimelineEvent,
  ProbeKind,
  Severity,
  CaptureStatusDisplay,
} from './diagnosticTimeline.model';

export { fetchTimelineResponses } from './diagnosticTimeline.model';
