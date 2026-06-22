// Diagnostic Timeline Model Tests - Unit tests for model layer

import { describe, it, expect } from 'vitest';
import {
  buildTimelineState,
  buildEmptyTimelineState,
  buildLoadingTimelineState,
  buildErrorTimelineState,
  hasCrossProbeSuppression,
  getCooldownInfo,
  getAnchorProbeKind,
  getSuppressedProbeKind,
} from './diagnosticTimeline.model';
import {
  createSpikeEvent,
  createSpikeResponse,
  createOkCapture,
  createErrorCapture,
  createSkippedCooldownCaptureWithHiddenAnchor,
  createIcmpToHttpCrossProbeCooldownInfo,
  defaultRetention,
} from './diagnosticTimeline.fixtures';

describe('diagnosticTimeline.model', () => {
  describe('buildEmptyTimelineState', () => {
    it('creates empty state with correct structure', () => {
      const state = buildEmptyTimelineState();
      
      expect(state.httpEvents).toEqual([]);
      expect(state.icmpEvents).toEqual([]);
      expect(state.mergedEvents).toEqual([]);
      expect(state.httpSummary.probeKind).toBe('http');
      expect(state.icmpSummary.probeKind).toBe('icmp');
      expect(state.isLoading).toBe(false);
      expect(state.error).toBeNull();
    });
    
    it('has zero counts in summaries', () => {
      const state = buildEmptyTimelineState();
      
      expect(state.httpSummary.totalEvents).toBe(0);
      expect(state.httpSummary.capturedCount).toBe(0);
      expect(state.httpSummary.suppressedCount).toBe(0);
      expect(state.httpSummary.failedCount).toBe(0);
      expect(state.icmpSummary.totalEvents).toBe(0);
    });
  });

  describe('buildLoadingTimelineState', () => {
    it('sets isLoading to true', () => {
      const state = buildLoadingTimelineState();
      expect(state.isLoading).toBe(true);
    });
    
    it('has empty events', () => {
      const state = buildLoadingTimelineState();
      expect(state.mergedEvents).toEqual([]);
    });
  });

  describe('buildErrorTimelineState', () => {
    it('sets error message', () => {
      const state = buildErrorTimelineState('Network error');
      expect(state.error).toBe('Network error');
      expect(state.isLoading).toBe(false);
    });
  });

  describe('buildTimelineState - HTTP and ICMP merge', () => {
    it('merges HTTP and ICMP events into one chronological list', () => {
      const httpSpike = createSpikeEvent({
        event_id: 'http-evt-1',
        kind: 'http',
        sample_ts: '2026-06-18T12:00:00Z',
      });
      const icmpSpike = createSpikeEvent({
        event_id: 'icmp-evt-1',
        kind: 'icmp',
        sample_ts: '2026-06-18T12:01:00Z', // newer
      });
      
      const httpResponse = createSpikeResponse(httpSpike);
      const icmpResponse = createSpikeResponse(icmpSpike);
      
      const state = buildTimelineState(httpResponse, icmpResponse);
      
      // Should have events from both
      expect(state.httpEvents.length).toBe(1);
      expect(state.icmpEvents.length).toBe(1);
      expect(state.mergedEvents.length).toBe(2);
      
      // Should be sorted newest-first (ICMP first)
      expect(state.mergedEvents[0].probeKind).toBe('icmp');
      expect(state.mergedEvents[1].probeKind).toBe('http');
    });

    it('computes summary counts correctly', () => {
      const httpSpike = createSpikeEvent({
        kind: 'http',
        severity: 'warning',
        captures: [createOkCapture()],
      });
      const icmpSpike = createSpikeEvent({
        kind: 'icmp',
        severity: 'critical',
        captures: [createErrorCapture({ error: 'test' })],
      });
      
      const httpResponse = createSpikeResponse(httpSpike);
      const icmpResponse = createSpikeResponse(icmpSpike);
      
      const state = buildTimelineState(httpResponse, icmpResponse);
      
      expect(state.httpSummary.totalEvents).toBe(1);
      expect(state.httpSummary.capturedCount).toBe(1);
      expect(state.icmpSummary.totalEvents).toBe(1);
      expect(state.icmpSummary.failedCount).toBe(1);
    });

    it('sorts newest-first correctly', () => {
      const olderHttp = createSpikeEvent({
        event_id: 'http-old',
        kind: 'http',
        sample_ts: '2026-06-18T10:00:00Z',
      });
      const newerHttp = createSpikeEvent({
        event_id: 'http-new',
        kind: 'http',
        sample_ts: '2026-06-18T12:00:00Z',
      });
      
      const httpResponse = createSpikeResponse(olderHttp);
      const icmpResponse = createSpikeResponse(newerHttp);
      
      const state = buildTimelineState(httpResponse, icmpResponse);
      
      // ICMP (newer) should be first
      expect(state.mergedEvents[0].eventId).toBe('http-new');
      expect(state.mergedEvents[1].eventId).toBe('http-old');
    });

    it('uses stable tie-breaks for same timestamp', () => {
      const sameTs = '2026-06-18T12:00:00Z';
      const httpEvent = createSpikeEvent({
        event_id: 'http-1',
        kind: 'http',
        severity: 'warning',
        sample_ts: sameTs,
      });
      const icmpEvent = createSpikeEvent({
        event_id: 'icmp-1',
        kind: 'icmp',
        severity: 'warning',
        sample_ts: sameTs,
      });
      
      const httpResponse = createSpikeResponse(httpEvent);
      const icmpResponse = createSpikeResponse(icmpEvent);
      
      const state = buildTimelineState(httpResponse, icmpResponse);
      
      // HTTP should come before ICMP (stable tie-break by probe kind)
      expect(state.mergedEvents[0].probeKind).toBe('http');
      expect(state.mergedEvents[1].probeKind).toBe('icmp');
    });
  });

  describe('capture status mapping', () => {
    it('maps captured status correctly', () => {
      const spike = createSpikeEvent({
        captures: [createOkCapture()],
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].captureStatus).toBe('captured');
      expect(state.httpSummary.capturedCount).toBe(1);
    });

    it('maps failed status correctly', () => {
      const spike = createSpikeEvent({
        captures: [createErrorCapture({ error: 'connection refused' })],
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].captureStatus).toBe('failed');
      expect(state.httpSummary.failedCount).toBe(1);
    });

    it('maps skipped_cooldown to suppressed', () => {
      const spike = createSpikeEvent({
        captures: [createSkippedCooldownCaptureWithHiddenAnchor()],
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].captureStatus).toBe('suppressed');
      expect(state.httpSummary.suppressedCount).toBe(1);
    });
  });

  describe('cross-probe suppression helpers', () => {
    it('detects cross-probe suppression', () => {
      const cooldownInfo = createIcmpToHttpCrossProbeCooldownInfo();
      const capture = createSkippedCooldownCaptureWithHiddenAnchor({ cooldown_info: cooldownInfo });
      const spike = createSpikeEvent({ captures: [capture] });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      const event = state.mergedEvents[0];
      expect(hasCrossProbeSuppression(event)).toBe(true);
    });

    it('returns cooldown info', () => {
      const cooldownInfo = createIcmpToHttpCrossProbeCooldownInfo();
      const capture = createSkippedCooldownCaptureWithHiddenAnchor({ cooldown_info: cooldownInfo });
      const spike = createSpikeEvent({ captures: [capture] });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      const event = state.mergedEvents[0];
      const info = getCooldownInfo(event);
      expect(info).not.toBeNull();
      expect(info?.anchor_probe_kind).toBe('icmp');
    });

    it('returns anchor probe kind', () => {
      const cooldownInfo = createIcmpToHttpCrossProbeCooldownInfo();
      const capture = createSkippedCooldownCaptureWithHiddenAnchor({ cooldown_info: cooldownInfo });
      const spike = createSpikeEvent({ captures: [capture] });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      const event = state.mergedEvents[0];
      expect(getAnchorProbeKind(event)).toBe('icmp');
    });

    it('returns suppressed probe kind', () => {
      const cooldownInfo = createIcmpToHttpCrossProbeCooldownInfo();
      const capture = createSkippedCooldownCaptureWithHiddenAnchor({ cooldown_info: cooldownInfo });
      const spike = createSpikeEvent({ captures: [capture] });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      const event = state.mergedEvents[0];
      expect(getSuppressedProbeKind(event)).toBe('http');
    });

    it('returns null for non-suppressed events', () => {
      const spike = createSpikeEvent({
        captures: [createOkCapture()],
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      const event = state.mergedEvents[0];
      expect(hasCrossProbeSuppression(event)).toBe(false);
      expect(getCooldownInfo(event)).toBeNull();
    });
  });

  describe('timestamp normalization', () => {
    it('marks valid ISO timestamp as ok', () => {
      const spike = createSpikeEvent({
        sample_ts: '2026-06-18T12:00:00Z',
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].canonicalTimeMs).not.toBeNull();
      expect(state.mergedEvents[0].timeStatus).toBe('ok');
    });

    it('marks missing timestamp as missing', () => {
      const spike = createSpikeEvent({
        sample_ts: '',
        collected_at: '',
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].canonicalTimeMs).toBeNull();
      expect(state.mergedEvents[0].timeStatus).toBe('missing');
    });

    it('marks malformed API timestamp as invalid', () => {
      const spike = createSpikeEvent({
        sample_ts: 'not-a-valid-date',
        collected_at: 'also-invalid',
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].canonicalTimeMs).toBeNull();
      expect(state.mergedEvents[0].timeStatus).toBe('invalid');
    });

    it('falls back to collected_at when sample_ts is missing', () => {
      const spike = createSpikeEvent({
        sample_ts: '',
        collected_at: '2026-06-18T14:00:00Z',
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].canonicalTimeMs).not.toBeNull();
      expect(state.mergedEvents[0].timeStatus).toBe('ok');
    });

    it('falls back to capture timestamp when both sample and collected are missing', () => {
      const spike = createSpikeEvent({
        sample_ts: '',
        collected_at: '',
        captures: [{
          source: 'peer-1',
          base_url: 'http://10.0.0.1:8080',
          capture_started_at: '2026-06-18T15:00:00Z',
          status: 'ok',
        }],
      });
      
      const response = createSpikeResponse(spike);
      const state = buildTimelineState(response, null);
      
      expect(state.mergedEvents[0].canonicalTimeMs).not.toBeNull();
      expect(state.mergedEvents[0].timeStatus).toBe('ok');
    });

    it('sorts events with invalid timestamps to the end', () => {
      // Create spikes directly to ensure invalid timestamps aren't overridden by fixtures
      const validSpike = {
        event_id: 'aaa-valid-1',
        target_id: 'test',
        kind: 'http',
        severity: 'warning',
        latency_ms: 100,
        sample_ts: '2026-06-18T12:00:00Z',
        rolling_median_ms: 50,
        reasons: [],
        thresholds: { warning_ms: 100, critical_ms: 500, relative_multiplier: 2 },
        previous_samples: [],
        collected_at: '2026-06-18T12:00:00Z',
      };
      const invalidSpike = {
        event_id: 'zzz-invalid-1',
        target_id: 'test',
        kind: 'http',
        severity: 'warning',
        latency_ms: 100,
        sample_ts: 'not-a-date',
        rolling_median_ms: 50,
        reasons: [],
        thresholds: { warning_ms: 100, critical_ms: 500, relative_multiplier: 2 },
        previous_samples: [],
        collected_at: 'also-invalid',
      };
      
      const state = buildTimelineState({ spikes: [validSpike], count: 1, retention: defaultRetention }, { spikes: [invalidSpike], count: 1, retention: defaultRetention });
      
      // Valid timestamp should come first
      expect(state.mergedEvents[0].eventId).toBe('aaa-valid-1');
      expect(state.mergedEvents[0].timeStatus).toBe('ok');
      expect(state.mergedEvents[1].eventId).toBe('zzz-invalid-1');
      expect(state.mergedEvents[1].timeStatus).toBe('invalid');
    });
  });
});
