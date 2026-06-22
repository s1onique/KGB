// Diagnostic Timeline Effects - Explicit side effects

import { fetchTimelineResponses, sortTimelineEvents } from '../diagnosticTimeline.model';
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
