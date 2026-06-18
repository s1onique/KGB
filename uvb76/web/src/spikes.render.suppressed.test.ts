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

  it('renders "suppressed by cooldown"', async () => {
    const response = spikeResponseWithSuppressedCapture();
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('suppressed by cooldown');
  });

  it('does not render network_diag content for suppressed capture', async () => {
    const response = spikeResponseWithSuppressedCapture();
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // Network diag should NOT appear for suppressed captures
    expect(container.textContent).not.toContain('Network diag:');
    expect(container.textContent).not.toContain('xray');
    expect(container.textContent).not.toContain('RTT');
  });
});
