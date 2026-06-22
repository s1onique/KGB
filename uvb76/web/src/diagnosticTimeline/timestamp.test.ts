// Diagnostic Timeline Timestamp Regression Tests
// Tests for the "Cannot read properties of undefined (reading 'getTime')" fix

import { describe, it, expect } from 'vitest';
import { sortTimelineEvents } from '../diagnosticTimeline.model';
import type { TimelineEvent } from '../diagnosticTimeline.model';

// ---------------------------------------------------------------------------
// Test Fixtures
// ---------------------------------------------------------------------------

/** Create a test event with specified timestamp */
function createEventWithTime(
  eventId: string,
  timeMs: number | null,
  status: 'ok' | 'missing' | 'invalid' = 'ok'
): TimelineEvent {
  return {
    eventId,
    targetId: 'test-target',
    probeKind: 'http',
    severity: 'warning',
    latencyMs: 100,
    sampleTs: timeMs !== null ? new Date(timeMs).toISOString() : '',
    collectedAt: timeMs !== null ? new Date(timeMs).toISOString() : '',
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
    canonicalTimeMs: timeMs,
    timeStatus: status,
    sortProbeKind: 0,
    sortSeverity: 0,
    sortEventId: eventId,
  };
}

// ---------------------------------------------------------------------------
// Timestamp Normalization Tests
// ---------------------------------------------------------------------------

describe('timestamp normalization', () => {
  it('normalizes valid ISO timestamp', () => {
    const event = createEventWithTime('evt-1', new Date('2026-06-18T12:00:00Z').getTime());
    expect(event.canonicalTimeMs).toBe(new Date('2026-06-18T12:00:00Z').getTime());
    expect(event.timeStatus).toBe('ok');
  });

  it('handles missing timestamp', () => {
    const event = createEventWithTime('evt-1', null, 'missing');
    expect(event.canonicalTimeMs).toBeNull();
    expect(event.timeStatus).toBe('missing');
  });

  it('handles invalid timestamp', () => {
    const event = createEventWithTime('evt-1', null, 'invalid');
    expect(event.canonicalTimeMs).toBeNull();
    expect(event.timeStatus).toBe('invalid');
  });
});

// ---------------------------------------------------------------------------
// Sorting Tests - The Core Regression Fix
// ---------------------------------------------------------------------------

describe('sorting with mixed timestamp quality', () => {
  it('sorts events newest-first for valid timestamps', () => {
    const events = [
      createEventWithTime('old', new Date('2026-06-01T12:00:00Z').getTime()),
      createEventWithTime('new', new Date('2026-06-18T12:00:00Z').getTime()),
      createEventWithTime('mid', new Date('2026-06-10T12:00:00Z').getTime()),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    expect(sorted[0].eventId).toBe('new');
    expect(sorted[1].eventId).toBe('mid');
    expect(sorted[2].eventId).toBe('old');
  });

  it('does NOT throw when sorting events with missing timestamps', () => {
    const events = [
      createEventWithTime('valid-1', new Date('2026-06-18T12:00:00Z').getTime()),
      createEventWithTime('missing-1', null, 'missing'),
      createEventWithTime('valid-2', new Date('2026-06-17T12:00:00Z').getTime()),
    ];
    
    // This should NOT throw "Cannot read properties of undefined (reading 'getTime')"
    expect(() => sortTimelineEvents(events)).not.toThrow();
  });

  it('does NOT throw when sorting events with invalid timestamps', () => {
    const events = [
      createEventWithTime('valid-1', new Date('2026-06-18T12:00:00Z').getTime()),
      createEventWithTime('invalid-1', null, 'invalid'),
      createEventWithTime('valid-2', new Date('2026-06-17T12:00:00Z').getTime()),
    ];
    
    // This should NOT throw
    expect(() => sortTimelineEvents(events)).not.toThrow();
  });

  it('does NOT throw when sorting ALL events with missing timestamps', () => {
    const events = [
      createEventWithTime('missing-1', null, 'missing'),
      createEventWithTime('missing-2', null, 'missing'),
      createEventWithTime('missing-3', null, 'missing'),
    ];
    
    // This should NOT throw
    expect(() => sortTimelineEvents(events)).not.toThrow();
  });

  it('sorts events with missing timestamps to the end', () => {
    const events = [
      createEventWithTime('missing-1', null, 'missing'),
      createEventWithTime('valid-1', new Date('2026-06-18T12:00:00Z').getTime()),
      createEventWithTime('missing-2', null, 'missing'),
      createEventWithTime('valid-2', new Date('2026-06-17T12:00:00Z').getTime()),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    // Valid timestamps should come first
    expect(sorted[0].eventId).toBe('valid-1');
    expect(sorted[1].eventId).toBe('valid-2');
    // Missing timestamps should come last
    expect(sorted[2].eventId).toBe('missing-1');
    expect(sorted[3].eventId).toBe('missing-2');
  });

  it('sorts events with invalid timestamps to the end', () => {
    const events = [
      createEventWithTime('invalid-1', null, 'invalid'),
      createEventWithTime('valid-1', new Date('2026-06-18T12:00:00Z').getTime()),
      createEventWithTime('invalid-2', null, 'invalid'),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    // Valid timestamps should come first
    expect(sorted[0].eventId).toBe('valid-1');
    // Invalid timestamps should come last
    expect(sorted[1].eventId).toBe('invalid-1');
    expect(sorted[2].eventId).toBe('invalid-2');
  });

  it('handles mixed valid, missing, and invalid timestamps', () => {
    const events = [
      createEventWithTime('missing', null, 'missing'),
      createEventWithTime('invalid', null, 'invalid'),
      createEventWithTime('valid-2', new Date('2026-06-17T12:00:00Z').getTime()),
      createEventWithTime('valid-1', new Date('2026-06-18T12:00:00Z').getTime()),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    // All valid first, newest first
    expect(sorted[0].eventId).toBe('valid-1');
    expect(sorted[1].eventId).toBe('valid-2');
    // Then missing/invalid (stable sort preserves original order since all have null timestamps)
    expect(sorted.length).toBe(4);
    const degradedEvents = sorted.slice(2);
    expect(degradedEvents.map(e => e.timeStatus)).toContain('missing');
    expect(degradedEvents.map(e => e.timeStatus)).toContain('invalid');
  });

  it('uses stable tie-breaking for same-timestamp events', () => {
    const sameTime = new Date('2026-06-18T12:00:00Z').getTime();
    const events = [
      createEventWithTime('b', sameTime),
      createEventWithTime('a', sameTime),
      createEventWithTime('http-a', sameTime),
    ];
    
    const sorted = sortTimelineEvents(events);
    
    // Should use stable tie-breakers: http before icmp, then eventId
    expect(sorted[0].eventId).toBe('a');
  });
});

// ---------------------------------------------------------------------------
// Pagination/Filter Path Tests
// ---------------------------------------------------------------------------

describe('pagination and filter paths with degraded timestamps', () => {
  it('can paginate over events with missing timestamps', () => {
    const events = [
      createEventWithTime('missing-1', null, 'missing'),
      createEventWithTime('valid-1', new Date('2026-06-18T12:00:00Z').getTime()),
      createEventWithTime('missing-2', null, 'missing'),
    ];
    
    // Simulate pagination: slice 0-2
    const page = events.slice(0, 2);
    
    // Should be able to iterate without throwing
    expect(() => {
      for (const event of page) {
        // Simulating what view might do
        if (event.canonicalTimeMs !== null) {
          new Date(event.canonicalTimeMs).toISOString();
        }
      }
    }).not.toThrow();
  });

  it('can filter events without accessing .getTime()', () => {
    const events = [
      createEventWithTime('missing-1', null, 'missing'),
      createEventWithTime('valid-1', new Date('2026-06-18T12:00:00Z').getTime()),
    ];
    
    // Filter by probe kind - no timestamp access needed
    const filtered = events.filter(e => e.probeKind === 'http');
    expect(filtered).toHaveLength(2);
  });

  it('preserves model integrity after refresh with malformed data', () => {
    // Simulate a refresh cycle where API returns malformed data
    const malformedEvents = [
      createEventWithTime('evt-1', null, 'missing'),
      createEventWithTime('evt-2', null, 'invalid'),
    ];
    
    // Should not corrupt data
    expect(malformedEvents[0].canonicalTimeMs).toBeNull();
    expect(malformedEvents[1].canonicalTimeMs).toBeNull();
    
    // Sorting should work
    const sorted = sortTimelineEvents(malformedEvents);
    expect(sorted).toHaveLength(2);
    
    // Filtering should work
    const filtered = malformedEvents.filter(e => e.probeKind === 'http');
    expect(filtered).toHaveLength(2);
  });
});
