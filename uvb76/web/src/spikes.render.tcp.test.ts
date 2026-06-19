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

  it('renders "xray ESTAB" combined in expanded details', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    // TCP details are in expanded view - need to click View details
    const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 10));
    expect(container.textContent).toContain('xray');
    expect(container.textContent).toContain('ESTAB');
  });

  it('renders RTT with one decimal place in expanded details', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 10));
    // RTT is formatted as "RTT:" in expanded details
    expect(container.textContent).toContain('RTT:');
    expect(container.textContent).toContain('123.4 ms');
  });

  it('renders RTO value in expanded details', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 10));
    expect(container.textContent).toContain('RTO:');
    expect(container.textContent).toContain('456 ms');
  });

  it('renders retransmits count in expanded details', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue(createXrayTcpSpikeResponse());
    await loadSpikeDiagnostics('test-target');
    const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 10));
    expect(container.textContent).toContain('retransmits:');
    expect(container.textContent).toContain('7');
  });
});
