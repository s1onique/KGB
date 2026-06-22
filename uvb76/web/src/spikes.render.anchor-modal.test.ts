// DOM renderer tests: anchor capture modal timestamp rendering
//
// These tests verify that the anchor capture modal renders timestamps consistently
// in local time (not raw UTC RFC3339) and uses accurate degradation messaging.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// Mock the api module
const mockGetAnchorCapture = vi.fn();

vi.mock('./api', () => ({
  api: {
    getAnchorCapture: (...args: unknown[]) => mockGetAnchorCapture(...args),
  },
}));

// Import after mock setup
import { displayAnchorCaptureModal } from './spikes';

describe('spikes DOM renderer: anchor capture modal', () => {
  let overlay: HTMLDivElement | null = null;

  beforeEach(() => {
    vi.clearAllMocks();
    // Remove any existing overlay
    const existing = document.querySelector('.anchor-modal-overlay');
    if (existing) existing.remove();
  });

  afterEach(() => {
    // Clean up modal after each test
    const modal = document.querySelector('.anchor-modal-overlay');
    if (modal) modal.remove();
  });

  // =======================================================================
  // Test Case A: UTC timestamp renders as local time
  // =======================================================================
  describe('timestamp formatting', () => {
    it('renders provenance timestamp in local time format (YYYY-MM-DD HH:mm:ss)', () => {
      // Use a timestamp that's safe for any timezone (midday UTC, won't cross date boundary)
      // UTC 2026-06-21T12:00:00Z will be 2026-06-21 in UTC and all positive offsets up to +12
      const response = {
        status: 'available' as const,
        degraded: false,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
          anchor_created_at: '2026-06-21T12:00:00Z', // UTC midday - safe for all timezones
          anchor_probe_kind: 'icmp',
        },
        capture: {
          source: 'test-peer',
          capture_started_at: '2026-06-21T12:00:00Z',
          capture_finished_at: '2026-06-21T12:00:06Z',
          status: 'ok' as const,
          duration_ms: 5923,
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      expect(modal).not.toBeNull();

      const text = modal!.textContent || '';
      
      // Should NOT show raw UTC with Z suffix
      expect(text).not.toContain('T12:00:00Z');
      
      // Should show formatted date (2026-06-21 in all timezones since it's midday UTC)
      expect(text).toContain('2026-06-21');
      
      // Should show formatted time (HH:mm:ss format without Z)
      expect(text).toMatch(/\d{2}:\d{2}:\d{2}/);
    });

    it('renders capture artifact timestamps in local time format', () => {
      const response = {
        status: 'available' as const,
        degraded: false,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
          anchor_created_at: '2026-06-21T08:39:56Z',
        },
        capture: {
          source: 'test-peer',
          capture_started_at: '2026-06-21T08:40:00Z',
          capture_finished_at: '2026-06-21T08:40:05Z',
          status: 'ok' as const,
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      const text = modal!.textContent || '';

      // Should show "Started:" and "Finished:" labels
      expect(text).toContain('Started:');
      expect(text).toContain('Finished:');

      // Should NOT show raw RFC3339 strings
      expect(text).not.toContain('T08:40:00Z');
      expect(text).not.toContain('T08:40:05Z');

      // Should show formatted date
      expect(text).toContain('2026-06-21');
    });

    it('shows timezone indicator in modal header', () => {
      const response = {
        status: 'available' as const,
        degraded: false,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      const text = modal!.textContent || '';

      // Should show timezone indicator
      expect(text).toContain('local timezone');
      
      // Should contain timezone name in parentheses (e.g., Europe/Moscow, America/New_York)
      expect(text).toMatch(/\([A-Za-z_/]+\)/);
    });
  });

  // =======================================================================
  // Test Case B: Degraded reason messaging
  // =======================================================================
  describe('degraded reason messaging', () => {
    it('renders spike_event_evicted reason correctly', () => {
      const response = {
        status: 'artifact_missing' as const,
        degraded: true,
        degradation_reason: 'spike_event_evicted' as const,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
          anchor_created_at: '2026-06-21T08:39:56Z',
        },
        capture: {
          source: 'test-peer',
          capture_started_at: '2026-06-21T08:39:56Z',
          status: 'ok' as const,
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      const text = modal!.textContent || '';

      // Should show accurate messaging about spike event outside retention
      expect(text).toContain('spike event is outside retention');
      expect(text).not.toContain('capture artifact is missing');
    });

    it('renders artifact_purged reason correctly', () => {
      const response = {
        status: 'artifact_missing' as const,
        degraded: true,
        degradation_reason: 'artifact_purged' as const,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
          anchor_created_at: '2026-06-21T08:39:56Z',
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      const text = modal!.textContent || '';

      // Should show messaging about artifact being purged
      expect(text).toContain('capture artifact has been purged');
      expect(text).not.toContain('outside retention');
    });

    it('renders missing_provenance reason correctly', () => {
      const response = {
        status: 'metadata_only' as const,
        degraded: true,
        degradation_reason: 'missing_provenance' as const,
        message: 'metadata only',
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      const text = modal!.textContent || '';

      // Should show messaging about missing provenance
      expect(text).toContain('provenance metadata is missing');
    });

    it('does not say "outside retention" when reason is artifact_purged', () => {
      const response = {
        status: 'artifact_missing' as const,
        degraded: true,
        degradation_reason: 'artifact_purged' as const,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      const text = modal!.textContent || '';

      // Should NOT say "outside retention" for artifact_purged
      expect(text).not.toContain('spike event is outside retention');
      expect(text).not.toContain('outside retention window');
    });
  });

  // =======================================================================
  // Test Case C: Available (non-degraded) status
  // =======================================================================
  describe('available status', () => {
    it('shows available badge when not degraded', () => {
      const response = {
        status: 'available' as const,
        degraded: false,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
          anchor_created_at: '2026-06-21T08:39:56Z',
        },
        capture: {
          source: 'test-peer',
          capture_started_at: '2026-06-21T08:39:56Z',
          status: 'ok' as const,
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const modal = document.querySelector('.anchor-modal-content');
      const text = modal!.textContent || '';

      // Should show "Available" badge
      expect(text).toContain('Available');
      expect(text).toContain('artifact is available');
      
      // Should NOT show "Degraded" badge
      expect(text).not.toContain('Degraded');
    });
  });

  // =======================================================================
  // Test Case D: JSON download preserves raw timestamps
  // =======================================================================
  describe('JSON download preserves raw data', () => {
    it('download button exists when capture is available', () => {
      const response = {
        status: 'available' as const,
        degraded: false,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
          anchor_created_at: '2026-06-21T08:39:56Z',
        },
        capture: {
          source: 'test-peer',
          capture_started_at: '2026-06-21T08:39:56Z',
          status: 'ok' as const,
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const downloadBtn = document.querySelector('.download-anchor-btn');
      expect(downloadBtn).not.toBeNull();
      expect(downloadBtn!.textContent).toContain('Download anchor JSON');
    });

    it('does not show download button when no capture', () => {
      const response = {
        status: 'not_found' as const,
        degraded: true,
        degradation_reason: 'missing_provenance' as const,
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const downloadBtn = document.querySelector('.download-anchor-btn');
      expect(downloadBtn).toBeNull();
    });
  });

  // =======================================================================
  // Test Case E: Close button and overlay click
  // =======================================================================
  describe('modal interactions', () => {
    it('removes modal when close button is clicked', () => {
      const response = {
        status: 'available' as const,
        degraded: false,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const overlay = document.querySelector('.anchor-modal-overlay');
      expect(overlay).not.toBeNull();

      const closeBtn = document.querySelector('.close-anchor-modal-btn') as HTMLButtonElement;
      closeBtn.click();

      const overlayAfter = document.querySelector('.anchor-modal-overlay');
      expect(overlayAfter).toBeNull();
    });

    it('removes modal when overlay is clicked', () => {
      const response = {
        status: 'available' as const,
        degraded: false,
        anchor: {
          anchor_capture_id: 'test-anchor-001',
        },
      };
      mockGetAnchorCapture.mockResolvedValue(response);

      displayAnchorCaptureModal(response, 'test-target');

      const overlay = document.querySelector('.anchor-modal-overlay') as HTMLDivElement;
      expect(overlay).not.toBeNull();

      // Click on the overlay (not the modal content)
      overlay.click();

      const overlayAfter = document.querySelector('.anchor-modal-overlay');
      expect(overlayAfter).toBeNull();
    });
  });
});
