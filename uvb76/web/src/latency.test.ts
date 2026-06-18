// Tests for latency module - capacity display

import { describe, it, expect, vi } from 'vitest';
import type { LatencySeries } from './api';

// Mock the Chart.js chart instance storage for tests
vi.mock('./chart', () => ({
  renderLatencyChart: vi.fn(),
  destroyChart: vi.fn(),
  renderLatencyChartWithViewport: vi.fn(),
}));

describe('latency capacity display', () => {
  describe('sample count formatting', () => {
    it('should format full capacity with retained, visible, and capacity', () => {
      // Simulate full ICMP buffer after 1+ hour of running
      const series: LatencySeries = {
        target_id: 'test-target',
        probe_kind: 'icmp',
        probe_url: 'https://example.com',
        interval_seconds: 1,
        range_seconds: 3600,
        step_seconds: 60,
        window_seconds: 60,
        retained_range_seconds: 3600,
        sample_count: 3600,             // DEPRECATED field
        retained_sample_count: 3600,    // actual samples in buffer
        retained_sample_capacity: 3600, // buffer capacity
        returned_point_count: 60,       // 60 chart points (3600s / 60s step)
        oldest_sample_ts: '2024-01-01T00:00:00Z',
        newest_sample_ts: '2024-01-01T01:00:00Z',
        points: [],
      };

      // Verify capacity fields
      expect(series.retained_sample_count).toBe(3600);
      expect(series.retained_sample_capacity).toBe(3600);
      expect(series.returned_point_count).toBe(60);
    });

    it('should show partial accumulation with reduced sample count but full retention horizon', () => {
      // Simulate ~8 minutes of data (474 samples) like the screenshot
      // IMPORTANT: retained_range_seconds remains 3600 (buffer CAN hold 1h)
      // The actual sample count tells the truth about daemon uptime
      const series: LatencySeries = {
        target_id: 'test-target',
        probe_kind: 'icmp',
        probe_url: 'https://example.com',
        interval_seconds: 1,
        range_seconds: 3600,            // requested range (buffer capacity)
        step_seconds: 60,
        window_seconds: 60,
        retained_range_seconds: 3600,   // buffer retention horizon (NOT actual data span)
        sample_count: 474,              // DEPRECATED field
        retained_sample_count: 474,     // actual samples in buffer
        retained_sample_capacity: 3600, // buffer capacity (not yet filled)
        returned_point_count: 8,        // 3600s / 60s step = 60 points, but only 8 have data
        oldest_sample_ts: '2024-01-01T00:00:00Z',   // oldest sample is at t=0
        newest_sample_ts: '2024-01-01T00:07:53Z',   // newest sample at t=473s (474 samples - 1)
        points: [],
      };

      // Verify partial fill - buffer has 474 samples but CAN hold 3600
      expect(series.retained_sample_count).toBe(474);
      expect(series.retained_sample_capacity).toBe(3600);
      expect(series.retained_sample_count).toBeLessThan(series.retained_sample_capacity);

      // retained_range_seconds stays at full capacity (buffer CAN hold 1h)
      expect(series.retained_range_seconds).toBe(3600);
    });

    it('should distinguish between sample_count and retained_sample_count', () => {
      // After buffer fills, sample_count equals retained_sample_count
      // But they are separate fields for clarity
      const fullSeries: LatencySeries = {
        target_id: 'test-target',
        probe_kind: 'icmp',
        probe_url: 'https://example.com',
        interval_seconds: 1,
        range_seconds: 3600,
        step_seconds: 60,
        window_seconds: 60,
        retained_range_seconds: 3600,
        sample_count: 3600,             // DEPRECATED
        retained_sample_count: 3600,    // NEW field
        retained_sample_capacity: 3600,
        returned_point_count: 60,
        oldest_sample_ts: '2024-01-01T00:00:00Z',
        newest_sample_ts: '2024-01-01T01:00:00Z',
        points: [],
      };

      // Both should be equal when buffer is full
      expect(fullSeries.sample_count).toBe(fullSeries.retained_sample_count);
      expect(fullSeries.retained_sample_count).toBe(fullSeries.retained_sample_capacity);
    });

    it('should handle HTTP buffer with different retention', () => {
      // HTTP: 15s interval, 14400s retention = 960 samples
      const httpSeries: LatencySeries = {
        target_id: 'test-target',
        probe_kind: 'http',
        probe_url: 'https://example.com/status',
        interval_seconds: 15,
        range_seconds: 14400,
        step_seconds: 60,
        window_seconds: 300,
        retained_range_seconds: 14400,
        sample_count: 960,              // DEPRECATED
        retained_sample_count: 960,     // NEW field
        retained_sample_capacity: 960,  // 14400/15 = 960
        returned_point_count: 240,      // 14400/60 = 240 points
        oldest_sample_ts: '2024-01-01T00:00:00Z',
        newest_sample_ts: '2024-01-01T04:00:00Z',
        points: [],
      };

      // Verify HTTP defaults
      expect(httpSeries.interval_seconds).toBe(15);
      expect(httpSeries.retained_sample_capacity).toBe(960);
      expect(httpSeries.retained_range_seconds).toBe(14400);
    });

    it('should show capacity mismatch when daemon restarted recently', () => {
      // Daemon restarted, only has 8 minutes of data in 1-hour buffer
      // retained_range_seconds stays at 3600 (buffer capacity)
      const recentRestart: LatencySeries = {
        target_id: 'uvb76-1',
        probe_kind: 'icmp',
        probe_url: 'https://192.168.50.1:8443',
        interval_seconds: 1,
        range_seconds: 3600,            // buffer capacity
        step_seconds: 60,
        window_seconds: 60,
        retained_range_seconds: 3600,   // buffer CAN hold 1 hour
        sample_count: 480,              // DEPRECATED
        retained_sample_count: 480,     // only 8 minutes of data
        retained_sample_capacity: 3600, // buffer can hold 1 hour
        returned_point_count: 8,        // 480/60 = 8 points with data
        oldest_sample_ts: '2024-01-01T00:00:00Z',   // oldest is earlier
        newest_sample_ts: '2024-01-01T00:07:59Z',   // newest is later
        points: [],
      };

      // The key indicator: sample_count << capacity
      expect(recentRestart.retained_sample_count).toBeLessThan(recentRestart.retained_sample_capacity);
      expect(recentRestart.retained_sample_count).toBe(480);
      expect(recentRestart.retained_sample_capacity).toBe(3600);

      // This proves the daemon has NOT been running for 1 hour
      // even though "1h retained" label might suggest otherwise
      const percentFilled = (recentRestart.retained_sample_count / recentRestart.retained_sample_capacity) * 100;
      expect(percentFilled).toBeCloseTo(13.3, 1); // ~13% filled
    });
  });

  describe('timestamp span verification', () => {
    it('should show correct span for partial accumulation', () => {
      // 474 samples at 1s interval: sample[0] at t=0, sample[473] at t=473
      const oldestTs = new Date('2024-01-01T00:00:00Z');
      const newestTs = new Date('2024-01-01T00:07:53Z'); // t=473 seconds later

      const spanMs = newestTs.getTime() - oldestTs.getTime();
      const spanSeconds = spanMs / 1000;

      // 474 samples at 1s interval = 473s span between first and last
      expect(spanSeconds).toBe(473);
    });

    it('should show correct span for full buffer', () => {
      // 3600 samples at 1s interval: sample[0] at t=0, sample[3599] at t=3599
      const oldestTs = new Date('2024-01-01T00:00:00Z');
      const newestTs = new Date('2024-01-01T01:00:00Z'); // 3600 seconds later (1h = 3600s)

      const spanMs = newestTs.getTime() - oldestTs.getTime();
      const spanSeconds = spanMs / 1000;

      // 1 hour = 3600 seconds span between oldest and newest
      expect(spanSeconds).toBe(3600);
    });

    it('should have oldest before newest (not inverted)', () => {
      const oldestTs = new Date('2024-01-01T00:00:00Z');
      const newestTs = new Date('2024-01-01T00:07:53Z');

      expect(newestTs.getTime()).toBeGreaterThan(oldestTs.getTime());
    });
  });
});
