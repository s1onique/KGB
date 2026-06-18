// DOM renderer tests: underlay TCP summary rendering

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
import { createXrayTcpSpikeResponse } from './spikes.render.fixtures';

describe('spikes DOM renderer: underlay TCP summary rendering', () => {
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

  it('renders xray socket name', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('xray');
  });

  it('renders ESTAB state', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('ESTAB');
  });

  it('renders "xray ESTAB" combined', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('xray ESTAB');
  });

  it('renders RTT with one decimal place', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('RTT 123.4 ms');
  });

  it('renders RTO value', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('RTO 456 ms');
  });

  it('renders retransmits count', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('retransmits 7');
  });
});
