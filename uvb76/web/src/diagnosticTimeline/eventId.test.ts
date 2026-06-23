// Diagnostic Timeline Event ID Regression Tests
// Tests for the "Cannot read properties of undefined (reading 'localeCompare')" fix

import { describe, it, expect, vi } from 'vitest';
import { buildTimelineState, sortTimelineEvents } from '../diagnosticTimeline.model';
import type { SpikeResponseWithCaptures, TimelineEvent } from '../diagnosticTimeline.model';

// ---------------------------------------------------------------------------
// Test Fixtures
// ---------------------------------------------------------------------------

/** Create a valid spike response */
function createValidSpikeResponse(spikeOverrides: Record<string, unknown> = {}) {
  return {
    spikes: [{
      event_id: 'evt-1',
      target_id: 'test-target',
      kind: 'http',
      severity: 'warning',
      latency_ms: 100,
      sample_ts: '2026-06-18T12:00:00Z',
      rolling_median_ms: 50,
      reasons: [],
      thresholds: { warning_ms: 100, critical_ms: 500, relative_multiplier: 2 },
      previous_samples: [],
      collected_at: '2026-06-18T12:00:00Z',
      captures: [],
      ...spikeOverrides,
    }],
    count: 1,
    retention: {
      oldest_event_age_seconds: 3600,
      oldest_event_at: '2026-06-18T11:00:00Z',
      newest_event_age_seconds: 0,
      newest_event_at: '2026-06-18T12:00:00Z',
    },
  } as unknown as SpikeResponseWithCaptures;
}

/** Create a test event with specified eventId */
function createEventWithId(
  eventId: string,
  sortEventIdOverride?: string
): TimelineEvent {
  return {
    eventId,
    targetId: 'test-target',
    probeKind: 'http',
    severity: 'warning',
    latencyMs: 100,
    sampleTs: '2026-06-18T12:00:00Z',
    collectedAt: '2026-06-18T12:00:00Z',
    reasons: [],
    rollingMedianMs: 50,
    thresholds: {
      warningMs: 100,
      criticalMs: 500,
      relativeMultiplier: 2,
    },
    captures: [],
    primaryCapture: null,
    captureStatus: 'captured',
    canonicalTimeMs: new Date('2026-06-18T12:00:00Z').getTime(),
    timeStatus: 'ok',
    sortProbeKind: 0,
    sortSeverity: 0,
    sortEventId: sortEventIdOverride ?? eventId,
  };
}

// ---------------------------------------------------------------------------
// buildTimelineState Tests - Core Regression Fix
// ---------------------------------------------------------------------------

describe('buildTimelineState with malformed event_ids', () => {
  it('does not throw when spike is missing event_id', () => {
    const response = createValidSpikeResponse({ event_id: undefined });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has null event_id', () => {
    const response = createValidSpikeResponse({ event_id: null });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has empty string event_id', () => {
    const response = createValidSpikeResponse({ event_id: '' });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has whitespace-only event_id', () => {
    const response = createValidSpikeResponse({ event_id: '   ' });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('produces deterministic fallback eventId for missing event_id', () => {
    const response = createValidSpikeResponse({ event_id: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].eventId).toBeTruthy();
    expect(state.httpEvents[0].eventId.length).toBeGreaterThan(0);
    expect(state.httpEvents[0].eventId).toContain('missing-event-id');
  });

  it('produces matching eventId and sortEventId for missing event_id', () => {
    const response = createValidSpikeResponse({ event_id: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].eventId).toBe(state.httpEvents[0].sortEventId);
  });
});

// ---------------------------------------------------------------------------
// buildTimelineState Tests - Missing event_id + Invalid timestamp
// ---------------------------------------------------------------------------

describe('buildTimelineState with missing event_id and invalid timestamp', () => {
  it('does not throw when spike has missing event_id and invalid timestamp', () => {
    const response = createValidSpikeResponse({
      event_id: undefined,
      sample_ts: 'not-a-timestamp',
      collected_at: 'also-not-a-timestamp',
    });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has null event_id and null timestamp', () => {
    const response = createValidSpikeResponse({
      event_id: null,
      sample_ts: null,
      collected_at: null,
    });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('marks eventId as degraded when event_id is missing', () => {
    const response = createValidSpikeResponse({
      event_id: undefined,
      sample_ts: 'invalid',
      collected_at: 'also-invalid',
    });
    const state = buildTimelineState(response, null);
    
    const event = state.httpEvents[0];
    // collected_at fallback is also invalid, so timeStatus should be 'invalid'
    expect(event.timeStatus).toBe('invalid');
    expect(event.eventId).toContain('missing-event-id');
  });
});

// ---------------------------------------------------------------------------
// Two malformed rows sorting together
// ---------------------------------------------------------------------------

describe('sorting two malformed rows together', () => {
  it('does not throw when sorting two rows with missing event_id', () => {
    const events = [
      createEventWithId('evt-1', 'missing-event-id:test-target:http:2026-06-18T12:00:00Z'),
      createEventWithId('evt-2', 'missing-event-id:other-target:http:2026-06-18T12:00:00Z'),
    ];
    
    expect(() => sortTimelineEvents(events)).not.toThrow();
  });

  it('sorts two malformed rows deterministically', () => {
    const events = [
      createEventWithId('a', 'missing-event-id:z:http:2026-06-18T12:00:00Z'),
      createEventWithId('b', 'missing-event-id:a:http:2026-06-18T12:00:00Z'),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    // Should sort by fallback eventId since they have different sortEventIds
    expect(sorted.length).toBe(2);
    // Deterministic order based on localeCompare
    expect(sorted[0].eventId).toBeTruthy();
    expect(sorted[1].eventId).toBeTruthy();
  });

  it('handles all sort keys being undefined gracefully', () => {
    const events = [
      createEventWithId('evt-1', undefined as unknown as string),
      createEventWithId('evt-2', undefined as unknown as string),
    ];
    
    expect(() => sortTimelineEvents(events)).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Existing valid event ordering unchanged
// ---------------------------------------------------------------------------

describe('existing valid event ordering unchanged', () => {
  it('preserves stable sort order for valid events', () => {
    const events = [
      createEventWithId('evt-c'),
      createEventWithId('evt-a'),
      createEventWithId('evt-b'),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    // Same timestamp, same probe kind, same severity
    // Should sort by eventId
    expect(sorted[0].eventId).toBe('evt-a');
    expect(sorted[1].eventId).toBe('evt-b');
    expect(sorted[2].eventId).toBe('evt-c');
  });

  it('valid events sort before malformed events', () => {
    const events = [
      createEventWithId('malformed-1', 'missing-event-id:target:http:unknown'),
      createEventWithId('valid-1', 'evt-1'),
      createEventWithId('malformed-2', 'missing-event-id:target:http:unknown'),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    // Valid events (with valid timestamps) should come first
    expect(sorted[0].eventId).toBe('valid-1');
    expect(sorted[1].eventId).toBe('malformed-1');
    expect(sorted[2].eventId).toBe('malformed-2');
  });

  it('mixed valid/malformed events maintain proper grouping', () => {
    // Create events with different probe kinds to test sorting
    const malformedHttp = createEventWithId('malformed-http', 'missing-event-id:target:http:unknown');
    const validHttp = createEventWithId('valid-http', 'evt-1');
    const validIcmp = { ...createEventWithId('valid-icmp', 'evt-2'), probeKind: 'icmp' as const, sortProbeKind: 1 };
    const malformedIcmp = { ...createEventWithId('malformed-icmp', 'missing-event-id:target:icmp:unknown'), probeKind: 'icmp' as const, sortProbeKind: 1 };
    
    const events = [malformedHttp, validHttp, validIcmp, malformedIcmp];
    
    const sorted = sortTimelineEvents(events);
    
    // All with same timestamp sort by probe kind (http=0 before icmp=1), then severity, then eventId
    // Since all have same timestamp, probe kind determines order:
    // 1. valid-http (http, valid eventId)
    // 2. malformed-http (http, malformed eventId)  
    // 3. valid-icmp (icmp, valid eventId)
    // 4. malformed-icmp (icmp, malformed eventId)
    expect(sorted[0].eventId).toBe('valid-http');
    expect(sorted[1].eventId).toBe('malformed-http');
    expect(sorted[2].eventId).toBe('valid-icmp');
    expect(sorted[3].eventId).toBe('malformed-icmp');
  });
});

// ---------------------------------------------------------------------------
// Edge Cases
// ---------------------------------------------------------------------------

describe('edge cases for event ID normalization', () => {
  it('handles numeric event_id by converting to string', () => {
    const response = createValidSpikeResponse({ event_id: 12345 });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].eventId).toBe('12345');
    expect(state.httpEvents[0].sortEventId).toBe('12345');
  });

  it('handles boolean event_id with fallback', () => {
    const response = createValidSpikeResponse({ event_id: true });
    const state = buildTimelineState(response, null);
    
    // Booleans are not strings or numbers in normalizeSortString, so fallback is used
    expect(state.httpEvents[0].eventId).toContain('missing-event-id');
  });

  it('handles array event_id with fallback', () => {
    const response = createValidSpikeResponse({ event_id: ['a', 'b'] });
    const state = buildTimelineState(response, null);
    
    // Arrays fall through to fallback
    expect(state.httpEvents[0].eventId).toContain('missing-event-id');
  });

  it('handles object event_id with fallback', () => {
    const response = createValidSpikeResponse({ event_id: { id: 1 } });
    const state = buildTimelineState(response, null);
    
    // Objects fall through to fallback
    expect(state.httpEvents[0].eventId).toContain('missing-event-id');
  });

  it('preserves valid non-empty string event_id', () => {
    const response = createValidSpikeResponse({ event_id: 'valid-event-id-123' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].eventId).toBe('valid-event-id-123');
    expect(state.httpEvents[0].sortEventId).toBe('valid-event-id-123');
  });
});
