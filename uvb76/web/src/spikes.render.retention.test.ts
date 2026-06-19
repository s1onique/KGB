// DOM renderer tests: retention summary and table rendering

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import type { SpikeResponseWithCaptures } from './api';

// Mock the api module
const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) =>
      mockGetLatencySpikesWithCaptures(...args),
  },
}));

// Import after mock setup
import { loadSpikeDiagnostics } from './spikes';
import { spikeResponseWithOkCapture, defaultRetention } from './spikes.render.fixtures';

describe('spikes DOM renderer: retention summary and table', () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    vi.clearAllMocks();
    container = document.createElement('div');
    container.id = 'spike-diag-test-target';
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
  });

  it('renders retention summary with correct counts', async () => {
    const response = spikeResponseWithOkCapture();
    response.retention = {
      retained_spike_count: 137,
      visible_spike_count: 10,
      protected_capture_count: 14,
      purge_eligible_count: 123,
      max_uncaptured_spikes: 200,
    };
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Check retention summary text
    expect(container.textContent).toContain('showing 10 newest of 137 retained');
    expect(container.textContent).toContain('14 protected by captures');
    expect(container.textContent).toContain('123 purge-eligible');
    expect(container.textContent).toContain('max uncaptured retained: 200');
  });

  it('renders semantic table structure', async () => {
    const response = spikeResponseWithOkCapture();
    response.retention = defaultRetention;
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Check for semantic table elements
    const table = container.querySelector('table.spike-table');
    expect(table).not.toBeNull();
    
    const caption = container.querySelector('caption.spike-caption');
    expect(caption).not.toBeNull();
    
    const thead = container.querySelector('thead');
    expect(thead).not.toBeNull();
    
    const tbody = container.querySelector('tbody');
    expect(tbody).not.toBeNull();
  });

  it('renders table headers with scope="col"', async () => {
    const response = spikeResponseWithOkCapture();
    response.retention = defaultRetention;
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const headers = container.querySelectorAll('th[scope="col"]');
    expect(headers.length).toBe(8); // Time, Probe, Severity, Latency, Target, Thresholds, Capture, Details
    
    const headerTexts = Array.from(headers).map(h => h.textContent?.trim());
    expect(headerTexts).toContain('Time');
    expect(headerTexts).toContain('Probe');
    expect(headerTexts).toContain('Severity');
    expect(headerTexts).toContain('Latency');
    expect(headerTexts).toContain('Target');
    expect(headerTexts).toContain('Thresholds');
    expect(headerTexts).toContain('Capture');
    expect(headerTexts).toContain('Details');
  });

  it('renders spike rows in table body', async () => {
    const response = spikeResponseWithOkCapture();
    response.retention = defaultRetention;
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const rows = container.querySelectorAll('tbody tr.spike-row');
    expect(rows.length).toBe(1);
  });

  it('renders empty state with retention summary', async () => {
    const response: SpikeResponseWithCaptures = {
      spikes: [],
      count: 0,
      retention: {
        retained_spike_count: 50,
        visible_spike_count: 0,
        protected_capture_count: 5,
        purge_eligible_count: 45,
        max_uncaptured_spikes: 200,
      },
    };
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('No recent spikes');
    expect(container.textContent).toContain('showing 0 newest of 50 retained');
    expect(container.textContent).toContain('5 protected by captures');
  });

  it('renders capture status badges correctly', async () => {
    const response = spikeResponseWithOkCapture({
      network_diag: { started_at: '2026-06-18T12:00:00Z', status: 'ok', underlay_tcp: [] },
    });
    response.retention = defaultRetention;
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Should show "ready" badge for ok capture with network_diag
    expect(container.textContent).toContain('ready');
  });

  it('renders suppressed capture badge', async () => {
    const response = spikeResponseWithOkCapture();
    response.spikes[0].captures = [{
      source: 'peer-1',
      base_url: 'http://10.0.0.1:8080',
      capture_started_at: '2026-06-18T12:00:00Z',
      status: 'ok',
      suppressed_by_cooldown: true,
    }];
    response.retention = {
      retained_spike_count: 1,
      visible_spike_count: 1,
      protected_capture_count: 0,
      purge_eligible_count: 1,
      max_uncaptured_spikes: 200,
    };
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Should show "suppressed" badge
    expect(container.textContent).toContain('suppressed');
  });

  it('renders table caption with retention info', async () => {
    const response = spikeResponseWithOkCapture();
    response.retention = {
      retained_spike_count: 50,
      visible_spike_count: 10,
      protected_capture_count: 5,
      purge_eligible_count: 45,
      max_uncaptured_spikes: 200,
    };
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const caption = container.querySelector('caption.spike-caption');
    expect(caption).not.toBeNull();
    // Caption starts with "Showing" (capitalized)
    expect(caption?.textContent).toContain('Showing 10 newest of 50 retained spikes');
    expect(caption?.textContent).toContain('5 protected by captures');
    expect(caption?.textContent).toContain('45 purge-eligible');
  });

  it('renders error capture status', async () => {
    const response = spikeResponseWithOkCapture();
    response.spikes[0].captures = [{
      source: 'peer-1',
      base_url: 'http://10.0.0.1:8080',
      capture_started_at: '2026-06-18T12:00:00Z',
      status: 'error',
      error: 'connection refused',
    }];
    response.retention = {
      retained_spike_count: 1,
      visible_spike_count: 1,
      protected_capture_count: 0,
      purge_eligible_count: 1,
      max_uncaptured_spikes: 200,
    };
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('error');
  });

  it('renders timeout capture status', async () => {
    const response = spikeResponseWithOkCapture();
    response.spikes[0].captures = [{
      source: 'peer-1',
      base_url: 'http://10.0.0.1:8080',
      capture_started_at: '2026-06-18T12:00:00Z',
      status: 'timeout',
    }];
    response.retention = {
      retained_spike_count: 1,
      visible_spike_count: 1,
      protected_capture_count: 0,
      purge_eligible_count: 1,
      max_uncaptured_spikes: 200,
    };
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('timeout');
  });

  it('renders critical severity badge', async () => {
    const response = spikeResponseWithOkCapture();
    response.spikes[0].severity = 'critical';
    response.retention = defaultRetention;
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('CRITICAL');
  });

  it('renders warning severity badge', async () => {
    const response = spikeResponseWithOkCapture();
    response.spikes[0].severity = 'warning';
    response.retention = defaultRetention;
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('WARNING');
  });

  it('renders multiple spikes in table', async () => {
    const response = spikeResponseWithOkCapture();
    // Add more spikes
    response.spikes.push({
      ...response.spikes[0],
      event_id: 'evt-2',
      latency_ms: 5000,
      severity: 'critical',
      sample_ts: '2026-06-18T12:05:00Z',
    });
    response.spikes.push({
      ...response.spikes[0],
      event_id: 'evt-3',
      latency_ms: 2000,
      severity: 'warning',
      sample_ts: '2026-06-18T12:10:00Z',
    });
    response.count = 3;
    response.retention = {
      retained_spike_count: 3,
      visible_spike_count: 3,
      protected_capture_count: 3,
      purge_eligible_count: 0,
      max_uncaptured_spikes: 200,
    };
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const rows = container.querySelectorAll('tbody tr.spike-row');
    expect(rows.length).toBe(3);
  });
});
