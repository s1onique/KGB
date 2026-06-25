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
import { createTimelineStateWithEvents, createEmptyTimelineState, createLoadingTimelineState, createHttpTimelineEvent, createIcmpTimelineEvent, createCriticalTimelineEvent, createPinnedAnchorsSpikeResponse, defaultTestFilters, createNativeTcpQuality, createSsTcpQuality, createSyntheticTcpQuality, createUnavailableTcpQuality } from './diagnosticTimeline.fixtures';
import { normalizeHttpResponse } from './diagnosticTimeline.model';
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

  // -------------------------------------------------------------------------
  // Anchor Provenance Tests
  // -------------------------------------------------------------------------

  describe('Anchor Provenance Rendering', () => {
    it('renders anchor badge for pinned anchor events', () => {
      // Create event with pinned anchor via cooldown_info
      const event = createHttpTimelineEvent({
        eventId: 'evt-pinned',
        captureStatus: 'suppressed',
      });
      
      // Add cooldown info with pinned anchor
      event.primaryCapture = {
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
        cooldown_info: {
          scope: 'per_diagnostic_peer',
          last_successful_capture_at: '2026-06-18T11:55:00Z',
          next_capture_eligible_at: '2026-06-18T12:05:00Z',
          remaining_cooldown_ms: 300000,
          cooldown_key: 'peer-1',
          anchor_visible: true,
          anchor_artifact_visible: true,
          anchor_timeline_visible: true,
          anchor_visibility_reason: 'pinned_anchor',
          skipped_attempt_updates_cooldown: false,
          cooldown_seconds: 300,
          anchor_capture_id: 'icmp-anchor-001',
          anchor_target_id: 'test-target',
          anchor_probe_kind: 'icmp',
          anchor_source: 'peer-1',
          suppressed_probe_kind: 'http',
          is_cross_probe_suppression: false,
        },
      };
      
      const html = renderTimelineRow(event, 0);
      
      // Should render anchor badge
      expect(html).toContain('anchor-badge');
      expect(html).toContain('anchor');
      // Should render with pinned class
      expect(html).toContain('timeline-row-pinned');
    });

    it('renders degraded badge for suppressed events with degraded quality', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-degraded',
        captureStatus: 'suppressed',
      });
      
      event.primaryCapture = {
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
        cooldown_info: {
          scope: 'per_diagnostic_peer',
          last_successful_capture_at: '2026-06-18T11:55:00Z',
          next_capture_eligible_at: '2026-06-18T12:05:00Z',
          remaining_cooldown_ms: 300000,
          cooldown_key: 'peer-1',
          anchor_visible: true,
          anchor_artifact_visible: true,
          anchor_timeline_visible: true,
          anchor_visibility_reason: 'retained_visible',
          skipped_attempt_updates_cooldown: false,
          cooldown_seconds: 300,
          anchor_capture_id: 'icmp-anchor-001',
          anchor_target_id: 'test-target',
          anchor_probe_kind: 'icmp',
          anchor_source: 'peer-1',
          suppressed_probe_kind: 'http',
          is_cross_probe_suppression: false,
          suppression_degraded: true,
          suppression_degraded_reason: 'Anchor capture incomplete',
          anchor_event_summary: {
            event_id: 'anchor-evt-001',
            capture_id: 'anchor-cap-001',
            probe_kind: 'icmp',
            severity: 'ok',
            latency_ms: 45,
            sample_ts: '2026-06-18T11:55:00Z',
            capture_status: 'ok',
            source: 'peer-1',
            captured_at: '2026-06-18T11:55:00Z',
          },
        },
      };
      
      const html = renderTimelineRow(event, 0);
      
      // Should render degraded badge styling
      expect(html).toContain('capture-degraded');
      expect(html).toContain('suppressed (degraded)');
      // Should render with degraded row class
      expect(html).toContain('timeline-row-degraded');
    });

    it('renders anchor summary section in expanded panel', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-anchor-summary',
        captureStatus: 'suppressed',
        latencyMs: 800,
        sampleTs: '2026-06-18T12:00:00Z',
        collectedAt: '2026-06-18T12:00:01Z',
        rollingMedianMs: 100,
      });
      
      event.primaryCapture = {
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
        cooldown_info: {
          scope: 'per_diagnostic_peer',
          last_successful_capture_at: '2026-06-18T11:55:00Z',
          next_capture_eligible_at: '2026-06-18T12:05:00Z',
          remaining_cooldown_ms: 300000,
          cooldown_key: 'peer-1',
          anchor_visible: true,
          anchor_artifact_visible: true,
          anchor_timeline_visible: true,
          anchor_visibility_reason: 'retained_visible',
          skipped_attempt_updates_cooldown: false,
          cooldown_seconds: 300,
          anchor_capture_id: 'icmp-anchor-001',
          anchor_target_id: 'test-target',
          anchor_probe_kind: 'icmp',
          anchor_source: 'peer-1',
          suppressed_probe_kind: 'http',
          is_cross_probe_suppression: false,
          anchor_event_summary: {
            event_id: 'anchor-evt-001',
            capture_id: 'anchor-cap-001',
            probe_kind: 'icmp',
            severity: 'ok',
            latency_ms: 45,
            sample_ts: '2026-06-18T11:55:00Z',
            capture_status: 'ok',
            source: 'peer-1',
            captured_at: '2026-06-18T11:55:00Z',
          },
        },
      };
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render anchor summary section
      expect(html).toContain('anchor-summary-section');
      expect(html).toContain('Anchor Provenance (Embedded Summary)');
      // Should render anchor event fields
      expect(html).toContain('anchor-evt-001');
      expect(html).toContain('icmp');
      expect(html).toContain('45');
    });

    it('renders degraded warning in expanded panel', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-degraded-warning',
        captureStatus: 'suppressed',
        latencyMs: 800,
        sampleTs: '2026-06-18T12:00:00Z',
        collectedAt: '2026-06-18T12:00:01Z',
        rollingMedianMs: 100,
      });
      
      event.primaryCapture = {
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
        cooldown_info: {
          scope: 'per_diagnostic_peer',
          last_successful_capture_at: '2026-06-18T11:55:00Z',
          next_capture_eligible_at: '2026-06-18T12:05:00Z',
          remaining_cooldown_ms: 300000,
          cooldown_key: 'peer-1',
          anchor_visible: true,
          anchor_artifact_visible: true,
          anchor_timeline_visible: true,
          anchor_visibility_reason: 'retained_visible',
          skipped_attempt_updates_cooldown: false,
          cooldown_seconds: 300,
          anchor_capture_id: 'icmp-anchor-001',
          anchor_target_id: 'test-target',
          anchor_probe_kind: 'icmp',
          anchor_source: 'peer-1',
          suppressed_probe_kind: 'http',
          is_cross_probe_suppression: false,
          suppression_degraded: true,
          suppression_degraded_reason: 'Anchor capture data incomplete',
          anchor_event_summary: {
            event_id: 'anchor-evt-001',
            capture_id: 'anchor-cap-001',
            probe_kind: 'icmp',
            severity: 'ok',
            latency_ms: 45,
            sample_ts: '2026-06-18T11:55:00Z',
            capture_status: 'ok',
            source: 'peer-1',
            captured_at: '2026-06-18T11:55:00Z',
          },
        },
      };
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render degraded warning with correct CSS class names
      expect(html).toContain('suppression-degraded-warning');
      expect(html).toContain('warning-icon');  // CSS class name, not degraded-warning-icon
      expect(html).toContain('warning-title'); // CSS class name, not degraded-warning-title
      expect(html).toContain('Suppression provenance degraded');
      expect(html).toContain('Anchor capture data incomplete');
      // Should render panel-degraded class
      expect(html).toContain('panel-degraded');
    });

    it('pinned anchors from API response are rendered with anchor badge through production normalization', () => {
      // Regression test: verifies that pinned_anchors from the API response are:
      // 1. Normalized through the production normalizeHttpResponse() path
      // 2. Rendered with anchor badges in the timeline table
      //
      // This test catches the production wiring bug where pinned_anchors were
      // typed but not actually merged into the timeline events.

      // Create a production-shaped API response with pinned_anchors
      const apiResponse = createPinnedAnchorsSpikeResponse();
      
      // Verify the fixture has the correct shape
      expect(apiResponse.spikes).toHaveLength(1);
      expect(apiResponse.pinned_anchors).toHaveLength(1);
      expect(apiResponse.pinned_anchors![0].event_id).toBe('anchor-evt-pinned-001');
      
      // Normalize through the production path (same path effects.ts uses)
      const events = normalizeHttpResponse(apiResponse);
      
      // Should have BOTH events: suppressed spike + pinned anchor
      expect(events).toHaveLength(2);
      
      // Find the pinned anchor event (should have isPinnedAnchor: true)
      const pinnedAnchorEvent = events.find(e => e.eventId === 'anchor-evt-pinned-001');
      expect(pinnedAnchorEvent).toBeDefined();
      expect(pinnedAnchorEvent!.isPinnedAnchor).toBe(true);
      
      // Find the suppressed event (should NOT have isPinnedAnchor)
      const suppressedEvent = events.find(e => e.eventId === 'evt-suppressed-001');
      expect(suppressedEvent).toBeDefined();
      expect(suppressedEvent!.isPinnedAnchor).toBe(false);
      expect(suppressedEvent!.captureStatus).toBe('suppressed');
      
      // Render the table with all events
      const html = renderTimelineTable(events);
      
      // Verify pinned anchor is rendered with anchor badge
      expect(html).toContain('anchor-evt-pinned-001');
      expect(html).toContain('anchor-badge');
      expect(html).toContain('timeline-row-pinned');
      
      // Verify suppressed event is also rendered
      expect(html).toContain('evt-suppressed-001');
    });
  });

  // -------------------------------------------------------------------------
  // TCP Quality Section Render Tests
  // -------------------------------------------------------------------------

  describe('TCP Quality Section Rendering', () => {
    it('renders "Actual HTTP probe socket TCP_INFO" label for native_tcp_info', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-native-tcp',
        latencyMs: 2500,
        sampleTs: '2026-06-18T12:00:00Z',
        collectedAt: '2026-06-18T12:00:01Z',
        rollingMedianMs: 100,
        nativeTcpQuality: createNativeTcpQuality({
          rtt_us: 50000,
          rttvar_us: 5000,
          retransmits_current: 0,
          retransmits_total: 0,
          snd_cwnd: 10,
          congestion_algorithm: 'cubic',
        }),
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render TCP Path Quality section
      expect(html).toContain('tcp-quality-section');
      expect(html).toContain('TCP Path Quality');
      // Should render native badge label
      expect(html).toContain('Actual HTTP probe socket TCP_INFO');
      expect(html).toContain('tcp-quality-native');
      // Should render RTT
      expect(html).toContain('RTT:');
      expect(html).toContain('50.0 ms');
    });

    it('renders RTT and other metrics for native TCP quality', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-native-metrics',
        latencyMs: 3000,
        nativeTcpQuality: createNativeTcpQuality({
          rtt_us: 100000,
          rttvar_us: 10000,
          retransmits_current: 5,
          retransmits_total: 20,
          snd_cwnd: 20,
          congestion_algorithm: 'bbr',
          delivery_rate_bps: 2000000,
          send_queue_bytes: 1000,
          recv_queue_bytes: 2000,
          lost: 3,
          unacked: 5,
        }),
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render all metrics
      expect(html).toContain('100.0 ms'); // RTT
      expect(html).toContain('10.0 ms'); // RTT variance
      expect(html).toContain('Retransmits (current):');
      expect(html).toContain('5');
      expect(html).toContain('Retransmits (total):');
      expect(html).toContain('20');
      expect(html).toContain('Congestion window:');
      expect(html).toContain('20');
      expect(html).toContain('Congestion algorithm:');
      expect(html).toContain('bbr');
      expect(html).toContain('Delivery rate:');
      expect(html).toContain('2.00 Mbps');
      expect(html).toContain('Send queue:');
      expect(html).toContain('1000 bytes');
      expect(html).toContain('Receive queue:');
      expect(html).toContain('2000 bytes');
      expect(html).toContain('Lost packets:');
      expect(html).toContain('3');
      expect(html).toContain('Unacked:');
      expect(html).toContain('5');
    });

    it('renders "Synthetic diagnostic TCP_INFO" for synthetic_tcp_info', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-synthetic-tcp',
        latencyMs: 2000,
        nativeTcpQuality: createSyntheticTcpQuality({
          rtt_us: 60000,
          rttvar_us: 6000,
          retransmits_current: 1,
        }),
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render synthetic badge label
      expect(html).toContain('TCP Path Quality');
      expect(html).toContain('Synthetic diagnostic TCP_INFO');
      expect(html).toContain('tcp-quality-synthetic');
      // Should render metrics
      expect(html).toContain('RTT:');
      expect(html).toContain('60.0 ms');
    });

    it('renders "ss fallback TCP info" for ss-tcp-info', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-ss-tcp',
        latencyMs: 1500,
        nativeTcpQuality: createSsTcpQuality({
          rtt_us: 75000,
          retransmits_current: 2,
        }),
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render ss badge label
      expect(html).toContain('ss fallback TCP info');
      expect(html).toContain('tcp-quality-synthetic');
      // Should render metrics
      expect(html).toContain('RTT:');
      expect(html).toContain('75.0 ms');
    });

    it('renders "TCP quality unavailable" for unavailable source with error', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-unavailable-tcp',
        latencyMs: 3000,
        nativeTcpQuality: createUnavailableTcpQuality({
          error_kind: 'no_matching_socket',
          error: 'No matching TCP socket found for probe connection',
        }),
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render unavailable badge
      expect(html).toContain('TCP Path Quality');
      expect(html).toContain('TCP quality unavailable');
      expect(html).toContain('tcp-quality-unavailable');
      // Should render error message
      expect(html).toContain('tcp-quality-error');
      expect(html).toContain('No matching TCP socket found for probe connection');
    });

    it('renders unavailable with error_kind when no error message', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-unavailable-kind',
        latencyMs: 2500,
        nativeTcpQuality: {
          kind: 'http',
          lookup_target: 'example.com',
          matched_socket: false,
          source: 'unavailable',
          error_kind: 'permission_denied',
          collected_at: '2026-06-18T12:00:00Z',
        },
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render error_kind when error is not present
      expect(html).toContain('permission_denied');
    });

    it('does not render TCP Path Quality section when nativeTcpQuality is null', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-no-tcp',
        latencyMs: 1234,
        nativeTcpQuality: null,
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should NOT render TCP quality section
      expect(html).not.toContain('tcp-quality-section');
      expect(html).not.toContain('TCP Path Quality');
    });

    it('renders socket state when available', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-state-tcp',
        latencyMs: 2000,
        nativeTcpQuality: createNativeTcpQuality({
          state: 'TIME_WAIT',
        }),
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render state
      expect(html).toContain('State:');
      expect(html).toContain('TIME_WAIT');
    });

    it('renders collected timestamp', () => {
      const event = createHttpTimelineEvent({
        eventId: 'evt-collected-tcp',
        latencyMs: 2000,
        nativeTcpQuality: createNativeTcpQuality({
          collected_at: '2026-06-18T12:00:00Z',
        }),
      });
      
      const html = renderExpandedPanel(event, 0);
      
      // Should render collected timestamp
      expect(html).toContain('Collected:');
    });
  });
});
