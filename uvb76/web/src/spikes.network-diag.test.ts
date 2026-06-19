// DOM renderer tests: Network diagnostics missing warning

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) =>
      mockGetLatencySpikesWithCaptures(...args),
  },
}));

import { loadSpikeDiagnostics } from './spikes';
import {
  spikeResponseWithOkCapture,
  createSpikeResponse,
  createSpike,
  createOkCapture,
  createNetworkDiag,
  createTcpSocket,
} from './spikes.render.fixtures';

describe('spikes DOM renderer: network diagnostics missing warning', () => {
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

  it('renders Network diag: ok for ok capture with network_diag', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('ok');
    expect(container.textContent).not.toContain('missing');
  });

  it('renders Network diag: missing for ok capture without network_diag', async () => {
    // Create a capture with status 'ok' but no network_diag
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer-1', network_diag: undefined })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('missing');
  });

  it('renders missing network diag status for ok capture without network_diag', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer-1', network_diag: undefined })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // In table view, shows "Network diag: missing" in expanded details
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('missing');
  });

  it('does not render incomplete warning for suppressed capture', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [{
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
      }],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).not.toContain('Capture succeeded, but network diagnostics were not included');
  });

  it('does not render incomplete warning for error capture', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [{
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'error',
        error: 'connection refused',
      }],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).not.toContain('Capture succeeded, but network diagnostics were not included');
  });

  it('renders suppressed for network_diag status when suppressed_by_cooldown', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [{
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
        network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [createTcpSocket()] }),
      }],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Should show suppressed, not ok
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('suppressed');
    expect(container.textContent).not.toContain('Network diag: ok');
  });

  it('renders Underlay TCP summary when network_diag present', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 50.0, rto_ms: 200, retransmits: 0 })],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('Underlay TCP:');
  });

  it('renders no TCP summary when network_diag missing', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer-1', network_diag: undefined })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Should not have Underlay TCP section in compact view when network_diag missing
    // The compact view shows Network diag: missing, but no Underlay TCP
    expect(container.textContent).not.toContain('Underlay TCP:');
  });
});
