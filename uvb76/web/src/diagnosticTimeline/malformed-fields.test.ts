// Diagnostic Timeline Malformed Fields Regression Tests
// Tests for the "Cannot read properties of undefined (reading 'toUpperCase')" fix

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
  };
}

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

  it('normalizes missing kind to http (default)', () => {
    const response = createValidSpikeResponse({ kind: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].probeKind).toBe('http');
  });

  it('normalizes invalid kind to http (default)', () => {
    const response = createValidSpikeResponse({ kind: 'garbage' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].probeKind).toBe('http');
  });

  it('preserves icmp when kind is icmp', () => {
    const response = createValidSpikeResponse({ kind: 'icmp' });
    const state = buildTimelineState(response, null);
    
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

  it('normalizes missing severity to warning (default)', () => {
    const response = createValidSpikeResponse({ severity: undefined });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].severity).toBe('warning');
  });

  it('normalizes invalid severity to warning (default)', () => {
    const response = createValidSpikeResponse({ severity: 'garbage' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].severity).toBe('warning');
  });

  it('preserves critical when severity is critical', () => {
    const response = createValidSpikeResponse({ severity: 'critical' });
    const state = buildTimelineState(response, null);
    
    expect(state.httpEvents[0].severity).toBe('critical');
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

  it('produces valid TimelineEvent when all fields are malformed', () => {
    const response = createValidSpikeResponse({
      event_id: undefined,
      target_id: undefined,
      kind: undefined,
      severity: undefined,
      sample_ts: 'not-a-timestamp',  // Use clearly invalid value
    });
    const state = buildTimelineState(response, null);
    
    const event = state.httpEvents[0];
    
    // All fields should be normalized to valid values
    expect(event.eventId).toBeTruthy();
    expect(event.eventId.length).toBeGreaterThan(0);
    expect(event.targetId).toBe('unknown-target');
    expect(event.probeKind).toBe('http');
    expect(event.severity).toBe('warning');
    // Note: sampleTs is the raw value from API; canonicalTimeMs/timeStatus reflect parsing result
    expect(event.sampleTs).toBeTruthy();
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
// Sorting with Normalized Enum Fields
// ---------------------------------------------------------------------------

describe('sorting with normalized enum fields', () => {
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

  it('does not throw when sorting events with normalized default values', () => {
    const events = [
      createEventWithFields({ eventId: 'evt-1', probeKind: 'http', severity: 'warning' }),
      createEventWithFields({ eventId: 'evt-2', probeKind: 'http', severity: 'warning' }),
    ];
    
    expect(() => sortTimelineEvents(events)).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// upperLabel Defense-in-Depth Tests (simulated)
// ---------------------------------------------------------------------------

describe('upperLabel defense-in-depth simulation', () => {
  it('does not throw when event.probeKind is used in toUpperCase simulation', () => {
    const response = createValidSpikeResponse({ kind: undefined });
    const state = buildTimelineState(response, null);
    
    const event = state.httpEvents[0];
    
    // Simulate what view.ts does - this should not throw
    expect(() => {
      // Direct toUpperCase on normalized field (now safe)
      const label = event.probeKind.toUpperCase();
      expect(label).toBe('HTTP');
    }).not.toThrow();
  });

  it('does not throw when event.severity is used in toUpperCase simulation', () => {
    const response = createValidSpikeResponse({ severity: undefined });
    const state = buildTimelineState(response, null);
    
    const event = state.httpEvents[0];
    
    // Simulate what view.ts does - this should not throw
    expect(() => {
      const label = event.severity.toUpperCase();
      expect(label).toBe('WARNING');
    }).not.toThrow();
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
