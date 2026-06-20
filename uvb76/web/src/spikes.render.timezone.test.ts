// DOM renderer tests: timezone handling for cooldown timestamps
//
// These tests verify that timestamp parsing and rendering correctly handles
// explicit UTC instants and rejects timezone-less formats.

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
import { spikeResponseWithHiddenAnchorCooldown } from './spikes.render.fixtures';

describe('spikes DOM renderer: timezone handling', () => {
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

  // =======================================================================
  // Test Case A: Explicit UTC renders as UTC
  // =======================================================================
  describe('explicit UTC timestamps', () => {
    it('renders UTC timestamp with explicit UTC label', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T21:09:59Z',
          next_capture_eligible_at: '2026-06-20T21:11:29Z',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should render as "2026-06-20 21:09:59 UTC" not a local time
      expect(container.textContent).toContain('2026-06-20');
      expect(container.textContent).toContain('21:09:59');
      expect(container.textContent).toContain('UTC');
    });

    it('does not apply local timezone offset to UTC timestamp', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T21:09:59Z',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // The rendered time should show 21:09:59 UTC
      // NOT 00:09:59 (Moscow UTC+3) or any other local offset
      expect(container.textContent).toContain('21:09:59 UTC');
      // Should NOT show 00:09:59 (if browser is in Moscow timezone)
      expect(container.textContent).not.toContain('00:09:59');
    });
  });

  // =======================================================================
  // Test Case B: Explicit offset preserves instant
  // =======================================================================
  describe('explicit offset timestamps', () => {
    it('renders +02:00 offset as UTC instant', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T23:09:59+02:00', // 21:09:59 UTC
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should render as UTC 21:09:59
      expect(container.textContent).toContain('21:09:59 UTC');
    });

    it('renders -05:00 offset as UTC instant', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T02:09:59-05:00', // 07:09:59 UTC
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should render as UTC 07:09:59
      expect(container.textContent).toContain('07:09:59 UTC');
    });

    it('offset timestamp matches equivalent Z timestamp by instant', async () => {
      // Two responses with equivalent instants
      const response1 = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T21:09:59Z',
        },
      });
      const response2 = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T23:09:59+02:00', // same instant
        },
      });

      mockGetLatencySpikesWithCaptures.mockResolvedValue(response1);
      await loadSpikeDiagnostics('test-target');
      const text1 = container.textContent;

      // Reset container
      container.innerHTML = '';
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response2);
      await loadSpikeDiagnostics('test-target');
      const text2 = container.textContent;

      // Both should render the same UTC time
      expect(text1).toContain('21:09:59 UTC');
      expect(text2).toContain('21:09:59 UTC');
    });
  });

  // =======================================================================
  // Test Case C: Timezone-less timestamp rejected
  // =======================================================================
  describe('timezone-less timestamps', () => {
    it('renders invalid timestamp for timezone-less T format', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T21:09:59', // no Z, no offset
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should show "—" or "invalid timestamp" instead of incorrectly parsed time
      // The key is it should NOT append "UTC" to a locally-interpreted time
      expect(container.textContent).not.toContain('T21:09:59'); // raw string
    });

    it('renders invalid timestamp for space-separated format', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20 21:09:59', // space, no timezone
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Should NOT incorrectly append UTC to this
      expect(container.textContent).not.toContain('2026-06-20 21:09:59 UTC');
    });
  });

  // =======================================================================
  // Test Case D: Screenshot regression
  // =======================================================================
  describe('screenshot regression', () => {
    it('renders anchor and next eligible correctly', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-20T21:09:59Z',
          next_capture_eligible_at: '2026-06-20T21:11:29Z',
          remaining_cooldown_ms: 33000,
          anchor_visible: false,
          anchor_visibility_reason: 'outside_filter_window',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');

      // Both timestamps should be visible
      expect(container.textContent).toContain('Anchor:');
      expect(container.textContent).toContain('21:09:59 UTC');
      expect(container.textContent).toContain('Next eligible:');
      expect(container.textContent).toContain('21:11:29 UTC');
    });

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
});
