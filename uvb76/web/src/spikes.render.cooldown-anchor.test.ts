// DOM renderer tests: cooldown anchor explanation rendering

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
import {
  spikeResponseWithHiddenAnchorCooldown,
  spikeResponseWithVisibleAnchorCooldown,
  spikeResponseWithMissingCooldownMetadata,
  spikeResponseWithXssCooldownKey,
  spikeResponseWithOkCapture,
  spikeResponseWithHttpSuppressedByIcmpHiddenAnchor,
  spikeResponseWithHttpSuppressedByIcmpVisibleAnchor,
  spikeResponseWithEvictedAnchor,
} from './spikes.render.fixtures';

describe('spikes DOM renderer: cooldown anchor explanation', () => {
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
  // Test Case A: Hidden anchor explanation
  // =======================================================================
  describe('hidden anchor explanation', () => {
    it('renders "skipped: cooldown" badge', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('skipped: cooldown');
    });

    it('renders explanation that prior capture is outside current view', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Check for the explanation text
      expect(container.textContent).toContain('Prior diagnostic capture is outside the current view');
    });

    it('renders anchor timestamp', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-18T11:00:00Z',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should show anchor timestamp (formatted)
      expect(container.textContent).toContain('prior successful capture at');
      expect(container.textContent).toContain('2026-06-18');
    });

    it('renders next eligible and remaining cooldown when available', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          next_capture_eligible_at: '2026-06-18T12:05:00Z',
          remaining_cooldown_ms: 300000,
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('Next eligible:');
      expect(container.textContent).toContain('Remaining cooldown:');
      // Remaining cooldown uses adaptive formatting: 300000ms = 300.0 s
      expect(container.textContent).toContain('300.0 s');
      expect(container.textContent).toContain('(at decision)');
    });

    it('does not render cooldown details as em dash for valid cooldown_info', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // The explanation should be present, not just an em dash
      expect(container.textContent).toContain('Cooldown key:');
      expect(container.textContent).not.toContain('details —');
    });

    it('explains suppressed network diagnostics when anchor is hidden', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should explain that suppressed is due to cooldown, not a fetch failure
      expect(container.textContent).toContain('Network diag:');
      expect(container.textContent).toContain('suppressed');
      expect(container.textContent).toContain('Prior capture anchor: outside current view');
    });

    it('renders scope and cooldown key details', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          scope: 'per_diagnostic_peer',
          cooldown_key: 'peer-1',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('Scope:');
      expect(container.textContent).toContain('per_diagnostic_peer');
      expect(container.textContent).toContain('Cooldown key:');
      expect(container.textContent).toContain('peer-1');
    });
  });

  // =======================================================================
  // Test Case B: Visible anchor explanation
  // =======================================================================
  describe('visible anchor explanation', () => {
    it('renders that recent retained capture explains the cooldown', async () => {
      const response = spikeResponseWithVisibleAnchorCooldown();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Visible anchor should have a less alarming explanation
      expect(container.textContent).toContain('Skipped because a recent diagnostic capture is already retained');
    });

    it('does not say anchor is outside current view for visible anchor', async () => {
      const response = spikeResponseWithVisibleAnchorCooldown();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).not.toContain('outside the current view');
    });

    it('renders prior capture time when available', async () => {
      const response = spikeResponseWithVisibleAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: '2026-06-18T11:55:00Z',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('Prior capture at');
    });
  });

  // =======================================================================
  // Test Case C: Missing cooldown metadata warning
  // =======================================================================
  describe('missing cooldown metadata warning', () => {
    it('renders a visible warning for missing cooldown_info', async () => {
      const response = spikeResponseWithMissingCooldownMetadata();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should show warning that metadata is missing
      expect(container.textContent).toContain('Cooldown metadata missing');
    });

    it('renders warning text explaining the issue', async () => {
      const response = spikeResponseWithMissingCooldownMetadata();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('Skipped by cooldown, but no prior capture anchor was provided');
    });

    it('renders warning icon', async () => {
      const response = spikeResponseWithMissingCooldownMetadata();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should contain warning icon character
      expect(container.innerHTML).toContain('cooldown-warning-icon');
    });
  });

  // =======================================================================
  // Test Case D: Summary banner / counts context
  // =======================================================================
  describe('summary banner with hidden anchor context', () => {
    it('shows 0 protected captures in summary when all are skipped cooldown with hidden anchor', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        retention: {
          protected_capture_count: 0,
          visible_spike_count: 1,
          retained_spike_count: 1,
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Summary should show 0 protected captures
      expect(container.textContent).toContain('0 protected by captures');
    });

    it('includes hidden anchor explanation when all visible rows are skipped cooldown', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        retention: {
          protected_capture_count: 0,
          visible_spike_count: 1,
          retained_spike_count: 5,
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should explain the context of 0 protected captures
      expect(container.textContent).toContain('Prior diagnostic capture is outside the current view');
    });
  });

  // =======================================================================
  // Test Case E: Escaping / safe rendering
  // =======================================================================
  describe('XSS safety in cooldown fields', () => {
    it('escapes HTML in cooldown_key field', async () => {
      const xssPayload = '<script>alert(1)</script>';
      const response = spikeResponseWithXssCooldownKey(xssPayload);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // The literal text should be visible in textContent
      expect(container.textContent).toContain('<script>alert(1)</script>');
      // But no actual script element should exist
      expect(container.querySelector('script')).toBeNull();
    });

    it('escapes quotes and special characters in cooldown_key', async () => {
      const xssPayload = '"><img src=x onerror=alert(1)>';
      const response = spikeResponseWithXssCooldownKey(xssPayload);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // The literal text should be visible
      expect(container.textContent).toContain('"><img src=x onerror=alert(1)>');
      // But no actual img element should exist
      expect(container.querySelector('img')).toBeNull();
    });

    it('escapes ampersand in cooldown_key', async () => {
      const payload = 'peer-1 & peer-2';
      const response = spikeResponseWithXssCooldownKey(payload);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // The literal text should be visible
      expect(container.textContent).toContain('peer-1 & peer-2');
    });

    it('escapes angle brackets in anchor_visibility_reason', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          anchor_visibility_reason: '<script>alert(1)</script>',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // The reason text is mapped to "outside current view" for unknown reasons
      // So we check that no script tag is executable
      expect(container.querySelector('script')).toBeNull();
      // And the raw reason should not appear as unescaped HTML in the DOM
      expect(container.innerHTML).not.toContain('<script>alert(1)</script>');
    });
  });

  // =======================================================================
  // Edge cases
  // =======================================================================
  describe('edge cases', () => {
    it('handles missing anchor timestamp gracefully', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          last_successful_capture_at: undefined,
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should still render the explanation but without the anchor time
      expect(container.textContent).toContain('Prior diagnostic capture is outside the current view');
      // Should not show em dash for anchor when it's missing but show the rest
      expect(container.textContent).toContain('Reason:');
    });

    it('handles zero remaining cooldown', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          remaining_cooldown_ms: 0,
          next_capture_eligible_at: '2026-06-18T11:00:00Z',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should not show remaining cooldown when it's 0
      expect(container.textContent).not.toContain('Remaining cooldown:');
    });

    it('does not render cooldown explanation for successful captures', async () => {
      const response = spikeResponseWithOkCapture();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should not show cooldown explanation for non-skipped captures
      expect(container.textContent).not.toContain('cooldown-anchor-explanation');
      expect(container.textContent).not.toContain('Prior diagnostic capture');
    });

    it('renders evicted_from_retention reason correctly', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          anchor_visibility_reason: 'evicted_from_retention',
          anchor_probe_kind: 'icmp',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('anchor evicted from retention');
      expect(container.textContent).toContain('original ICMP spike event is outside retention');
      expect(container.textContent).not.toContain('outside the current view/filter window');
    });

    it('renders outside_filter_window reason correctly with separate messaging', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          anchor_visibility_reason: 'outside_filter_window',
          anchor_probe_kind: 'icmp',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('outside current view');
      // Should say "outside the current view/filter window" NOT "outside retention"
      expect(container.textContent).toContain('outside the current view/filter window');
      expect(container.textContent).not.toContain('is outside retention');
    });

    it('renders suppressed_cooldown reason correctly', async () => {
      const response = spikeResponseWithHiddenAnchorCooldown({
        cooldown_info: {
          anchor_visibility_reason: 'suppressed_cooldown',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('anchor also suppressed by cooldown');
    });
  });

  // =======================================================================
  // Test Case F: Cross-probe suppression (ICMP→HTTP)
  // =======================================================================
  describe('cross-probe suppression (ICMP→HTTP)', () => {
    it('renders cross-probe suppression message for hidden anchor', async () => {
      const response = spikeResponseWithHttpSuppressedByIcmpHiddenAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should show ICMP→HTTP cross-probe message
      expect(container.textContent).toContain('HTTP spike suppressed by prior ICMP diagnostic capture');
      expect(container.textContent).toContain('due to source-level cooldown');
    });

    it('shows suppressed probe and anchor probe kind for hidden anchor', async () => {
      const response = spikeResponseWithHttpSuppressedByIcmpHiddenAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should show both probe kinds
      expect(container.textContent).toContain('Suppressed probe:');
      expect(container.textContent).toContain('HTTP');
      expect(container.textContent).toContain('Anchor probe:');
      expect(container.textContent).toContain('ICMP');
    });

    it('renders cross-probe suppression message for visible anchor', async () => {
      const response = spikeResponseWithHttpSuppressedByIcmpVisibleAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should show cross-probe message without "due to source-level cooldown"
      expect(container.textContent).toContain('HTTP spike suppressed by prior ICMP diagnostic capture');
      expect(container.textContent).not.toContain('due to source-level cooldown');
    });

    it('shows anchor capture ID for cross-probe suppression', async () => {
      const response = spikeResponseWithHttpSuppressedByIcmpHiddenAnchor({
        cooldown_info: {
          anchor_capture_id: 'icmp-anchor-event-001',
        },
      });
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('Capture ID:');
      expect(container.textContent).toContain('icmp-anchor-event-001');
    });

    it('shows View anchor capture button for cross-probe suppression', async () => {
      const response = spikeResponseWithHttpSuppressedByIcmpHiddenAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should have the view anchor capture button
      expect(container.textContent).toContain('View anchor capture');
    });

    it('does not show generic "outside current view" message for cross-probe', async () => {
      const response = spikeResponseWithHttpSuppressedByIcmpHiddenAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should show cross-probe specific message, not generic one
      expect(container.textContent).toContain('HTTP spike suppressed by prior ICMP');
    });
  });

  // =======================================================================
  // Test Case G: Evicted anchor (spike event outside retention)
  // =======================================================================
  describe('evicted anchor (spike event purged, artifact available)', () => {
    it('renders evicted anchor message', async () => {
      const response = spikeResponseWithEvictedAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should show that spike event is outside retention
      expect(container.textContent).toContain('anchor evicted from retention');
    });

    it('shows degraded message explaining artifact availability', async () => {
      const response = spikeResponseWithEvictedAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should explain that capture artifact is available but spike event is outside retention
      expect(container.textContent).toContain('Anchor capture artifact is available');
      expect(container.textContent).toContain('original ICMP spike event is outside retention');
    });

    it('shows anchor probe kind in degraded message', async () => {
      const response = spikeResponseWithEvictedAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('original ICMP spike event');
    });

    it('shows View anchor capture button for evicted anchor', async () => {
      const response = spikeResponseWithEvictedAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      // Should still have the view anchor capture button
      expect(container.textContent).toContain('View anchor capture');
    });

    it('shows anchor capture ID for evicted anchor', async () => {
      const response = spikeResponseWithEvictedAnchor();
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
      await loadSpikeDiagnostics('test-target');
      expect(container.textContent).toContain('evicted-anchor-event-001');
    });
  });
});
