// Diagnostic Timeline Malformed Fields Regression Tests
// Tests for the "Cannot read properties of undefined (reading 'toUpperCase')" fix
// AND the DTO contract correctness fix for honest malformed-row rendering

import { describe, it, expect } from 'vitest';
import { buildTimelineState, sortTimelineEvents } from '../diagnosticTimeline.model';
import type { SpikeResponseWithCaptures, TimelineEvent, ProbeKind, Severity } from '../diagnosticTimeline.model';

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

/** Create a test event with normalized fields */
function createEventWithFields(overrides: Partial<{
  probeKind: ProbeKind;
  severity: Severity;
  targetId: string;
  eventId: string;
}> = {}): TimelineEvent {
  const eventId = overrides.eventId ?? 'evt-1';
  const probeKind = overrides.probeKind ?? 'http';
  const severity = overrides.severity ?? 'warning';
  const targetId = overrides.targetId ?? 'test-target';
  const sampleTs = '2026-06-18T12:00:00Z';
  
  return {
    eventId,
    targetId,
    probeKind,
    severity,
    latencyMs: 100,
    sampleTs,
    collectedAt: sampleTs,
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
    canonicalTimeMs: new Date(sampleTs).getTime(),
    timeStatus: 'ok',
    sortProbeKind: probeKind === 'http' ? 0 : 1,
    sortSeverity: severity === 'warning' ? 0 : 1,
    sortEventId: eventId,
    dataStatus: 'ok',
    malformedReasons: [],
  };
}

// ---------------------------------------------------------------------------
// Valid Event Tests (should remain unchanged)
// ---------------------------------------------------------------------------

describe('buildTimelineState with valid spike event', () => {
  it('produces ok event with correct probeKind', () => {
    const response = createValidSpikeResponse({ kind: 'http' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('ok');
    expect(state.httpEvents[0].probeKind).toBe('http');
    expect(state.httpEvents[0].malformedReasons).toHaveLength(0);
  });

  it('produces ok event with icmp probeKind', () => {
    const response = createValidSpikeResponse({ kind: 'icmp' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('ok');
    expect(state.httpEvents[0].probeKind).toBe('icmp');
  });

  it('produces ok event with critical severity', () => {
    const response = createValidSpikeResponse({ severity: 'critical' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('ok');
    expect(state.httpEvents[0].severity).toBe('critical');
  });
});

// ---------------------------------------------------------------------------
// buildTimelineState Tests - Missing/Invalid probeKind
// ---------------------------------------------------------------------------

describe('buildTimelineState with malformed probeKind', () => {
  it('does not throw when spike is missing kind', () => {
    const response = createValidSpikeResponse({ kind: undefined });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has null kind', () => {
    const response = createValidSpikeResponse({ kind: null });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has empty string kind', () => {
    const response = createValidSpikeResponse({ kind: '' });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has invalid kind value', () => {
    const response = createValidSpikeResponse({ kind: 'not-a-valid-kind' });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has numeric kind', () => {
    const response = createValidSpikeResponse({ kind: 123 });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('marks missing kind as malformed with UNKNOWN probeKind', () => {
    const response = createValidSpikeResponse({ kind: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('malformed');
    expect(state.httpEvents[0].probeKind).toBe('unknown');
    expect(state.httpEvents[0].malformedReasons).toContain('missing kind');
  });

  it('marks invalid kind as malformed with UNKNOWN probeKind', () => {
    const response = createValidSpikeResponse({ kind: 'garbage' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('malformed');
    expect(state.httpEvents[0].probeKind).toBe('unknown');
    expect(state.httpEvents[0].malformedReasons).toContain('invalid kind: garbage');
  });

  it('preserves icmp when kind is icmp', () => {
    const response = createValidSpikeResponse({ kind: 'icmp' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('ok');
    expect(state.httpEvents[0].probeKind).toBe('icmp');
  });
});

// ---------------------------------------------------------------------------
// buildTimelineState Tests - Missing/Invalid severity
// ---------------------------------------------------------------------------

describe('buildTimelineState with malformed severity', () => {
  it('does not throw when spike is missing severity', () => {
    const response = createValidSpikeResponse({ severity: undefined });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has null severity', () => {
    const response = createValidSpikeResponse({ severity: null });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has empty string severity', () => {
    const response = createValidSpikeResponse({ severity: '' });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has invalid severity value', () => {
    const response = createValidSpikeResponse({ severity: 'not-a-valid-severity' });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has numeric severity', () => {
    const response = createValidSpikeResponse({ severity: 999 });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('marks missing severity as malformed with UNKNOWN severity', () => {
    const response = createValidSpikeResponse({ severity: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('malformed');
    expect(state.httpEvents[0].severity).toBe('unknown');
    expect(state.httpEvents[0].malformedReasons).toContain('missing severity');
  });

  it('marks invalid severity as malformed with UNKNOWN severity', () => {
    const response = createValidSpikeResponse({ severity: 'garbage' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('malformed');
    expect(state.httpEvents[0].severity).toBe('unknown');
    expect(state.httpEvents[0].malformedReasons).toContain('invalid severity: garbage');
  });

  it('preserves critical when severity is critical', () => {
    const response = createValidSpikeResponse({ severity: 'critical' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('ok');
    expect(state.httpEvents[0].severity).toBe('critical');
  });
});

// ---------------------------------------------------------------------------
// buildTimelineState Tests - Missing/Invalid latency_ms
// ---------------------------------------------------------------------------

describe('buildTimelineState with malformed latency_ms', () => {
  it('does not throw when spike is missing latency_ms', () => {
    const response = createValidSpikeResponse({ latency_ms: undefined });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('marks missing latency as malformed', () => {
    const response = createValidSpikeResponse({ latency_ms: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('malformed');
    expect(state.httpEvents[0].latencyMs).toBeNull();
    expect(state.httpEvents[0].malformedReasons).toContain('missing latency_ms');
  });

  it('marks invalid latency as malformed', () => {
    const response = createValidSpikeResponse({ latency_ms: NaN });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].dataStatus).toBe('malformed');
    expect(state.httpEvents[0].latencyMs).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// buildTimelineState Tests - Missing/Invalid target_id
// ---------------------------------------------------------------------------

describe('buildTimelineState with malformed target_id', () => {
  it('does not throw when spike is missing target_id', () => {
    const response = createValidSpikeResponse({ target_id: undefined });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has null target_id', () => {
    const response = createValidSpikeResponse({ target_id: null });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has empty string target_id', () => {
    const response = createValidSpikeResponse({ target_id: '' });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('normalizes missing target_id to unknown-target', () => {
    const response = createValidSpikeResponse({ target_id: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].targetId).toBe('unknown-target');
  });

  it('normalizes empty target_id to unknown-target', () => {
    const response = createValidSpikeResponse({ target_id: '' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].targetId).toBe('unknown-target');
  });

  it('preserves valid target_id', () => {
    const response = createValidSpikeResponse({ target_id: 'my-custom-target' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].targetId).toBe('my-custom-target');
  });
});

// ---------------------------------------------------------------------------
// Combined Malformed Fields Tests
// ---------------------------------------------------------------------------

describe('buildTimelineState with multiple malformed fields', () => {
  it('does not throw when spike has missing kind + missing severity + missing event_id', () => {
    const response = createValidSpikeResponse({
      kind: undefined,
      severity: undefined,
      event_id: undefined,
    });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('does not throw when spike has all fields malformed', () => {
    const response = createValidSpikeResponse({
      event_id: undefined,
      target_id: undefined,
      kind: undefined,
      severity: undefined,
      sample_ts: 'not-a-timestamp',
      collected_at: 'also-not-a-timestamp',
    });
    
    expect(() => {
      buildTimelineState(response, null);
    }).not.toThrow();
  });

  it('produces malformed event when all fields are missing', () => {
    const response = createValidSpikeResponse({
      event_id: undefined,
      target_id: undefined,
      kind: undefined,
      severity: undefined,
      latency_ms: undefined, // Must explicitly set to undefined
      sample_ts: 'not-a-timestamp',
    });
    const state = buildTimelineState(response, null);
    
    const event = state.httpEvents[0];
    
    // Should be marked as malformed
    expect(event.dataStatus).toBe('malformed');
    expect(event.malformedReasons.length).toBeGreaterThan(0);
    
    // probeKind and severity should be 'unknown', NOT default http/warning
    expect(event.probeKind).toBe('unknown');
    expect(event.severity).toBe('unknown');
    
    // Event ID should still be generated (fallback)
    expect(event.eventId).toBeTruthy();
    expect(event.eventId.length).toBeGreaterThan(0);
    
    // latencyMs should be null when latency_ms is undefined
    expect(event.latencyMs).toBeNull();
  });

  it('accumulates all malformed reasons', () => {
    const response = createValidSpikeResponse({
      event_id: undefined,
      kind: undefined,
      severity: undefined,
      latency_ms: undefined,
    });
    const state = buildTimelineState(response, null);
    
    const reasons = state.httpEvents[0].malformedReasons;
    expect(reasons).toContain('missing event_id');
    expect(reasons).toContain('missing kind');
    expect(reasons).toContain('missing severity');
    expect(reasons).toContain('missing latency_ms');
  });

  it('sorts multiple malformed events without throwing', () => {
    const response1 = createValidSpikeResponse({
      event_id: undefined,
      kind: undefined,
      severity: undefined,
    });
    const response2 = createValidSpikeResponse({
      event_id: undefined,
      kind: 'icmp',
      severity: 'critical',
    });
    
    const state1 = buildTimelineState(response1, null);
    const state2 = buildTimelineState(response2, null);
    
    const allEvents = [...state1.httpEvents, ...state2.httpEvents];
    
    expect(() => {
      sortTimelineEvents(allEvents);
    }).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Sorting with Unknown Values
// ---------------------------------------------------------------------------

describe('sorting with unknown enum values', () => {
  it('sorts events by canonical time regardless of probeKind normalization', () => {
    const events = [
      createEventWithFields({ eventId: 'evt-newer', probeKind: 'http' }),
      createEventWithFields({ eventId: 'evt-older', probeKind: 'icmp' }),
    ];
    
    // Manually adjust timestamps for testing
    events[0].canonicalTimeMs = new Date('2026-06-18T12:00:00Z').getTime();
    events[1].canonicalTimeMs = new Date('2026-06-18T11:00:00Z').getTime();
    
    const sorted = sortTimelineEvents(events);
    
    expect(sorted[0].eventId).toBe('evt-newer');
    expect(sorted[1].eventId).toBe('evt-older');
  });

  it('sorts unknown probeKind after known values with same timestamp', () => {
    const response1 = createValidSpikeResponse({ kind: undefined, event_id: 'evt-unknown' });
    const response2 = createValidSpikeResponse({ kind: 'http', event_id: 'evt-http' });
    
    const state1 = buildTimelineState(response1, null);
    const state2 = buildTimelineState(response2, null);
    
    const allEvents = [...state1.httpEvents, ...state2.httpEvents];
    
    // Same timestamp - sortProbeKind should be 2 for unknown, 0 for http
    allEvents[0].canonicalTimeMs = allEvents[1].canonicalTimeMs = new Date('2026-06-18T12:00:00Z').getTime();
    
    const sorted = sortTimelineEvents(allEvents);
    
    // HTTP (sortProbeKind=0) should come before unknown (sortProbeKind=2)
    expect(sorted[0].probeKind).toBe('http');
    expect(sorted[1].probeKind).toBe('unknown');
  });

  it('does not throw when sorting events with unknown values', () => {
    const events = [
      createEventWithFields({ eventId: 'evt-1', probeKind: 'http', severity: 'warning' }),
      createEventWithFields({ eventId: 'evt-2', probeKind: 'unknown', severity: 'unknown' }),
    ];
    
    expect(() => sortTimelineEvents(events)).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// upperLabel Defense-in-Depth Tests
// ---------------------------------------------------------------------------

describe('upperLabel defense-in-depth', () => {
  it('does not throw when event.probeKind is unknown', () => {
    const response = createValidSpikeResponse({ kind: undefined });
    const state = buildTimelineState(response, null);
    
    const event = state.httpEvents[0];
    
    // Simulate what view.ts does - this should not throw
    expect(() => {
      // Direct toUpperCase on unknown field (now safe with upperLabel fallback)
      const label = event.probeKind.toUpperCase();
      expect(label).toBe('UNKNOWN');
    }).not.toThrow();
  });

  it('does not throw when event.severity is unknown', () => {
    const response = createValidSpikeResponse({ severity: undefined });
    const state = buildTimelineState(response, null);
    
    const event = state.httpEvents[0];
    
    // Simulate what view.ts does - this should not throw
    expect(() => {
      const label = event.severity.toUpperCase();
      expect(label).toBe('UNKNOWN');
    }).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Summary Counts Match Table Rows (Invariant Test)
// ---------------------------------------------------------------------------

describe('summary counts match mergedEvents', () => {
  it('httpSummary.totalEvents equals httpEvents.length', () => {
    const response = createValidSpikeResponse({ kind: 'http' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpSummary.totalEvents).toBe(state.httpEvents.length);
  });

  it('icmpSummary.totalEvents equals icmpEvents.length', () => {
    // ICMP events should go through normalizeIcmpResponse, not normalizeHttpResponse
    const icmpResponse = createValidSpikeResponse({ kind: 'icmp' });
    const state = buildTimelineState(null, icmpResponse);
    
    expect(state.icmpSummary.totalEvents).toBe(state.icmpEvents.length);
  });

  it('mergedEvents length equals httpEvents + icmpEvents', () => {
    const httpResponse = createValidSpikeResponse({ kind: 'http' });
    const icmpResponse = createValidSpikeResponse({ kind: 'icmp' });
    const state = buildTimelineState(httpResponse, icmpResponse);
    
    expect(state.mergedEvents.length).toBe(state.httpEvents.length + state.icmpEvents.length);
  });

  it('summary counts reflect malformed events correctly', () => {
    // When kind is unknown, it should NOT appear in httpSummary
    const response = createValidSpikeResponse({ kind: undefined });
    const state = buildTimelineState(response, null);
    
    // Unknown events should NOT be counted in httpSummary or icmpSummary
    // because they don't match either 'http' or 'icmp' probeKind
    expect(state.httpSummary.totalEvents).toBe(0);
    expect(state.icmpSummary.totalEvents).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// Existing Valid Event Ordering Unchanged
// ---------------------------------------------------------------------------

describe('existing valid event ordering unchanged', () => {
  it('preserves correct probeKind for valid http events', () => {
    const response = createValidSpikeResponse({ kind: 'http' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].probeKind).toBe('http');
    expect(state.httpEvents[0].sortProbeKind).toBe(0);
  });

  it('preserves correct probeKind for valid icmp events', () => {
    const response = createValidSpikeResponse({ kind: 'icmp' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].probeKind).toBe('icmp');
    expect(state.httpEvents[0].sortProbeKind).toBe(1);
  });

  it('preserves correct severity for valid warning events', () => {
    const response = createValidSpikeResponse({ severity: 'warning' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].severity).toBe('warning');
    expect(state.httpEvents[0].sortSeverity).toBe(0);
  });

  it('preserves correct severity for valid critical events', () => {
    const response = createValidSpikeResponse({ severity: 'critical' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].severity).toBe('critical');
    expect(state.httpEvents[0].sortSeverity).toBe(1);
  });

  it('http events sort before icmp events with same timestamp', () => {
    const httpEvent = createEventWithFields({ eventId: 'http-evt', probeKind: 'http' });
    const icmpEvent = createEventWithFields({ eventId: 'icmp-evt', probeKind: 'icmp' });
    
    // Same timestamp
    httpEvent.canonicalTimeMs = icmpEvent.canonicalTimeMs = new Date('2026-06-18T12:00:00Z').getTime();
    
    const sorted = sortTimelineEvents([icmpEvent, httpEvent]);
    
    expect(sorted[0].probeKind).toBe('http');
    expect(sorted[1].probeKind).toBe('icmp');
  });

  it('warning events sort before critical events with same timestamp and probeKind', () => {
    const warningEvent = createEventWithFields({ eventId: 'warn-evt', severity: 'warning' });
    const criticalEvent = createEventWithFields({ eventId: 'crit-evt', severity: 'critical' });
    
    // Same timestamp and probeKind
    warningEvent.canonicalTimeMs = criticalEvent.canonicalTimeMs = new Date('2026-06-18T12:00:00Z').getTime();
    warningEvent.sortProbeKind = criticalEvent.sortProbeKind = 0;
    
    const sorted = sortTimelineEvents([criticalEvent, warningEvent]);
    
    expect(sorted[0].severity).toBe('warning');
    expect(sorted[1].severity).toBe('critical');
  });
});
