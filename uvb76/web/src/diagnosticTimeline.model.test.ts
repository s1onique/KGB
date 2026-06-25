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
  getNativeTcpQuality,
  hasNativeTcpQuality,
  getTcpQualitySourceLabel,
  isNativeTcpQuality,
  isSyntheticTcpQuality,
  isSsTcpQuality,
  isTcpQualityUnavailable,
} from './diagnosticTimeline.model';
import {
  createSpikeEvent,
  createSpikeResponse,
  createOkCapture,
  createErrorCapture,
  createSkippedCooldownCaptureWithHiddenAnchor,
  createIcmpToHttpCrossProbeCooldownInfo,
  createNativeTcpQuality,
  createSsTcpQuality,
  createSyntheticTcpQuality,
  createUnavailableTcpQuality,
  createNativeTcpQualitySpikeResponse,
  createUnavailableTcpQualitySpikeResponse,
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

  describe('native TCP quality helpers', () => {
    describe('getNativeTcpQuality', () => {
      it('returns null when no TCP quality data', () => {
        const spike = createSpikeEvent({});
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(getNativeTcpQuality(state.mergedEvents[0])).toBeNull();
      });

      it('returns TCP quality when present', () => {
        const response = createNativeTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        const tcpQuality = getNativeTcpQuality(state.mergedEvents[0]);
        expect(tcpQuality).not.toBeNull();
        expect(tcpQuality?.source).toBe('native_tcp_info');
      });
    });

    describe('hasNativeTcpQuality', () => {
      it('returns false when no TCP quality data', () => {
        const spike = createSpikeEvent({});
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(hasNativeTcpQuality(state.mergedEvents[0])).toBe(false);
      });

      it('returns true when TCP quality is present', () => {
        const response = createNativeTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(hasNativeTcpQuality(state.mergedEvents[0])).toBe(true);
      });
    });

    describe('isNativeTcpQuality', () => {
      it('returns true for native_tcp_info with matched_socket=true', () => {
        const response = createNativeTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(isNativeTcpQuality(state.mergedEvents[0])).toBe(true);
      });

      it('returns false for ss-tcp-info (synthetic)', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSsTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isNativeTcpQuality(state.mergedEvents[0])).toBe(false);
      });

      it('returns false for unavailable (no socket)', () => {
        const response = createUnavailableTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(isNativeTcpQuality(state.mergedEvents[0])).toBe(false);
      });

      it('returns false for native_tcp_info with matched_socket=false', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createNativeTcpQuality({ matched_socket: false }),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        // Even with source=native_tcp_info, if matched_socket=false, it's not "native"
        expect(isNativeTcpQuality(state.mergedEvents[0])).toBe(false);
      });
    });

    describe('isSyntheticTcpQuality', () => {
      it('returns true for ss-tcp-info', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSsTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isSyntheticTcpQuality(state.mergedEvents[0])).toBe(true);
      });

      it('returns true for synthetic_tcp_info', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSyntheticTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isSyntheticTcpQuality(state.mergedEvents[0])).toBe(true);
      });

      it('returns false for native_tcp_info', () => {
        const response = createNativeTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(isSyntheticTcpQuality(state.mergedEvents[0])).toBe(false);
      });
    });

    describe('isSsTcpQuality', () => {
      it('returns true for ss-tcp-info source', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSsTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isSsTcpQuality(state.mergedEvents[0])).toBe(true);
      });

      it('returns false for native_tcp_info', () => {
        const response = createNativeTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(isSsTcpQuality(state.mergedEvents[0])).toBe(false);
      });

      it('returns false for synthetic_tcp_info', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSyntheticTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isSsTcpQuality(state.mergedEvents[0])).toBe(false);
      });
    });

    describe('getTcpQualitySourceLabel', () => {
      it('returns "Actual HTTP probe socket TCP_INFO" for native_tcp_info', () => {
        const response = createNativeTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(getTcpQualitySourceLabel(state.mergedEvents[0])).toBe('Actual HTTP probe socket TCP_INFO');
      });

      it('returns "ss fallback TCP info" for ss-tcp-info', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSsTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(getTcpQualitySourceLabel(state.mergedEvents[0])).toBe('ss fallback TCP info');
      });

      it('returns "Synthetic diagnostic TCP_INFO" for synthetic_tcp_info', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSyntheticTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(getTcpQualitySourceLabel(state.mergedEvents[0])).toBe('Synthetic diagnostic TCP_INFO');
      });

      it('returns "TCP quality unavailable" for unavailable', () => {
        const response = createUnavailableTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(getTcpQualitySourceLabel(state.mergedEvents[0])).toBe('TCP quality unavailable');
      });

      it('returns "TCP quality unavailable" when no TCP quality data', () => {
        const spike = createSpikeEvent({});
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(getTcpQualitySourceLabel(state.mergedEvents[0])).toBe('TCP quality unavailable');
      });
    });

    describe('isTcpQualityUnavailable', () => {
      it('returns true when no TCP quality data', () => {
        const spike = createSpikeEvent({});
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isTcpQualityUnavailable(state.mergedEvents[0])).toBe(true);
      });

      it('returns true when source=unavailable', () => {
        const response = createUnavailableTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(isTcpQualityUnavailable(state.mergedEvents[0])).toBe(true);
      });

      it('returns true when has error_kind (socket not found)', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createUnavailableTcpQuality({ error_kind: 'no_matching_socket' }),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isTcpQualityUnavailable(state.mergedEvents[0])).toBe(true);
      });

      it('returns false for native_tcp_info with matched_socket=true', () => {
        const response = createNativeTcpQualitySpikeResponse();
        const state = buildTimelineState(response, null);
        
        expect(isTcpQualityUnavailable(state.mergedEvents[0])).toBe(false);
      });

      it('returns false for ss-tcp-info', () => {
        const spike = createSpikeEvent({
          native_tcp_quality: createSsTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isTcpQualityUnavailable(state.mergedEvents[0])).toBe(false);
      });

      it('returns false for synthetic_tcp_info (even with matched_socket=false)', () => {
        // Key semantic: synthetic_tcp_info is NOT unavailable - it's valid evidence
        // just from a different source than the native probe socket
        const spike = createSpikeEvent({
          native_tcp_quality: createSyntheticTcpQuality(),
        });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        expect(isTcpQualityUnavailable(state.mergedEvents[0])).toBe(false);
      });
    });

    describe('TCP quality data propagation', () => {
      it('propagates RTT from native TCP quality', () => {
        const response = createNativeTcpQualitySpikeResponse({ rtt_us: 100000 });
        const state = buildTimelineState(response, null);
        
        const tcpQuality = getNativeTcpQuality(state.mergedEvents[0]);
        expect(tcpQuality?.rtt_us).toBe(100000);
      });

      it('propagates retransmits from native TCP quality', () => {
        const tcpQuality = createNativeTcpQuality({
          retransmits_current: 5,
          retransmits_total: 20,
        });
        const spike = createSpikeEvent({ native_tcp_quality: tcpQuality });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        const result = getNativeTcpQuality(state.mergedEvents[0]);
        expect(result?.retransmits_current).toBe(5);
        expect(result?.retransmits_total).toBe(20);
      });

      it('propagates congestion window from native TCP quality', () => {
        const tcpQuality = createNativeTcpQuality({
          snd_cwnd: 20,
          congestion_algorithm: 'bbr',
        });
        const spike = createSpikeEvent({ native_tcp_quality: tcpQuality });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        const result = getNativeTcpQuality(state.mergedEvents[0]);
        expect(result?.snd_cwnd).toBe(20);
        expect(result?.congestion_algorithm).toBe('bbr');
      });

      it('propagates socket state from native TCP quality', () => {
        const tcpQuality = createNativeTcpQuality({
          state: 'TIME_WAIT',
        });
        const spike = createSpikeEvent({ native_tcp_quality: tcpQuality });
        const response = createSpikeResponse(spike);
        const state = buildTimelineState(response, null);
        
        const result = getNativeTcpQuality(state.mergedEvents[0]);
        expect(result?.state).toBe('TIME_WAIT');
      });
    });
  });
});
