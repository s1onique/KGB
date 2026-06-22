// Diagnostic Timeline Render Tests - DOM renderer tests

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  renderSummaryCard,
  renderSummaryCards,
  renderFilterControls,
  renderTimelineTable,
  renderTimelineRow,
  renderExpandedPanel,
  renderLoadingState,
  renderErrorState,
  renderEmptyState,
  renderTimeline,
} from './diagnosticTimeline.render';
import { createTimelineStateWithEvents, createEmptyTimelineState, createLoadingTimelineState, createHttpTimelineEvent, createIcmpTimelineEvent, createCriticalTimelineEvent, defaultTestFilters } from './diagnosticTimeline.fixtures';
import type { TimelineEvent, ProbeKindSummary } from './diagnosticTimeline.model';
import type { TimelineFilters } from './diagnosticTimeline.filters';

// Mock the api module
const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) =>
      mockGetLatencySpikesWithCaptures(...args),
  },
}));

describe('diagnosticTimeline.render', () => {
  let container: HTMLElement;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
  });

  describe('renderSummaryCard', () => {
    it('renders HTTP summary card correctly', () => {
      const summary: ProbeKindSummary = {
        probeKind: 'http',
        totalEvents: 5,
        capturedCount: 3,
        suppressedCount: 1,
        failedCount: 1,
        criticalCount: 2,
        warningCount: 3,
      };
      
      const html = renderSummaryCard(summary);
      expect(html).toContain('HTTP');
      expect(html).toContain('5 events');
      expect(html).toContain('3'); // captured
      expect(html).toContain('1'); // suppressed/failed
      expect(html).toContain('probe-http');
    });

    it('renders ICMP summary card correctly', () => {
      const summary: ProbeKindSummary = {
        probeKind: 'icmp',
        totalEvents: 3,
        capturedCount: 2,
        suppressedCount: 0,
        failedCount: 1,
        criticalCount: 1,
        warningCount: 2,
      };
      
      const html = renderSummaryCard(summary);
      expect(html).toContain('ICMP');
      expect(html).toContain('3 events');
      expect(html).toContain('probe-icmp');
    });
  });

  describe('renderSummaryCards', () => {
    it('renders both HTTP and ICMP summary cards', () => {
      const httpSummary: ProbeKindSummary = {
        probeKind: 'http',
        totalEvents: 5,
        capturedCount: 3,
        suppressedCount: 1,
        failedCount: 1,
        criticalCount: 2,
        warningCount: 3,
      };
      const icmpSummary: ProbeKindSummary = {
        probeKind: 'icmp',
        totalEvents: 3,
        capturedCount: 2,
        suppressedCount: 0,
        failedCount: 1,
        criticalCount: 1,
        warningCount: 2,
      };
      
      const html = renderSummaryCards(httpSummary, icmpSummary);
      expect(html).toContain('timeline-summary-container');
      expect(html).toContain('HTTP');
      expect(html).toContain('ICMP');
    });
  });

  describe('renderFilterControls', () => {
    it('renders filter controls with all options', () => {
      const filters: TimelineFilters = {
        probeKind: 'all',
        captureStatus: 'all',
        severity: 'all',
      };
      
      const html = renderFilterControls(filters, 10, 10);
      expect(html).toContain('filter-select');
      expect(html).toContain('Showing 10 of 10');
      expect(html).not.toContain('filter-reset-btn'); // No reset when no filters active
    });

    it('shows reset button when filters are active', () => {
      const filters: TimelineFilters = {
        probeKind: 'http',
        captureStatus: 'all',
        severity: 'all',
      };
      
      const html = renderFilterControls(filters, 10, 5);
      expect(html).toContain('filter-reset-btn');
      expect(html).toContain('Showing 5 of 10');
    });

    it('preserves selected values', () => {
      const filters: TimelineFilters = {
        probeKind: 'icmp',
        captureStatus: 'captured',
        severity: 'critical',
      };
      
      const html = renderFilterControls(filters, 10, 2);
      expect(html).toContain('selected');
      expect(html).toContain('value="icmp"');
      expect(html).toContain('value="captured"');
      expect(html).toContain('value="critical"');
    });
  });

  describe('renderTimelineRow', () => {
    it('renders HTTP event row correctly', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-1',
        latencyMs: 1234,
        severity: 'warning',
        captureStatus: 'captured',
      });
      
      const html = renderTimelineRow(event, 0);
      expect(html).toContain('HTTP');
      expect(html).toContain('probe-http');
      expect(html).toContain('WARNING');
      expect(html).toContain('captured');
      expect(html).toContain('timeline-row');
    });

    it('renders ICMP event row correctly', () => {
      const event = createIcmpTimelineEvent({
        eventId: 'evt-2',
        latencyMs: 500,
        severity: 'critical',
        captureStatus: 'failed',
      });
      
      const html = renderTimelineRow(event, 1);
      expect(html).toContain('ICMP');
      expect(html).toContain('probe-icmp');
      expect(html).toContain('CRITICAL');
      expect(html).toContain('failed');
    });

    it('shows error text for failed captures', () => {
      const event = createHttpTimelineEvent({
        captureStatus: 'failed',
      });
      event.primaryCapture = {
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'error',
        error: 'connection refused',
      };
      
      const html = renderTimelineRow(event, 0);
      expect(html).toContain('connection refused');
    });
  });

  describe('renderTimelineTable', () => {
    it('renders empty state when no events', () => {
      const html = renderTimelineTable([]);
      expect(html).toContain('timeline-empty');
      expect(html).toContain('No diagnostic events');
    });

    it('renders table with events', () => {
      const events: TimelineEvent[] = [
        createHttpTimelineEvent({ eventId: 'evt-1' }),
        createIcmpTimelineEvent({ eventId: 'evt-2' }),
      ];
      
      const html = renderTimelineTable(events);
      expect(html).toContain('timeline-table');
      expect(html).toContain('evt-1');
      expect(html).toContain('evt-2');
      expect(html).toContain('timeline-expanded-panels');
    });
  });

  describe('renderExpandedPanel', () => {
    it('renders event metadata', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-123',
        latencyMs: 1234,
        sampleTs: '2026-06-18T12:00:00Z',
        collectedAt: '2026-06-18T12:00:01Z',
        rollingMedianMs: 100,
      });
      
      const html = renderExpandedPanel(event, 0);
      expect(html).toContain('evt-123');
      expect(html).toContain('Event ID');
      expect(html).toContain('latency-value');
      expect(html).toContain('timeline-expanded-panel');
    });

    it('includes action buttons', () => {
      const event = createHttpTimelineEvent();
      
      const html = renderExpandedPanel(event, 0);
      expect(html).toContain('copy-btn');
      expect(html).toContain('download-btn');
    });
  });

  describe('renderLoadingState', () => {
    it('renders loading spinner and text', () => {
      const html = renderLoadingState();
      expect(html).toContain('timeline-loading');
      expect(html).toContain('spinner');
      expect(html).toContain('Loading');
    });
  });

  describe('renderErrorState', () => {
    it('renders error message', () => {
      const html = renderErrorState('Network error');
      expect(html).toContain('timeline-error');
      expect(html).toContain('Network error');
      expect(html).toContain('⚠');
    });
  });

  describe('renderEmptyState', () => {
    it('renders empty state message', () => {
      const html = renderEmptyState();
      expect(html).toContain('timeline-empty-state');
      expect(html).toContain('No Diagnostic Events');
      expect(html).toContain('📭');
    });
  });

  describe('renderTimeline', () => {
    it('renders loading state when isLoading is true', () => {
      const state = createLoadingTimelineState();
      
      renderTimeline(container, state, defaultTestFilters, []);
      
      expect(container.innerHTML).toContain('timeline-loading');
    });

    it('renders error state when error is set', () => {
      const state = createEmptyTimelineState();
      state.error = 'API error';
      
      renderTimeline(container, state, defaultTestFilters, []);
      
      expect(container.innerHTML).toContain('timeline-error');
      expect(container.innerHTML).toContain('API error');
    });

    it('renders empty state when no events', () => {
      const state = createEmptyTimelineState();
      
      renderTimeline(container, state, defaultTestFilters, []);
      
      expect(container.innerHTML).toContain('timeline-empty-state');
      expect(container.innerHTML).toContain('No Diagnostic Events');
    });

    it('renders timeline with events and summary cards', () => {
      const events = [
        createHttpTimelineEvent({ eventId: 'evt-1' }),
        createIcmpTimelineEvent({ eventId: 'evt-2' }),
      ];
      const state = createTimelineStateWithEvents(events);
      
      renderTimeline(container, state, defaultTestFilters, events);
      
      expect(container.innerHTML).toContain('timeline-summary-container');
      expect(container.innerHTML).toContain('timeline-filters');
      expect(container.innerHTML).toContain('timeline-table');
      expect(container.innerHTML).toContain('evt-1');
      expect(container.innerHTML).toContain('evt-2');
    });
  });
});
