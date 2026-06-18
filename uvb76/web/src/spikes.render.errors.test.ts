// DOM renderer tests: error capture rendering

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
import { spikeResponseWithErrorCapture } from './spikes.render.fixtures';

describe('spikes DOM renderer: error capture rendering', () => {
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

  it('renders "Capture: error" for error status', async () => {
    const response = spikeResponseWithErrorCapture({ error: 'connection refused' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    expect(container.textContent).toContain('Capture: error');
  });

  it('renders safe/truncated error text', async () => {
    const response = spikeResponseWithErrorCapture({ error: 'connection refused' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // Error text should be visible in textContent (escaped)
    expect(container.textContent).toContain('connection refused');
  });

  it('truncates long error messages', async () => {
    const response = spikeResponseWithErrorCapture({ error: 'A'.repeat(100), truncated: true });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // The ellipsis character should appear for truncated errors
    expect(container.textContent).toContain('…');
  });
});
