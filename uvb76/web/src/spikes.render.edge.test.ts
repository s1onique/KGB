// DOM renderer tests: edge cases (missing network_diag, API errors, missing container)

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
import { createSpikeResponse, createSpike, createOkCapture } from './spikes.render.fixtures';

describe('spikes DOM renderer: edge cases', () => {
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

  describe('missing network_diag handling', () => {
    it('does not crash when network_diag is missing', async () => {
      const response = createSpikeResponse(createSpike({
        captures: [createOkCapture({ network_diag: undefined })],
      }));
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      // Should not throw
      await expect(loadSpikeDiagnostics('test-target')).resolves.not.toThrow();
      // Should still render the capture status
      expect(container.textContent).toContain('Capture: ok');
    });
  });

  describe('API error handling', () => {
    it('renders "Spike diagnostics unavailable" on API rejection', async () => {
      mockGetLatencySpikesWithCaptures.mockRejectedValue(new Error('Network error'));
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('Spike diagnostics unavailable');
    });

    it('does not crash on API error', async () => {
      mockGetLatencySpikesWithCaptures.mockRejectedValue(new Error('Network error'));
      await expect(loadSpikeDiagnostics('test-target')).resolves.not.toThrow();
    });
  });

  describe('missing target container', () => {
    it('is a no-op and does not throw when container is missing', async () => {
      // Remove the container that was created in beforeEach
      container.remove();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(createSpikeResponse(createSpike({
        captures: [],
      })));
      // Should not throw - function returns early when container is missing
      await expect(loadSpikeDiagnostics('nonexistent-target')).resolves.not.toThrow();
      // API should NOT be called when container is missing (returns early)
      expect(mockGetLatencySpikesWithCaptures).not.toHaveBeenCalled();
    });
  });
});
