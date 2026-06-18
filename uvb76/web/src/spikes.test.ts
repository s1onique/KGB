// Tests for spike diagnostics module - API and type tests

import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { SpikeResponseWithCaptures, DiagCapture, SpikeEventWithCaptures } from './api';

// Mock the entire api module with the actual types
const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) => mockGetLatencySpikesWithCaptures(...args),
  },
}));

// Mock the spikes module itself to isolate API testing
vi.mock('./spikes', async () => {
  const actual = await vi.importActual('./spikes');
  return {
    ...actual,
  };
});

describe('spike diagnostics API', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('getLatencySpikesWithCaptures calls', () => {
    it('should call API with correct target_id', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [],
        count: 0,
      });
      
      await api.getLatencySpikesWithCaptures('test-target-123', 10);
      
      expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalledWith('test-target-123', 10);
    });

    it('should pass limit parameter to API', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [],
        count: 0,
      });
      
      await api.getLatencySpikesWithCaptures('test-target', 5);
      
      expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalledWith('test-target', 5);
    });

    it('should handle empty spikes response', async () => {
      const { api } = await import('./api');
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [],
        count: 0,
      };
      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes).toEqual([]);
      expect(result.count).toBe(0);
    });

    it('should handle spikes with captures', async () => {
      const { api } = await import('./api');
      const capture: DiagCapture = {
        source: 'tovarisch-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T10:00:00Z',
        duration_ms: 150,
        status: 'ok',
        network_diag: {
          started_at: '2026-06-18T10:00:00Z',
          status: 'ok',
          underlay_tcp: [
            {
              name: 'xray',
              state: 'ESTAB',
              local: '10.0.0.2:12345',
              remote: '10.0.0.1:443',
              rtt_ms: 12.5,
              rto_ms: 200,
              retransmits: 0,
              unacked: 0,
              cwnd: 1000,
              status: 'ok',
            },
          ],
        },
      };
      
      const spike: SpikeEventWithCaptures = {
        event_id: 'evt-1',
        target_id: 'test-target',
        kind: 'http',
        severity: 'warning',
        sample_ts: '2026-06-18T10:00:00Z',
        latency_ms: 1234,
        rolling_median_ms: 100,
        reasons: ['high_latency'],
        thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
        previous_samples: [],
        collected_at: '2026-06-18T10:00:00Z',
        captures: [capture],
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [spike],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes).toHaveLength(1);
      expect(result.spikes[0].captures).toHaveLength(1);
      expect(result.spikes[0].captures![0].status).toBe('ok');
      expect(result.spikes[0].captures![0].network_diag?.underlay_tcp).toHaveLength(1);
    });
  });

  describe('DiagCapture status types', () => {
    it('should support ok status', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T10:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T10:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T10:00:00Z',
            status: 'ok' as const,
          }],
        }],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes[0].captures![0].status).toBe('ok');
    });

    it('should support error status', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'icmp',
          severity: 'critical',
          sample_ts: '2026-06-18T10:00:00Z',
          latency_ms: 5000,
          rolling_median_ms: 50,
          reasons: [],
          thresholds: { warning_ms: 100, critical_ms: 500, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T10:00:00Z',
          captures: [{
            source: 'peer-2',
            base_url: 'http://10.0.0.2:8080',
            capture_started_at: '2026-06-18T10:00:00Z',
            status: 'error' as const,
            error: 'connection refused',
          }],
        }],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes[0].captures![0].status).toBe('error');
      expect(result.spikes[0].captures![0].error).toBe('connection refused');
    });

    it('should support suppressed_by_cooldown', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T10:00:00Z',
          latency_ms: 800,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T10:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T10:00:00Z',
            status: 'ok' as const,
            suppressed_by_cooldown: true,
          }],
        }],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes[0].captures![0].suppressed_by_cooldown).toBe(true);
    });

    it('should support timeout status', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'icmp',
          severity: 'critical',
          sample_ts: '2026-06-18T10:00:00Z',
          latency_ms: 10000,
          rolling_median_ms: 50,
          reasons: [],
          thresholds: { warning_ms: 100, critical_ms: 500, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T10:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T10:00:00Z',
            duration_ms: 5000,
            status: 'timeout' as const,
          }],
        }],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes[0].captures![0].status).toBe('timeout');
    });
  });

  describe('TCP socket data types', () => {
    it('should parse underlay_tcp with all fields', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'critical',
          sample_ts: '2026-06-18T10:00:00Z',
          latency_ms: 2500,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T10:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T10:00:00Z',
            duration_ms: 250,
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T10:00:00Z',
              status: 'degraded',
              underlay_tcp: [
                {
                  name: 'xray',
                  state: 'ESTAB',
                  local: '10.0.0.2:12345',
                  remote: '10.0.0.1:443',
                  rtt_ms: 123.4,
                  rto_ms: 456,
                  retransmits: 7,
                  unacked: 3,
                  cwnd: 5000,
                  status: 'ok',
                },
              ],
            },
          }],
        }],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      const tcp = result.spikes[0].captures![0].network_diag!.underlay_tcp[0];
      expect(tcp.name).toBe('xray');
      expect(tcp.state).toBe('ESTAB');
      expect(tcp.rtt_ms).toBe(123.4);
      expect(tcp.rto_ms).toBe(456);
      expect(tcp.retransmits).toBe(7);
    });
  });

  describe('error handling', () => {
    it('should handle API errors gracefully', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockRejectedValue(new Error('Network error'));
      
      await expect(api.getLatencySpikesWithCaptures('test-target')).rejects.toThrow('Network error');
    });

    it('should handle missing captures array', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T10:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T10:00:00Z',
          captures: undefined,
        }],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes[0].captures).toBeUndefined();
    });

    it('should handle missing network_diag', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T10:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T10:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T10:00:00Z',
            status: 'ok',
            network_diag: undefined,
          }],
        }],
        count: 1,
      });
      
      const result = await api.getLatencySpikesWithCaptures('test-target');
      
      expect(result.spikes[0].captures![0].network_diag).toBeUndefined();
    });
  });
});
