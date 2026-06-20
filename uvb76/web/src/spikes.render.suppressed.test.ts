// DOM renderer tests: suppressed capture rendering

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
import { spikeResponseWithSuppressedCapture } from './spikes.render.fixtures';

describe('spikes DOM renderer: suppressed capture rendering', () => {
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

  it('renders "skipped: cooldown" for suppressed captures', async () => {
    const response = spikeResponseWithSuppressedCapture();
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('skipped: cooldown');
  });

  it('shows suppressed status for network_diag on suppressed captures', async () => {
    const response = spikeResponseWithSuppressedCapture();
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // Network diag should show "suppressed" status for suppressed captures
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('suppressed');
    // But should NOT show the actual network_diag content as if it was successfully captured
    expect(container.textContent).not.toContain('xray');
    expect(container.textContent).not.toContain('RTT:');
  });
});
