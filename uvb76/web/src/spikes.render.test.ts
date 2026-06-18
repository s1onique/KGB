// DOM renderer tests for spike diagnostics module

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { SpikeResponseWithCaptures } from './api';

// Mock the api module
const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) => mockGetLatencySpikesWithCaptures(...args),
  },
}));

// Import after mock setup
import { loadSpikeDiagnostics } from './spikes';

describe('spikes DOM renderer', () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    vi.clearAllMocks();
    // Create a real DOM container for each test
    container = document.createElement('div');
    container.id = 'spike-diag-test-target';
    document.body.appendChild(container);
  });

  afterEach(() => {
    // Clean up DOM after each test
    container.remove();
  });

  describe('empty response', () => {
    it('renders "No recent spikes" when response has no spikes', async () => {
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [],
        count: 0,
      });

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('No recent spikes');
    });

    it('renders "No recent spikes" when response is undefined', async () => {
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [],
        count: 0,
      });

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('No recent spikes');
    });
  });

  describe('successful capture rendering', () => {
    it('renders spike diagnostics header', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: ['high_latency'],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'tovarisch-peer',
            base_url: 'http://10.77.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            duration_ms: 123,
            status: 'ok',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('Spike diagnostics');
    });

    it('renders probe kind', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('http');
    });

    it('renders severity', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('warning');
    });

    it('renders formatted latency', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      // 1234ms should render as "1.23 s" (adaptive formatting)
      expect(container.textContent).toContain('1.23 s');
    });

    it('renders "Capture: ok" for successful capture', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'tovarisch-peer',
            base_url: 'http://10.77.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            duration_ms: 123,
            status: 'ok',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('Capture: ok');
    });

    it('renders source peer name', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'tovarisch-peer',
            base_url: 'http://10.77.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('tovarisch-peer');
    });

    it('renders "Network diag:" label', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('Network diag:');
    });

    it('renders network_diag status "ok"', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('ok');
    });
  });

  describe('underlay TCP summary rendering', () => {
    it('renders xray socket name', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 123.4,
                rto_ms: 456,
                retransmits: 7,
                unacked: 3,
                cwnd: 10,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('xray');
    });

    it('renders ESTAB state', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 123.4,
                rto_ms: 456,
                retransmits: 7,
                unacked: 3,
                cwnd: 10,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('ESTAB');
    });

    it('renders "xray ESTAB" combined', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 123.4,
                rto_ms: 456,
                retransmits: 7,
                unacked: 3,
                cwnd: 10,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('xray ESTAB');
    });

    it('renders RTT with one decimal place', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 123.4,
                rto_ms: 456,
                retransmits: 7,
                unacked: 3,
                cwnd: 10,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('RTT 123.4 ms');
    });

    it('renders RTO value', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 123.4,
                rto_ms: 456,
                retransmits: 7,
                unacked: 3,
                cwnd: 10,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('RTO 456 ms');
    });

    it('renders retransmits count', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 123.4,
                rto_ms: 456,
                retransmits: 7,
                unacked: 3,
                cwnd: 10,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('retransmits 7');
    });
  });

  describe('error capture rendering', () => {
    it('renders "Capture: error" for error status', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'error',
            error: 'connection refused',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('Capture: error');
    });

    it('renders safe/truncated error text', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'error',
            error: 'connection refused',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      // Error text should be visible in textContent (escaped)
      expect(container.textContent).toContain('connection refused');
    });

    it('truncates long error messages', async () => {
      const longError = 'A'.repeat(100);
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'error',
            error: longError,
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      // The ellipsis character should appear for truncated errors
      expect(container.textContent).toContain('…');
    });
  });

  describe('suppressed capture rendering', () => {
    it('renders "suppressed by cooldown"', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 800,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            suppressed_by_cooldown: true,
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 50.0,
                rto_ms: 200,
                retransmits: 0,
                unacked: 0,
                cwnd: 100,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('suppressed by cooldown');
    });

    it('does not render network_diag content for suppressed capture', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 800,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            suppressed_by_cooldown: true,
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: 'xray',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 50.0,
                rto_ms: 200,
                retransmits: 0,
                unacked: 0,
                cwnd: 100,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      // Network diag should NOT appear for suppressed captures
      expect(container.textContent).not.toContain('Network diag:');
      expect(container.textContent).not.toContain('xray');
      expect(container.textContent).not.toContain('RTT');
    });
  });

  describe('missing network_diag handling', () => {
    it('does not crash when network_diag is missing', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: undefined,
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      // Should not throw
      await expect(loadSpikeDiagnostics('test-target')).resolves.not.toThrow();

      // Should still render the capture status
      expect(container.textContent).toContain('Capture: ok');
    });
  });

  describe('API error handling', () => {
    it('renders "Spike diagnostics unavailable" on API rejection', async () => {
      mockGetLatencySpikesWithCaptures.mockRejectedValue(new Error('Network error'));

      await loadSpikeDiagnostics('test-target');

      expect(container.textContent).toContain('Spike diagnostics unavailable');
    });

    it('does not crash on API error', async () => {
      mockGetLatencySpikesWithCaptures.mockRejectedValue(new Error('Network error'));

      await expect(loadSpikeDiagnostics('test-target')).resolves.not.toThrow();
    });
  });

  describe('missing target container', () => {
    it('is a no-op and does not throw when container is missing', async () => {
      // Remove the container that was created in beforeEach
      container.remove();

      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [],
        }],
        count: 1,
      });

      // Should not throw - function returns early when container is missing
      await expect(loadSpikeDiagnostics('nonexistent-target')).resolves.not.toThrow();

      // API should NOT be called when container is missing (returns early)
      expect(mockGetLatencySpikesWithCaptures).not.toHaveBeenCalled();
    });
  });

  describe('XSS safety', () => {
    it('escapes HTML-like text in capture source', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: '<script>alert(1)</script>',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      // The literal text should be visible in textContent
      expect(container.textContent).toContain('<script>alert(1)</script>');
      // But no actual script element should exist
      expect(container.querySelector('script')).toBeNull();
    });

    it('escapes HTML-like text in error message', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'error',
            error: '<img src=x onerror=alert(1)>',
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      // The literal text should be visible in textContent
      expect(container.textContent).toContain('<img src=x onerror=alert(1)>');
      // But no actual img element should exist
      expect(container.querySelector('img')).toBeNull();
    });

    it('escapes HTML-like text in TCP socket name', async () => {
      const mockResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'evt-1',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1234,
          rolling_median_ms: 100,
          reasons: [],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            network_diag: {
              started_at: '2026-06-18T12:00:00Z',
              status: 'ok',
              underlay_tcp: [{
                name: '<b>bold</b>',
                state: 'ESTAB',
                local: 'redacted:443',
                remote: 'redacted:443',
                rtt_ms: 50.0,
                rto_ms: 200,
                retransmits: 0,
                unacked: 0,
                cwnd: 100,
                status: 'ok',
              }],
            },
          }],
        }],
        count: 1,
      };

      mockGetLatencySpikesWithCaptures.mockResolvedValue(mockResponse);

      await loadSpikeDiagnostics('test-target');

      // The literal text should be visible in textContent
      expect(container.textContent).toContain('<b>bold</b>');
      // But no actual b element should exist
      expect(container.querySelector('b')).toBeNull();
    });
  });
});
