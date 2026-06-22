// Diagnostic Timeline Update Tests - Pure update function tests (no DOM, no async)

import { describe, it, expect } from 'vitest';
import { createInitialModel, defaultFilters, defaultPagination, applyFilters, clampPageIndex, getFirstVisibleRow, getPageIndexForRow, computeProbeKindSummary } from './model';
import { update, verifyInvariants } from './update';
import type { TimelineEvent, TimelineModel } from '../diagnosticTimeline.model';
import type { TimelineMsg } from './msg';

// ---------------------------------------------------------------------------
// Test Fixtures
// ---------------------------------------------------------------------------

/** Create a test event */
function createTestEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  const eventId = overrides.eventId ?? 'evt-1';
  const probeKind = overrides.probeKind ?? 'http';
  const severity = overrides.severity ?? 'warning';
  const sampleTs = overrides.sampleTs ?? '2026-06-18T12:00:00Z';
  const canonicalTime = new Date(sampleTs);
  
  return {
    eventId,
    targetId: overrides.targetId ?? 'test-target',
    probeKind: probeKind as 'http' | 'icmp',
    severity: severity as 'warning' | 'critical',
    latencyMs: overrides.latencyMs ?? 1234,
    sampleTs,
    collectedAt: overrides.collectedAt ?? sampleTs,
    reasons: overrides.reasons ?? [],
    rollingMedianMs: overrides.rollingMedianMs ?? 100,
    thresholds: {
      warningMs: 500,
      criticalMs: 1000,
      relativeMultiplier: 10,
    },
    captures: overrides.captures ?? [],
    primaryCapture: overrides.primaryCapture ?? null,
    captureStatus: overrides.captureStatus ?? 'captured',
    canonicalTime,
    sortProbeKind: probeKind === 'http' ? 0 : 1,
    sortSeverity: severity === 'warning' ? 0 : 1,
    sortEventId: eventId,
  };
}

/** Create a model with test events */
function createModelWithEvents(events: TimelineEvent[], overrides: Partial<TimelineModel> = {}): TimelineModel {
  return {
    ...createInitialModel(),
    mergedEvents: events,
    httpSummary: computeProbeKindSummary('http', events),
    icmpSummary: computeProbeKindSummary('icmp', events),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Pure Update Tests
// ---------------------------------------------------------------------------

describe('update', () => {
  describe('LoadStarted', () => {
    it('sets isLoading to true', () => {
      const model = createModelWithEvents([]);
      const result = update(model, { type: 'LoadStarted' });
      
      expect(result.model.isLoading).toBe(true);
      expect(result.model.error).toBeNull();
    });
  });

  describe('LoadFailed', () => {
    it('sets error and isLoading to false', () => {
      const model = createModelWithEvents([]);
      const result = update(model, { type: 'LoadFailed', error: 'Network error' });
      
      expect(result.model.isLoading).toBe(false);
      expect(result.model.error).toBe('Network error');
    });
  });

  describe('TimelineLoaded', () => {
    it('updates merged events', () => {
      const model = createModelWithEvents([]);
      const events = [
        createTestEvent({ eventId: 'evt-1' }),
        createTestEvent({ eventId: 'evt-2' }),
      ];
      const result = update(model, { type: 'TimelineLoaded', events });
      
      expect(result.model.mergedEvents).toHaveLength(2);
      expect(result.model.isLoading).toBe(false);
      expect(result.model.error).toBeNull();
    });

    it('preserves expanded rows for surviving event IDs', () => {
      const model = createModelWithEvents([
        createTestEvent({ eventId: 'evt-1' }),
        createTestEvent({ eventId: 'evt-2' }),
        createTestEvent({ eventId: 'evt-3' }),
      ], {
        expandedEventIds: new Set(['evt-1', 'evt-2', 'evt-3']),
      });
      
      const events = [
        createTestEvent({ eventId: 'evt-1' }),
        createTestEvent({ eventId: 'evt-2' }),
        // evt-3 no longer exists
      ];
      const result = update(model, { type: 'TimelineLoaded', events });
      
      expect(result.model.expandedEventIds).toEqual(new Set(['evt-1', 'evt-2']));
    });

    it('clamps pagination to valid range', () => {
      const model = createModelWithEvents(
        Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` })),
        {
          pagination: { pageIndex: 10, pageSize: 20 }, // Page 10 doesn't exist after filtering
        }
      );
      
      const events: TimelineEvent[] = []; // Empty events
      const result = update(model, { type: 'TimelineLoaded', events });
      
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('PageChanged', () => {
    it('updates page index', () => {
      const model = createModelWithEvents(
        Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` }))
      );
      const result = update(model, { type: 'PageChanged', page: 3 });
      
      expect(result.model.pagination.pageIndex).toBe(3);
    });

    it('clamps page index to valid range', () => {
      const model = createModelWithEvents(
        Array(50).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` })),
        { pagination: { pageIndex: 0, pageSize: 20 } }
      );
      const result = update(model, { type: 'PageChanged', page: 100 });
      
      expect(result.model.pagination.pageIndex).toBe(2); // 3 pages max
    });

    it('clamps negative page index to 0', () => {
      const model = createModelWithEvents([createTestEvent()]);
      const result = update(model, { type: 'PageChanged', page: -5 });
      
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('PageSizeChanged', () => {
    it('updates page size', () => {
      const model = createModelWithEvents([createTestEvent()]);
      const result = update(model, { type: 'PageSizeChanged', pageSize: 50 });
      
      expect(result.model.pagination.pageSize).toBe(50);
    });

    it('preserves first visible row when increasing page size', () => {
      // Start on page 2 with page size 20 (rows 40-59 visible)
      const model = createModelWithEvents(
        Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` })),
        {
          pagination: { pageIndex: 2, pageSize: 20 },
        }
      );
      
      const result = update(model, { type: 'PageSizeChanged', pageSize: 50 });
      
      // First visible row (40) should be on page 0 with size 50
      expect(result.model.pagination.pageIndex).toBe(0);
      expect(result.model.pagination.pageSize).toBe(50);
    });

    it('preserves first visible row when decreasing page size', () => {
      // Start on page 1 with page size 20 (rows 20-39 visible)
      const model = createModelWithEvents(
        Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` })),
        {
          pagination: { pageIndex: 1, pageSize: 20 },
        }
      );
      
      const result = update(model, { type: 'PageSizeChanged', pageSize: 10 });
      
      // First visible row (20) should be on page 2 with size 10
      expect(result.model.pagination.pageIndex).toBe(2);
      expect(result.model.pagination.pageSize).toBe(10);
    });

    it('resets to page 0 when no events', () => {
      const model = createModelWithEvents([], {
        pagination: { pageIndex: 5, pageSize: 20 },
      });
      const result = update(model, { type: 'PageSizeChanged', pageSize: 50 });
      
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('FilterChanged', () => {
    it('updates filters', () => {
      const model = createModelWithEvents([]);
      const result = update(model, { type: 'FilterChanged', filters: { probeKind: 'http' } });
      
      expect(result.model.filters.probeKind).toBe('http');
    });

    it('resets page index to 0 when filters change', () => {
      const model = createModelWithEvents(
        Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` })),
        { pagination: { pageIndex: 5, pageSize: 20 } }
      );
      const result = update(model, { type: 'FilterChanged', filters: { probeKind: 'http' } });
      
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('FiltersReset', () => {
    it('resets all filters to default', () => {
      const model = createModelWithEvents([], {
        filters: { probeKind: 'http', captureStatus: 'captured', severity: 'critical' },
      });
      const result = update(model, { type: 'FiltersReset' });
      
      expect(result.model.filters.probeKind).toBe('all');
      expect(result.model.filters.captureStatus).toBe('all');
      expect(result.model.filters.severity).toBe('all');
    });

    it('resets page index to 0', () => {
      const model = createModelWithEvents(
        Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` })),
        { pagination: { pageIndex: 5, pageSize: 20 } }
      );
      const result = update(model, { type: 'FiltersReset' });
      
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('RowToggled', () => {
    it('adds eventId to expanded set when not present', () => {
      const model = createModelWithEvents([createTestEvent({ eventId: 'evt-1' })]);
      const result = update(model, { type: 'RowToggled', eventId: 'evt-1' });
      
      expect(result.model.expandedEventIds.has('evt-1')).toBe(true);
    });

    it('removes eventId from expanded set when present', () => {
      const model = createModelWithEvents([createTestEvent({ eventId: 'evt-1' })], {
        expandedEventIds: new Set(['evt-1']),
      });
      const result = update(model, { type: 'RowToggled', eventId: 'evt-1' });
      
      expect(result.model.expandedEventIds.has('evt-1')).toBe(false);
    });
  });

  describe('RefreshRequested', () => {
    it('does not change model', () => {
      const model = createModelWithEvents([createTestEvent()]);
      const result = update(model, { type: 'RefreshRequested' });
      
      expect(result.model).toBe(model);
    });
  });
});

// ---------------------------------------------------------------------------
// Pagination Utility Tests
// ---------------------------------------------------------------------------

describe('pagination utilities', () => {
  describe('clampPageIndex', () => {
    it('clamps negative index to 0', () => {
      expect(clampPageIndex(-1, 5)).toBe(0);
    });

    it('clamps index exceeding total to last page', () => {
      expect(clampPageIndex(10, 5)).toBe(4);
    });

    it('returns 0 for zero total pages', () => {
      expect(clampPageIndex(0, 0)).toBe(0);
    });

    it('returns valid index within range', () => {
      expect(clampPageIndex(2, 5)).toBe(2);
    });
  });

  describe('getFirstVisibleRow', () => {
    it('calculates correct first visible row', () => {
      expect(getFirstVisibleRow(0, 20)).toBe(0);
      expect(getFirstVisibleRow(1, 20)).toBe(20);
      expect(getFirstVisibleRow(2, 20)).toBe(40);
    });
  });

  describe('getPageIndexForRow', () => {
    it('calculates correct page index for row', () => {
      expect(getPageIndexForRow(0, 20)).toBe(0);
      expect(getPageIndexForRow(15, 20)).toBe(0);
      expect(getPageIndexForRow(20, 20)).toBe(1);
      expect(getPageIndexForRow(45, 20)).toBe(2);
    });
  });
});

// ---------------------------------------------------------------------------
// Invariant Tests
// ---------------------------------------------------------------------------

describe('verifyInvariants', () => {
  it('returns no errors for valid model', () => {
    const model = createModelWithEvents([
      createTestEvent({ eventId: 'evt-1' }),
    ]);
    const errors = verifyInvariants(model);
    
    expect(errors).toHaveLength(0);
  });

  it('detects negative page index', () => {
    const model = createModelWithEvents([createTestEvent()], {
      pagination: { pageIndex: -1, pageSize: 20 },
    });
    const errors = verifyInvariants(model);
    
    expect(errors.some(e => e.includes('negative'))).toBe(true);
  });

  it('detects invalid page size', () => {
    const model = createModelWithEvents([createTestEvent()], {
      pagination: { pageIndex: 0, pageSize: 15 }, // 15 is not a valid option
    });
    const errors = verifyInvariants(model);
    
    expect(errors.some(e => e.includes('Invalid page size'))).toBe(true);
  });

  it('detects expanded IDs not in events', () => {
    const model = createModelWithEvents([createTestEvent({ eventId: 'evt-1' })], {
      expandedEventIds: new Set(['evt-1', 'evt-2']), // evt-2 doesn't exist
    });
    const errors = verifyInvariants(model);
    
    expect(errors.some(e => e.includes('evt-2'))).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Filter Tests
// ---------------------------------------------------------------------------

describe('applyFilters', () => {
  const events = [
    createTestEvent({ eventId: 'http-warn-1', probeKind: 'http', severity: 'warning' }),
    createTestEvent({ eventId: 'http-crit-1', probeKind: 'http', severity: 'critical' }),
    createTestEvent({ eventId: 'icmp-warn-1', probeKind: 'icmp', severity: 'warning' }),
    createTestEvent({ eventId: 'icmp-crit-1', probeKind: 'icmp', severity: 'critical' }),
  ];

  it('returns all events when no filters active', () => {
    const filtered = applyFilters(events, defaultFilters);
    expect(filtered).toHaveLength(4);
  });

  it('filters by probe kind', () => {
    const filtered = applyFilters(events, { ...defaultFilters, probeKind: 'http' });
    expect(filtered).toHaveLength(2);
    expect(filtered.every(e => e.probeKind === 'http')).toBe(true);
  });

  it('filters by severity', () => {
    const filtered = applyFilters(events, { ...defaultFilters, severity: 'critical' });
    expect(filtered).toHaveLength(2);
    expect(filtered.every(e => e.severity === 'critical')).toBe(true);
  });
});
