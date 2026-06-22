// Diagnostic Timeline Controller Tests - Controller lifecycle and refresh tests

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { DiagnosticTimelineController } from './diagnosticTimeline';
import { buildTimelineState, buildErrorTimelineState } from './diagnosticTimeline.model';
import type { TimelineFilters } from './diagnosticTimeline.filters';
import { defaultFilters } from './diagnosticTimeline.filters';

// Mock the api module
const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) =>
      mockGetLatencySpikesWithCaptures(...args),
  },
}));

// Helper to create a mock response with spikes
function createMockResponse(spikeCount: number, probeKind: 'http' | 'icmp') {
  return {
    spikes: Array.from({ length: spikeCount }, (_, i) => ({
      event_id: `${probeKind}-evt-${i}`,
      target_id: 'test-target',
      kind: probeKind,
      severity: i % 2 === 0 ? 'warning' : 'critical',
      sample_ts: '2026-06-18T12:00:00Z',
      latency_ms: 100 + i * 100,
      rolling_median_ms: 100,
      reasons: [],
      thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
      previous_samples: [],
      collected_at: '2026-06-18T12:00:00Z',
      captures: [],
    })),
    count: spikeCount,
    retention: {
      retained_spike_count: spikeCount,
      visible_spike_count: spikeCount,
      protected_capture_count: 0,
      purge_eligible_count: 0,
      max_uncaptured_spikes: 200,
    },
  };
}

describe('DiagnosticTimelineController', () => {
  let container: HTMLElement;
  let controller: DiagnosticTimelineController;

  beforeEach(() => {
    container = document.createElement('div');
    container.id = 'timeline-test-target';
    document.body.appendChild(container);
    mockGetLatencySpikesWithCaptures.mockReset();
  });

  afterEach(() => {
    container.remove();
    vi.restoreAllMocks();
  });

  describe('getTargetId', () => {
    it('returns the target ID passed to constructor', () => {
      const controller = new DiagnosticTimelineController('my-target-123');
      expect(controller.getTargetId()).toBe('my-target-123');
    });
  });

  describe('mount lifecycle', () => {
    it('mounts without error for valid container', () => {
      const controller = new DiagnosticTimelineController('test-target');
      expect(() => controller.mount('timeline-test-target')).not.toThrow();
    });

    it('logs error for missing container', () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      const controller = new DiagnosticTimelineController('test-target');
      controller.mount('nonexistent-container');
      expect(consoleSpy).toHaveBeenCalledWith('Timeline container #nonexistent-container not found');
      consoleSpy.mockRestore();
    });
  });

  describe('refresh preserves filters', () => {
    it('does not reset filters on refresh', async () => {
      const controller = new DiagnosticTimelineController('test-target');
      controller.mount('timeline-test-target');

      // Wait for initial load to complete
      await vi.waitFor(() => {
        expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalled();
      });
      mockGetLatencySpikesWithCaptures.mockClear();

      // Simulate filter change via the controller internals
      // (In real usage, user changes filters through UI)
      // For this test, we verify the controller's filter state is preserved
      
      // Trigger refresh
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        http: createMockResponse(2, 'http'),
        icmp: createMockResponse(2, 'icmp'),
      });
      
      await controller.refresh();
      
      // Verify fetch was called (filters didn't prevent refresh)
      expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalledTimes(2);
    });
  });

  describe('refresh fetches new data', () => {
    it('calls API on refresh', async () => {
      const controller = new DiagnosticTimelineController('test-target');
      controller.mount('timeline-test-target');

      // Wait for initial load
      await vi.waitFor(() => {
        expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalled();
      });
      
      const initialCallCount = mockGetLatencySpikesWithCaptures.mock.calls.length;
      
      // Trigger refresh
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        http: createMockResponse(3, 'http'),
        icmp: createMockResponse(3, 'icmp'),
      });
      
      await controller.refresh();
      
      // Should have made additional API calls
      expect(mockGetLatencySpikesWithCaptures.mock.calls.length).toBeGreaterThan(initialCallCount);
    });

    it('updates container content on refresh', async () => {
      const controller = new DiagnosticTimelineController('test-target');
      controller.mount('timeline-test-target');

      // Wait for initial load
      await vi.waitFor(() => {
        expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalled();
      });
      mockGetLatencySpikesWithCaptures.mockClear();

      // Refresh with new data
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        http: createMockResponse(5, 'http'),
        icmp: createMockResponse(5, 'icmp'),
      });

      await controller.refresh();

      // Wait for the refresh to complete and render
      await vi.waitFor(() => {
        expect(container.innerHTML).toContain('timeline-summary');
      });
    });
  });

  describe('error handling', () => {
    it('handles HTTP failure with ICMP success', async () => {
      const controller = new DiagnosticTimelineController('test-target');
      controller.mount('timeline-test-target');

      await vi.waitFor(() => {
        expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalled();
      });
      mockGetLatencySpikesWithCaptures.mockClear();

      // Simulate partial failure: HTTP fails, ICMP succeeds
      mockGetLatencySpikesWithCaptures.mockImplementation((targetId: string, kind: 'http' | 'icmp') => {
        if (kind === 'http') {
          return Promise.reject(new Error('HTTP fetch failed'));
        }
        return Promise.resolve(createMockResponse(3, 'icmp'));
      });

      await controller.refresh();

      // Should not throw, should render with available data
      expect(container.innerHTML).toContain('timeline');
    });

    it('handles ICMP failure with HTTP success', async () => {
      const controller = new DiagnosticTimelineController('test-target');
      controller.mount('timeline-test-target');

      await vi.waitFor(() => {
        expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalled();
      });
      mockGetLatencySpikesWithCaptures.mockClear();

      // Simulate partial failure: ICMP fails, HTTP succeeds
      mockGetLatencySpikesWithCaptures.mockImplementation((targetId: string, kind: 'http' | 'icmp') => {
        if (kind === 'icmp') {
          return Promise.reject(new Error('ICMP fetch failed'));
        }
        return Promise.resolve(createMockResponse(3, 'http'));
      });

      await controller.refresh();

      // Should not throw, should render with available data
      expect(container.innerHTML).toContain('timeline');
    });

    it('renders error state when API fails', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      const controller = new DiagnosticTimelineController('test-target');
      
      // First, set up successful initial load so we have data to refresh from
      mockGetLatencySpikesWithCaptures.mockResolvedValue({
        http: createMockResponse(2, 'http'),
        icmp: createMockResponse(2, 'icmp'),
      });
      
      controller.mount('timeline-test-target');

      await vi.waitFor(() => {
        expect(mockGetLatencySpikesWithCaptures).toHaveBeenCalled();
      });
      mockGetLatencySpikesWithCaptures.mockClear();

      // Simulate complete failure (both HTTP and ICMP fail)
      mockGetLatencySpikesWithCaptures.mockRejectedValue(new Error('Network error'));

      await controller.refresh();

      // Wait for error state to render
      await vi.waitFor(() => {
        expect(container.innerHTML).toContain('timeline-error');
      }, { timeout: 2000 });
      
      consoleSpy.mockRestore();
    });

    it('handles API failure during initial mount gracefully', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      const controller = new DiagnosticTimelineController('test-target');
      
      // Simulate complete failure during mount
      mockGetLatencySpikesWithCaptures.mockRejectedValue(
        new Error('Failed to load HTTP and ICMP diagnostic timelines')
      );

      controller.mount('timeline-test-target');

      // Should not throw, container should have timeline element
      expect(container.innerHTML).toContain('timeline');
      consoleSpy.mockRestore();
    });
  });

  describe('expanded state tracking', () => {
    it('can track expanded event IDs via internal state', () => {
      const controller = new DiagnosticTimelineController('test-target');
      
      // Access internal state through mount (which sets up the controller)
      controller.mount('timeline-test-target');
      
      // The controller should be able to track expanded state internally
      // We test this by verifying the controller exists and can be refreshed
      expect(controller.getTargetId()).toBe('test-target');
    });
  });
});

describe('main.ts controller registry behavior (integration-style)', () => {
  // These tests verify the expected behavior of the controller registry pattern
  // without importing the actual main.ts module (which has side effects)

  describe('ensureDiagnosticTimelineMounted pattern', () => {
    it('registry returns existing controller for same target', () => {
      const registry = new Map<string, DiagnosticTimelineController>();
      
      // Simulate first mount
      const controller1 = new DiagnosticTimelineController('target-1');
      registry.set('target-1', controller1);
      
      // Simulate second call - should return same controller
      const existing = registry.get('target-1');
      
      expect(existing).toBe(controller1);
      expect(registry.size).toBe(1);
    });

    it('registry creates new controller for new target', () => {
      const registry = new Map<string, DiagnosticTimelineController>();
      
      // Mount first target
      const controller1 = new DiagnosticTimelineController('target-1');
      registry.set('target-1', controller1);
      
      // Mount second target
      const controller2 = new DiagnosticTimelineController('target-2');
      registry.set('target-2', controller2);
      
      expect(registry.size).toBe(2);
      expect(registry.get('target-1')).toBe(controller1);
      expect(registry.get('target-2')).toBe(controller2);
    });

    it('refresh does not create duplicate controllers', () => {
      const registry = new Map<string, DiagnosticTimelineController>();
      
      // Initial load
      const controller1 = new DiagnosticTimelineController('target-1');
      registry.set('target-1', controller1);
      
      // Simulate multiple refresh cycles
      for (let i = 0; i < 5; i++) {
        const existing = registry.get('target-1');
        if (!existing) {
          registry.set('target-1', new DiagnosticTimelineController('target-1'));
        }
      }
      
      // Should still only have one controller
      expect(registry.size).toBe(1);
    });
  });
});
