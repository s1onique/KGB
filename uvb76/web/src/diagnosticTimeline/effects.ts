// Diagnostic Timeline Effects - Explicit side effects

import { fetchTimelineResponses } from '../diagnosticTimeline.model';
import type { TimelineMsg } from './msg';

// ---------------------------------------------------------------------------
// Effect Executor
// ---------------------------------------------------------------------------

/** Execute a side effect and dispatch resulting messages */
export async function executeEffect(
  effect: { type: 'FetchTimeline'; targetId: string },
  dispatch: (msg: TimelineMsg) => void
): Promise<void> {
  if (effect.type === 'FetchTimeline') {
    await fetchTimelineEffect(effect.targetId, dispatch);
  }
}

// ---------------------------------------------------------------------------
// Fetch Timeline Effect
// ---------------------------------------------------------------------------

/** Fetch timeline data and dispatch appropriate messages */
async function fetchTimelineEffect(
  targetId: string,
  dispatch: (msg: TimelineMsg) => void
): Promise<void> {
  dispatch({ type: 'LoadStarted' });
  
  try {
    const { http, icmp } = await fetchTimelineResponses(targetId);
    
    // Merge HTTP and ICMP events
    const httpEvents = http?.spikes || [];
    const icmpEvents = icmp?.spikes || [];
    const mergedEvents = [...httpEvents, ...icmpEvents];
    
    // Sort newest-first with stable tie-breaks
    const sortedEvents = sortTimelineEvents(mergedEvents);
    
    dispatch({ type: 'TimelineLoaded', events: sortedEvents });
  } catch (e) {
    const errorMessage = e instanceof Error ? e.message : 'Failed to load diagnostic timeline';
    dispatch({ type: 'LoadFailed', error: errorMessage });
  }
}

// ---------------------------------------------------------------------------
// Sort Helpers (needed for effect execution)
// ---------------------------------------------------------------------------

import type { TimelineEvent } from '../diagnosticTimeline.model';

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
