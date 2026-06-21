// DOM renderer tests: timezone handling for spike/capture diagnostics
//
// These tests verify that timestamp rendering is CONSISTENTLY in local/browser
// timezone (matching the table row timestamps), NOT mixed with UTC display.
//
// FIXED: Spike table row uses local time, capture details anchor/next-eligible
// now also use local time (same as table), ensuring consistent UI.

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
import { spikeResponseWithHiddenAnchorCooldown, createSpike } from './spikes.render.fixtures';

describe('spikes DOM renderer: timezone handling', () => {
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

  // =======================================================================
  // Test Case A: Local time consistency (the fix)
  // =======================================================================
  describe('local time consistency (ACT regression fix)', () => {
    it('renders local time without UTC suffix in capture details', async () => {
      // ACT scenario: spike at 08:40:25 UTC, anchor at 08:39:56 UTC
      // In Helsinki (UTC+3), both should show local time: 11:40:25 and 11:39:56
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-21T08:39:56Z',
          next_capture_eligible_at: '2026-06-21T08:41:26Z',
          remaining_cooldown_ms: 33000,
          anchor_visible: false,
          anchor_visibility_reason: 'outside_filter_window',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Capture details should NOT contain "UTC" suffix (the regression was mixing times)
      // Anchor label should be visible
      expect(container.textContent).toContain('Anchor:');
      expect(container.textContent).toContain('Next eligible:');

      // Should NOT show raw UTC strings like "2026-06-21 08:39:56 UTC"
      // Instead should show local time (format: YYYY-MM-DD HH:mm:ss without UTC)
      expect(container.textContent).not.toContain('08:39:56 UTC');
      expect(container.textContent).not.toContain('08:41:26 UTC');

      // Should NOT show raw ISO strings
      expect(container.textContent).not.toContain('T08:39:56Z');
      expect(container.textContent).not.toContain('T08:41:26Z');
    });

    it('renders local time format (YYYY-MM-DD HH:mm:ss) in capture details', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-21T08:39:56Z',
          next_capture_eligible_at: '2026-06-21T08:41:26Z',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // The rendered anchor time should match format YYYY-MM-DD HH:mm:ss
      // (local time, no UTC suffix - consistent with spike table row)
      expect(container.textContent).toMatch(/Anchor:.*\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/);
      expect(container.textContent).toMatch(/Next eligible:.*\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/);
    });
  });

  // =======================================================================
  // Test Case B: Timezone-less timestamp handling
  // =======================================================================
  describe('timezone-less timestamps', () => {
    it('renders em dash for timezone-less T format', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T21:09:59', // no Z, no offset
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should show "—" for invalid timestamp
      // Should NOT show raw string or incorrectly format it
      expect(container.textContent).not.toContain('T21:09:59'); // raw string
    });

    it('renders em dash for space-separated format', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20 21:09:59', // space, no timezone
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should NOT show this incorrectly formatted
      expect(container.textContent).not.toContain('2026-06-20 21:09:59 UTC');
      expect(container.textContent).not.toContain('2026-06-20 21:09:59');
    });
  });

  // =======================================================================
  // Test Case C: Cooldown remaining time formatting
  // =======================================================================
  describe('cooldown remaining time formatting', () => {
    it('renders remaining cooldown with (at decision) label', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          remaining_cooldown_ms: 33000,
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should show adaptive formatting: 33.0 s not 33000 ms
      expect(container.textContent).toContain('Remaining cooldown:');
      expect(container.textContent).toContain('33.0 s');
      expect(container.textContent).toContain('(at decision)');
    });

    it('formats large cooldown in seconds, small in milliseconds', async () => {
      // 90 seconds = 90000ms should show "90.0 s"
      const response90s = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          remaining_cooldown_ms: 90000,
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response90s);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('90.0 s');

      // 500ms should show "500 ms"
      container.innerHTML = '';
      const response500ms = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          remaining_cooldown_ms: 500,
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response500ms);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('500 ms');
    });
  });

  // =======================================================================
  // Test Case D: Spike row timestamp consistency
  // =======================================================================
  describe('spike row timestamp', () => {
    it('renders spike time in local timezone', async () => {
      // Spike timestamp at 08:40:25 UTC should render as local time
      const spikeTimestamp = '2026-06-21T08:40:25Z';
      const response = spikeResponseWithHiddenAnchorCooldown({
        latency_ms: 800,
        cooldown_info: {
          last_successful_capture_at: '2026-06-21T08:39:56Z',
        },
      });
      // Override spike timestamp
      response.spikes[0].sample_ts = spikeTimestamp;

      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Spike row should show time (HH:mm:ss format from formatSpikeTime)
      // The formatSpikeTime uses Intl.DateTimeFormat with browser timezone
      expect(container.textContent).toMatch(/\d{2}:\d{2}:\d{2}/);

      // Should NOT show raw UTC in spike row
      expect(container.textContent).not.toContain('08:40:25 UTC');
    });
  });

  // =======================================================================
  // Test Case E: Visible anchor timestamp
  // =======================================================================
  describe('visible anchor timestamp', () => {
    it('renders visible anchor with local time format', async () => {
      const { spikeResponseWithVisibleAnchorCooldown } = await import('./spikes.render.fixtures');
      const response = spikeResponseWithVisibleAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-21T08:39:56Z',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should show "Prior capture at" with local time format
      expect(container.textContent).toContain('Prior capture at');

      // Should NOT show UTC suffix for visible anchor
      expect(container.textContent).not.toContain('08:39:56 UTC');

      // Should match local time format
      expect(container.textContent).toMatch(/Prior capture at.*\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/);
    });
  });
});
