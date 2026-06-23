// Diagnostic Timeline Effects - Explicit side effects

import { 
  fetchTimelineResponses, 
  sortTimelineEvents,
  normalizeHttpResponse,
  normalizeIcmpResponse 
} from '../diagnosticTimeline.model';
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
    
    // CRITICAL: Normalize API responses before using them.
    // Raw spike objects do NOT have TimelineEvent fields like probeKind, severity, 
    // canonicalTimeMs, captureStatus, dataStatus, etc.
    // Previously this code directly used http?.spikes which caused all the placeholder 
    // row issues because fields weren't properly mapped.
    const httpEvents = normalizeHttpResponse(http);
    const icmpEvents = normalizeIcmpResponse(icmp);
    const mergedEvents = [...httpEvents, ...icmpEvents];
    
    // Sort newest-first with stable tie-breaks
    const sortedEvents = sortTimelineEvents(mergedEvents);
    
    dispatch({ type: 'TimelineLoaded', events: sortedEvents });
  } catch (e) {
    const errorMessage = e instanceof Error ? e.message : 'Failed to load diagnostic timeline';
    dispatch({ type: 'LoadFailed', error: errorMessage });
  }
}
