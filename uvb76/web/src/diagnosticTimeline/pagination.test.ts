// Diagnostic Timeline Pagination Tests - Elm-ish pagination tests
// Tests the pure update function for pagination/filter/refresh/expanded-row invariants

import { describe, it, expect } from 'vitest';
import { 
  createInitialModel, 
  PAGE_SIZE_OPTIONS, 
  clampPageIndex, 
  getPageIndexForRow, 
  getFirstVisibleRow,
  applyFilters,
  defaultFilters,
  calculatePagination,
  computeProbeKindSummary,
} from './model';
import { update, verifyInvariants } from './update';
import type { TimelineEvent, TimelineModel } from '../diagnosticTimeline.model';

// ---------------------------------------------------------------------------
// Test Fixtures
// ---------------------------------------------------------------------------

/** Create a test event matching the TimelineEvent interface */
function createTestEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  const eventId = overrides.eventId ?? 'evt-1';
  const probeKind = overrides.probeKind ?? 'http';
  const severity = overrides.severity ?? 'warning';
  const sampleTs = overrides.sampleTs ?? '2026-06-18T12:00:00Z';
  
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
    canonicalTime: new Date(sampleTs),
    sortProbeKind: probeKind === 'http' ? 0 : 1,
    sortSeverity: severity === 'warning' ? 0 : 1,
    sortEventId: eventId,
  };
}

/** Create a model with events and optional overrides */
function createModelWithEvents(events: TimelineEvent[], overrides?: Partial<TimelineModel>): TimelineModel {
  const baseModel = createInitialModel();
  return {
    ...baseModel,
    ...overrides,
    mergedEvents: events,
    httpSummary: computeProbeKindSummary('http', events),
    icmpSummary: computeProbeKindSummary('icmp', events),
  };
}

// ---------------------------------------------------------------------------
// Pagination Utility Tests
// ---------------------------------------------------------------------------

describe('pagination utilities', () => {
  describe('PAGE_SIZE_OPTIONS', () => {
    it('contains expected page size options', () => {
      expect(PAGE_SIZE_OPTIONS).toEqual([10, 20, 50, 100]);
    });
  });

  describe('clampPageIndex', () => {
    it('returns 0 for negative index', () => {
      expect(clampPageIndex(-1, 5)).toBe(0);
    });

    it('returns 0 for zero total pages', () => {
      expect(clampPageIndex(0, 0)).toBe(0);
    });

    it('clamps index exceeding total pages', () => {
      expect(clampPageIndex(10, 5)).toBe(4);
    });

    it('returns valid index within range', () => {
      expect(clampPageIndex(2, 5)).toBe(2);
    });
  });

  describe('getPageIndexForRow', () => {
    it('returns correct page index for row', () => {
      expect(getPageIndexForRow(0, 20)).toBe(0);
      expect(getPageIndexForRow(15, 20)).toBe(0);
      expect(getPageIndexForRow(20, 20)).toBe(1);
      expect(getPageIndexForRow(45, 20)).toBe(2);
    });
  });

  describe('getFirstVisibleRow', () => {
    it('returns correct first visible row', () => {
      expect(getFirstVisibleRow(0, 20)).toBe(0);
      expect(getFirstVisibleRow(1, 20)).toBe(20);
      expect(getFirstVisibleRow(2, 20)).toBe(40);
    });
  });

  describe('calculatePagination', () => {
    it('returns correct values for default pagination', () => {
      const pagination = { pageIndex: 0, pageSize: 20 };
      const result = calculatePagination(100, pagination);
      
      expect(result.totalPages).toBe(5);
      expect(result.safePageIndex).toBe(0);
      expect(result.start).toBe(0);
      expect(result.end).toBe(20);
    });

    it('clamps page index to valid range', () => {
      const pagination = { pageIndex: 10, pageSize: 20 };
      const result = calculatePagination(100, pagination);
      
      expect(result.safePageIndex).toBe(4);
      expect(result.start).toBe(80);
      expect(result.end).toBe(100);
    });

    it('handles zero filtered count', () => {
      const pagination = { pageIndex: 0, pageSize: 20 };
      const result = calculatePagination(0, pagination);
      
      expect(result.totalPages).toBe(1);
      expect(result.safePageIndex).toBe(0);
    });
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

// ---------------------------------------------------------------------------
// Pure Update Tests
// ---------------------------------------------------------------------------

describe('update pagination invariants', () => {
  describe('PageChanged', () => {
    it('updates page index', () => {
      const events = Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` }));
      const model = createModelWithEvents(events);
      const result = update(model, { type: 'PageChanged', page: 3 });
      
      expect(result.model.pagination.pageIndex).toBe(3);
    });

    it('clamps page index to valid range', () => {
      const events = Array(50).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` }));
      const model = createModelWithEvents(events);
      const result = update(model, { type: 'PageChanged', page: 100 });
      
      expect(result.model.pagination.pageIndex).toBe(2);
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
      const events = Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` }));
      const model = createModelWithEvents(events, { pagination: { pageIndex: 2, pageSize: 20 } });
      const result = update(model, { type: 'PageSizeChanged', pageSize: 50 });
      
      // First visible row (40) should be on page 0 with size 50
      expect(result.model.pagination.pageIndex).toBe(0);
      expect(result.model.pagination.pageSize).toBe(50);
    });

    it('preserves first visible row when decreasing page size', () => {
      const events = Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` }));
      const model = createModelWithEvents(events, { pagination: { pageIndex: 1, pageSize: 20 } });
      const result = update(model, { type: 'PageSizeChanged', pageSize: 10 });
      
      // First visible row (20) should be on page 2 with size 10
      expect(result.model.pagination.pageIndex).toBe(2);
      expect(result.model.pagination.pageSize).toBe(10);
    });

    it('resets to page 0 when no events', () => {
      const model = createModelWithEvents([], { pagination: { pageIndex: 5, pageSize: 20 } });
      const result = update(model, { type: 'PageSizeChanged', pageSize: 50 });
      
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('FilterChanged resets page', () => {
    it('resets page index to 0 when filter changes', () => {
      const events = Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` }));
      const model = createModelWithEvents(events, { pagination: { pageIndex: 5, pageSize: 20 } });
      const result = update(model, { type: 'FilterChanged', filters: { probeKind: 'http' } });
      
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('FiltersReset', () => {
    it('resets all filters and page index', () => {
      const model = createModelWithEvents([], {
        filters: { probeKind: 'http', captureStatus: 'captured', severity: 'critical' },
        pagination: { pageIndex: 5, pageSize: 20 },
      });
      const result = update(model, { type: 'FiltersReset' });
      
      expect(result.model.filters.probeKind).toBe('all');
      expect(result.model.filters.captureStatus).toBe('all');
      expect(result.model.filters.severity).toBe('all');
      expect(result.model.pagination.pageIndex).toBe(0);
    });
  });

  describe('TimelineLoaded', () => {
    it('preserves expanded rows for surviving event IDs', () => {
      const model = createModelWithEvents(
        [
          createTestEvent({ eventId: 'evt-1' }),
          createTestEvent({ eventId: 'evt-2' }),
          createTestEvent({ eventId: 'evt-3' }),
        ],
        { expandedEventIds: new Set(['evt-1', 'evt-2', 'evt-3']) }
      );
      
      const events = [
        createTestEvent({ eventId: 'evt-1' }),
        createTestEvent({ eventId: 'evt-2' }),
        // evt-3 no longer exists
      ];
      const result = update(model, { type: 'TimelineLoaded', events });
      
      expect(result.model.expandedEventIds).toEqual(new Set(['evt-1', 'evt-2']));
    });

    it('clamps pagination to valid range after data changes', () => {
      const model = createModelWithEvents(
        Array(100).fill(null).map((_, i) => createTestEvent({ eventId: `evt-${i}` })),
        { pagination: { pageIndex: 10, pageSize: 20 } }
      );
      
      const events: TimelineEvent[] = [];
      const result = update(model, { type: 'TimelineLoaded', events });
      
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
      const model = createModelWithEvents(
        [createTestEvent({ eventId: 'evt-1' })],
        { expandedEventIds: new Set(['evt-1']) }
      );
      const result = update(model, { type: 'RowToggled', eventId: 'evt-1' });
      
      expect(result.model.expandedEventIds.has('evt-1')).toBe(false);
    });
  });
});

// ---------------------------------------------------------------------------
// Invariant Tests
// ---------------------------------------------------------------------------

describe('verifyInvariants', () => {
  it('returns no errors for valid model', () => {
    const model = createModelWithEvents([createTestEvent({ eventId: 'evt-1' })]);
    const errors = verifyInvariants(model);
    
    expect(errors).toHaveLength(0);
  });

  it('detects negative page index', () => {
    const model = createModelWithEvents(
      [createTestEvent()],
      { pagination: { pageIndex: -1, pageSize: 20 } }
    );
    const errors = verifyInvariants(model);
    
    expect(errors.some(e => e.includes('negative'))).toBe(true);
  });

  it('detects invalid page size', () => {
    const model = createModelWithEvents(
      [createTestEvent()],
      { pagination: { pageIndex: 0, pageSize: 15 } }
    );
    const errors = verifyInvariants(model);
    
    expect(errors.some(e => e.includes('Invalid page size'))).toBe(true);
  });
});
