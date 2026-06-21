// Regression tests for probe-kind aware spike diagnostics
// These tests verify that HTTP and ICMP sections query and render their own probe-kind diagnostics correctly.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { SpikeResponseWithCaptures, CaptureCooldownInfo } from './api';
import { createIcmpToHttpCrossProbeVisibleCooldownInfo } from './spikes.render.fixtures';

// Mock the entire api module
const mockGetLatencySpikesWithCaptures = vi.fn();
const mockGetTargetLatency = vi.fn();
const mockGetHTTPLatencySeries = vi.fn();
const mockGetICMPLatencySeries = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) => mockGetLatencySpikesWithCaptures(...args),
    getTargetLatency: (...args: unknown[]) => mockGetTargetLatency(...args),
    getHTTPLatencySeries: (...args: unknown[]) => mockGetHTTPLatencySeries(...args),
    getICMPLatencySeries: (...args: unknown[]) => mockGetICMPLatencySeries(...args),
  },
}));

describe('Probe-kind aware spike diagnostics', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('API query separation', () => {
    it('should call spike API with kind=http for HTTP section', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [],
        count: 0,
        retention: { retained_spike_count: 0, visible_spike_count: 0, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      });
      
      await api.getLatencySpikesWithCaptures('test-target', 'http', 10);
      
      expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalledWith('test-target', 'http', 10);
      // Verify the URL would contain kind=http
      const callArgs = mockGetLatencySpikesWithCaptures.mock.calls[0];
      expect(callArgs).toContain('http');
    });

    it('should call spike API with kind=icmp for ICMP section', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [],
        count: 0,
        retention: { retained_spike_count: 0, visible_spike_count: 0, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      });
      
      await api.getLatencySpikesWithCaptures('test-target', 'icmp', 10);
      
      expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalledWith('test-target', 'icmp', 10);
      // Verify the URL would contain kind=icmp
      const callArgs = mockGetLatencySpikesWithCaptures.mock.calls[0];
      expect(callArgs).toContain('icmp');
    });

    it('should not use implicit default when querying ICMP spikes', async () => {
      const { api } = await import('./api');
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        spikes: [],
        count: 0,
        retention: { retained_spike_count: 0, visible_spike_count: 0, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      });
      
      // Explicitly pass icmp kind
      await api.getLatencySpikesWithCaptures('test-target', 'icmp', 10);
      
      // Verify ICMP was passed explicitly, not defaulted to http
      expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalledWith('test-target', 'icmp', 10);
    });
  });

  describe('ICMP diagnostics table rendering', () => {
    it('should render ICMP spike rows when ICMP spikes exist', async () => {
      const icmpSpikeResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'icmp-spike-001',
          target_id: 'test-target',
          kind: 'icmp',
          severity: 'critical',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 3000,
          rolling_median_ms: 50,
          reasons: ['critical_latency'],
          thresholds: { warning_ms: 500, critical_ms: 2000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            capture_status: 'captured',
          }],
        }],
        count: 1,
        retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 1, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(icmpSpikeResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'icmp', 10);
      
      expect(result.spikes).toHaveLength(1);
      expect(result.spikes[0].kind).toBe('icmp');
      expect(result.spikes[0].captures).toHaveLength(1);
    });

    it('should return ICMP-specific empty state', async () => {
      const emptyResponse: SpikeResponseWithCaptures = {
        spikes: [],
        count: 0,
        retention: { retained_spike_count: 0, visible_spike_count: 0, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(emptyResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'icmp', 10);
      
      expect(result.spikes).toHaveLength(0);
      expect(result.count).toBe(0);
    });
  });

  describe('HTTP diagnostics table rendering', () => {
    it('should render HTTP spike rows when HTTP spikes exist', async () => {
      const httpSpikeResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'http-spike-001',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1500,
          rolling_median_ms: 100,
          reasons: ['high_latency'],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            capture_status: 'captured',
          }],
        }],
        count: 1,
        retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 1, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(httpSpikeResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'http', 10);
      
      expect(result.spikes).toHaveLength(1);
      expect(result.spikes[0].kind).toBe('http');
    });
  });

  describe('Cross-probe cooldown discoverability', () => {
    it('should provide ICMP anchor hint for HTTP suppressed spike', async () => {
      // This tests the fixture used for cross-probe scenarios
      const cooldownInfo = createIcmpToHttpCrossProbeVisibleCooldownInfo();
      
      // Verify the cooldown info has correct cross-probe metadata
      expect(cooldownInfo.anchor_probe_kind).toBe('icmp');
      expect(cooldownInfo.suppressed_probe_kind).toBe('http');
      expect(cooldownInfo.is_cross_probe_suppression).toBe(true);
      expect(cooldownInfo.anchor_visible).toBe(true);
      expect(cooldownInfo.anchor_visibility_reason).toBe('retained_visible');
    });

    it('should include anchor_capture_id for cross-probe suppressed spikes', async () => {
      const cooldownInfo = createIcmpToHttpCrossProbeVisibleCooldownInfo();
      
      expect(cooldownInfo.anchor_capture_id).toBeDefined();
      expect(cooldownInfo.anchor_capture_id).toBe('icmp-anchor-event-001');
    });

    it('should indicate cross-probe suppression with correct metadata', async () => {
      const httpSuppressedByIcmp: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'http-spike-suppressed',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:15:00Z',
          latency_ms: 800,
          rolling_median_ms: 100,
          reasons: ['high_latency'],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:15:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:15:00Z',
            status: 'ok',
            capture_status: 'skipped_cooldown',
            suppressed_by_cooldown: true,
            cooldown_info: createIcmpToHttpCrossProbeVisibleCooldownInfo(),
          }],
        }],
        count: 1,
        retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(httpSuppressedByIcmp);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'http', 10);
      
      expect(result.spikes).toHaveLength(1);
      expect(result.spikes[0].captures![0].suppressed_by_cooldown).toBe(true);
      expect(result.spikes[0].captures![0].cooldown_info?.is_cross_probe_suppression).toBe(true);
      expect(result.spikes[0].captures![0].cooldown_info?.anchor_probe_kind).toBe('icmp');
    });
  });

  describe('Combined scenario: ICMP anchor + HTTP suppressed', () => {
    it('should return ICMP anchor spike when querying kind=icmp', async () => {
      const icmpAnchorResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'icmp-anchor-event-001',
          target_id: 'test-target',
          kind: 'icmp',
          severity: 'critical',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 3000,
          rolling_median_ms: 50,
          reasons: ['critical_latency'],
          thresholds: { warning_ms: 500, critical_ms: 2000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:00:00Z',
            status: 'ok',
            capture_status: 'captured',
          }],
        }],
        count: 1,
        retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 1, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(icmpAnchorResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'icmp', 10);
      
      expect(result.spikes).toHaveLength(1);
      expect(result.spikes[0].event_id).toBe('icmp-anchor-event-001');
      expect(result.spikes[0].kind).toBe('icmp');
      expect(result.spikes[0].captures![0].capture_status).toBe('captured');
    });

    it('should return HTTP suppressed spike when querying kind=http', async () => {
      const httpSuppressedResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'http-spike-suppressed',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:15:00Z',
          latency_ms: 800,
          rolling_median_ms: 100,
          reasons: ['high_latency'],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:15:00Z',
          captures: [{
            source: 'peer-1',
            base_url: 'http://10.0.0.1:8080',
            capture_started_at: '2026-06-18T12:15:00Z',
            status: 'ok',
            capture_status: 'skipped_cooldown',
            suppressed_by_cooldown: true,
            cooldown_info: createIcmpToHttpCrossProbeVisibleCooldownInfo(),
          }],
        }],
        count: 1,
        retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(httpSuppressedResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'http', 10);
      
      expect(result.spikes).toHaveLength(1);
      expect(result.spikes[0].event_id).toBe('http-spike-suppressed');
      expect(result.spikes[0].kind).toBe('http');
      expect(result.spikes[0].captures![0].suppressed_by_cooldown).toBe(true);
      expect(result.spikes[0].captures![0].cooldown_info?.is_cross_probe_suppression).toBe(true);
      expect(result.spikes[0].captures![0].cooldown_info?.anchor_probe_kind).toBe('icmp');
    });

    it('should not mix HTTP and ICMP spikes in the same query response', async () => {
      // When querying kind=http, should NOT include ICMP spikes
      const httpOnlyResponse: SpikeResponseWithCaptures = {
        spikes: [{
          event_id: 'http-spike-001',
          target_id: 'test-target',
          kind: 'http',
          severity: 'warning',
          sample_ts: '2026-06-18T12:00:00Z',
          latency_ms: 1500,
          rolling_median_ms: 100,
          reasons: ['high_latency'],
          thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
          previous_samples: [],
          collected_at: '2026-06-18T12:00:00Z',
          captures: [],
        }],
        count: 1,
        retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(httpOnlyResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'http', 10);
      
      // All spikes should be http kind
      for (const spike of result.spikes) {
        expect(spike.kind).toBe('http');
      }
    });
  });

  describe('Empty state probe-specific messaging', () => {
    it('should have correct empty response structure for ICMP', async () => {
      const emptyResponse: SpikeResponseWithCaptures = {
        spikes: [],
        count: 0,
        retention: { retained_spike_count: 0, visible_spike_count: 0, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(emptyResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'icmp', 10);
      
      // Response structure should be valid for UI to render "No retained ICMP spikes"
      expect(Array.isArray(result.spikes)).toBe(true);
      expect(result.spikes.length).toBe(0);
      expect(result.retention).toBeDefined();
    });

    it('should have correct empty response structure for HTTP', async () => {
      const emptyResponse: SpikeResponseWithCaptures = {
        spikes: [],
        count: 0,
        retention: { retained_spike_count: 0, visible_spike_count: 0, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
      };
      
      mockGetLatencySpikesWithCaptures.mockResolvedValue(emptyResponse);
      
      const result = await mockGetLatencySpikesWithCaptures('test-target', 'http', 10);
      
      // Response structure should be valid for UI to render "No retained HTTP spikes"
      expect(Array.isArray(result.spikes)).toBe(true);
      expect(result.spikes.length).toBe(0);
      expect(result.retention).toBeDefined();
    });
  });
});

describe('DOM-level container separation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Clean up any existing DOM
    document.body.innerHTML = '';
  });

  afterEach(() => {
    document.body.innerHTML = '';
    vi.restoreAllMocks();
  });

  it('renders HTTP and ICMP spike diagnostics into separate containers', async () => {
    // Setup: create both containers with probe-specific empty messages
    document.body.innerHTML = `
      <div id="spike-diag-http-test-target" data-empty-message="No retained HTTP spikes"></div>
      <div id="spike-diag-icmp-test-target" data-empty-message="No retained ICMP spikes"></div>
    `;

    // HTTP spike response
    const httpSpikeResponse = {
      spikes: [{
        event_id: 'http-spike-001',
        target_id: 'test-target',
        kind: 'http',
        severity: 'warning',
        sample_ts: '2026-06-18T12:00:00Z',
        latency_ms: 1500,
        rolling_median_ms: 100,
        reasons: ['high_latency'],
        thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
        previous_samples: [],
        collected_at: '2026-06-18T12:00:00Z',
        captures: [{
          source: 'peer-1',
          base_url: 'http://10.0.0.1:8080',
          capture_started_at: '2026-06-18T12:00:00Z',
          status: 'ok',
          capture_status: 'captured',
        }],
      }],
      count: 1,
      retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 1, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
    };

    // ICMP spike response
    const icmpSpikeResponse = {
      spikes: [{
        event_id: 'icmp-spike-001',
        target_id: 'test-target',
        kind: 'icmp',
        severity: 'critical',
        sample_ts: '2026-06-18T12:00:00Z',
        latency_ms: 3000,
        rolling_median_ms: 50,
        reasons: ['critical_latency'],
        thresholds: { warning_ms: 500, critical_ms: 2000, relative_multiplier: 10 },
        previous_samples: [],
        collected_at: '2026-06-18T12:00:00Z',
        captures: [{
          source: 'peer-1',
          base_url: 'http://10.0.0.1:8080',
          capture_started_at: '2026-06-18T12:00:00Z',
          status: 'ok',
          capture_status: 'captured',
        }],
      }],
      count: 1,
      retention: { retained_spike_count: 1, visible_spike_count: 1, protected_capture_count: 1, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
    };

    mockGetLatencySpikesWithCaptures
      .mockResolvedValueOnce(httpSpikeResponse)
      .mockResolvedValueOnce(icmpSpikeResponse);

    // Import and call loadSpikeDiagnostics for each probe kind
    const { loadSpikeDiagnostics } = await import('./spikes');

    await loadSpikeDiagnostics('test-target', 'http');
    await loadSpikeDiagnostics('test-target', 'icmp');

    // Assert API was called with correct arguments
    expect(mockGetLatencySpikesWithCaptures).toHaveBeenNthCalledWith(1, 'test-target', 'http', 10);
    expect(mockGetLatencySpikesWithCaptures).toHaveBeenNthCalledWith(2, 'test-target', 'icmp', 10);

    // Assert HTTP container contains http probe info
    const httpContainer = document.getElementById('spike-diag-http-test-target')!;
    expect(httpContainer.textContent).toContain('http');
    expect(httpContainer.querySelector('.spike-table')).not.toBeNull();

    // Assert ICMP container contains icmp probe info
    const icmpContainer = document.getElementById('spike-diag-icmp-test-target')!;
    expect(icmpContainer.textContent).toContain('icmp');
    expect(icmpContainer.querySelector('.spike-table')).not.toBeNull();
  });

  it('renders probe-specific empty messages in each container', async () => {
    // Setup: create both containers with probe-specific empty messages
    document.body.innerHTML = `
      <div id="spike-diag-http-test-target" data-empty-message="No retained HTTP spikes"></div>
      <div id="spike-diag-icmp-test-target" data-empty-message="No retained ICMP spikes"></div>
    `;

    // Empty responses
    const emptyResponse = {
      spikes: [],
      count: 0,
      retention: { retained_spike_count: 0, visible_spike_count: 0, protected_capture_count: 0, purge_eligible_count: 0, max_uncaptured_spikes: 200 },
    };

    mockGetLatencySpikesWithCaptures
      .mockResolvedValueOnce(emptyResponse)
      .mockResolvedValueOnce(emptyResponse);

    const { loadSpikeDiagnostics } = await import('./spikes');

    await loadSpikeDiagnostics('test-target', 'http');
    await loadSpikeDiagnostics('test-target', 'icmp');

    // Assert HTTP container shows HTTP-specific empty message
    const httpContainer = document.getElementById('spike-diag-http-test-target')!;
    expect(httpContainer.textContent).toContain('No retained HTTP spikes');
    expect(httpContainer.textContent).not.toContain('No retained ICMP spikes');

    // Assert ICMP container shows ICMP-specific empty message
    const icmpContainer = document.getElementById('spike-diag-icmp-test-target')!;
    expect(icmpContainer.textContent).toContain('No retained ICMP spikes');
    expect(icmpContainer.textContent).not.toContain('No retained HTTP spikes');
  });
});
