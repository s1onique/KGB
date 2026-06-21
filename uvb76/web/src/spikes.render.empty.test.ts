// DOM renderer tests: empty response handling

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

describe('spikes DOM renderer: empty response', () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    vi.clearAllMocks();
    container = document.createElement('div');
    container.id = 'spike-diag-http-test-target';
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
  });

  it('renders "No recent spikes" when response has no spikes', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue({ spikes: [], count: 0 });
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('No recent spikes');
  });

  it('renders "No recent spikes" when spikes array is empty', async () => {
    mockGetLatencySpikesWithCaptures.mockResolvedValue({ spikes: [], count: 0 });
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('No recent spikes');
  });
});
