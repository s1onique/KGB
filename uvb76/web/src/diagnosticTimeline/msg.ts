// Diagnostic Timeline Messages - All possible user actions and system events

import type { TimelineEvent } from '../diagnosticTimeline.model';
import type { TimelineFilterState } from './model';

// ---------------------------------------------------------------------------
// Message Types
// ---------------------------------------------------------------------------

/** All possible messages (actions/events) for the diagnostic timeline */
export type TimelineMsg =
  | { type: 'TimelineLoaded'; events: TimelineEvent[] }
  | { type: 'RefreshRequested' }
  | { type: 'PageChanged'; page: number }
  | { type: 'PageSizeChanged'; pageSize: number }
  | { type: 'FilterChanged'; filters: Partial<TimelineFilterState> }
  | { type: 'FiltersReset' }
  | { type: 'RowToggled'; eventId: string }
  | { type: 'RefreshFailed'; error: string }
  | { type: 'LoadStarted' }
  | { type: 'LoadFailed'; error: string };

// ---------------------------------------------------------------------------
// Effect Types
// ---------------------------------------------------------------------------

/** Side effects that require async operations */
export type TimelineEffect =
  | { type: 'FetchTimeline'; targetId: string };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Check if a message requires fetching new data */
export function isFetchEffect(msg: TimelineMsg): boolean {
  return msg.type === 'RefreshRequested';
}

/** Get the error from a failed message if present */
export function getError(msg: TimelineMsg): string | null {
  switch (msg.type) {
    case 'RefreshFailed':
    case 'LoadFailed':
      return msg.error;
    default:
      return null;
  }
}
