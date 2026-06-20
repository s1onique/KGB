// DOM renderer tests: successful capture rendering

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

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
import {
  spikeResponseWithOkCapture,
  createNetworkDiag,
} from './spikes.render.fixtures';

describe('spikes DOM renderer: successful capture rendering', () => {
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

  it('renders spike diagnostics header', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'tovarisch-peer',
      duration_ms: 123,
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('Spike diagnostics');
  });

  it('renders probe kind', async () => {
    const response = spikeResponseWithOkCapture();
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('http');
  });

  it('renders severity', async () => {
    const response = spikeResponseWithOkCapture();
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // Severity is displayed as uppercase badge in table
    expect(container.textContent).toContain('WARNING');
  });

  it('renders formatted latency', async () => {
    const response = spikeResponseWithOkCapture({ latency_ms: 1234 });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // 1234ms should render as "1.23 s" (adaptive formatting)
    expect(container.textContent).toContain('1.23 s');
  });

  it('renders "Capture: captured" for successful capture', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'tovarisch-peer',
      duration_ms: 123,
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // Capture status is "captured" for status='ok'
    expect(container.textContent).toContain('Capture: captured');
  });

  it('renders source peer name', async () => {
    const response = spikeResponseWithOkCapture({ source: 'tovarisch-peer' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('tovarisch-peer');
  });

  it('renders "Network diag:" label', async () => {
    const response = spikeResponseWithOkCapture({
      network_diag: createNetworkDiag({ underlay_tcp: [] }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('Network diag:');
  });

  it('renders network_diag status "ok"', async () => {
    const response = spikeResponseWithOkCapture({
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('ok');
  });
});
