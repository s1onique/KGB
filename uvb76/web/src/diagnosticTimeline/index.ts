// Diagnostic Timeline Module - Re-exports for backward compatibility
// Use this module to import the Elm-ish diagnostic timeline components

export { DiagnosticTimelineController, mountDiagnosticTimeline } from './controller';
export type { TimelineModel, TimelineFilterState, TimelinePageState, ProbeKindSummary } from './model';
export { createInitialModel, defaultFilters, defaultPagination, PAGE_SIZE_OPTIONS } from './model';
export type { TimelineMsg } from './msg';
export { update } from './update';
export { executeEffect } from './effects';
export { renderTimeline } from './view';

// Re-export model utilities
export {
  applyFilters,
  matchesAllFilters,
  calculatePagination,
  clampPageIndex,
  getPageIndexForRow,
  getFirstVisibleRow,
  getFilteredEvents,
  getPagedEvents,
  getPaginationInfo,
} from './model';

// Re-export types from the base model
export type { 
  TimelineEvent, 
  ProbeKind, 
  Severity, 
  CaptureStatusDisplay,
  TimelineEventDataStatus,
  TimeStatus 
} from '../diagnosticTimeline.model';
