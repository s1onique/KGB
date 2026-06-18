import { describe, it, expect } from 'vitest';
import {
  TimeViewport,
  createViewport,
  createPresetViewport,
  createFullViewport,
  panLeft,
  panRight,
  zoomIn,
  zoomOut,
  jumpToNow,
  clampToRetained,
  advanceToNow,
  isInViewport,
  getViewportSpanSeconds,
  formatViewportSpan,
  ICMP_PRESETS,
  HTTP_PRESETS,
} from './viewport';

describe('TimeViewport', () => {
  describe('createViewport', () => {
    it('should create viewport with correct span', () => {
      const viewport = createViewport(300, 3600);
      const span = viewport.endMs - viewport.startMs;
      
      expect(span).toBe(300000); // 300 seconds in ms
      expect(viewport.followNow).toBe(true);
    });

    it('should cap viewport at retained range', () => {
      const viewport = createViewport(7200, 3600); // 2h window but 1h retained
      const span = viewport.endMs - viewport.startMs;
      
      expect(span).toBe(3600000); // capped at 3600s = 1h
    });
  });

  describe('createPresetViewport', () => {
    it('should create viewport for 1m preset', () => {
      const preset = ICMP_PRESETS[0]; // 1m
      const viewport = createPresetViewport(preset);
      const span = viewport.endMs - viewport.startMs;
      
      expect(span).toBe(60000); // 60 seconds
      expect(viewport.followNow).toBe(true);
    });

    it('should respect followNow parameter', () => {
      const preset = ICMP_PRESETS[0];
      const viewport = createPresetViewport(preset, false);
      
      expect(viewport.followNow).toBe(false);
    });
  });

  describe('createFullViewport', () => {
    it('should show full retained range', () => {
      const viewport = createFullViewport(3600);
      const span = viewport.endMs - viewport.startMs;
      
      expect(span).toBe(3600000);
      expect(viewport.followNow).toBe(false);
    });
  });

  describe('panLeft', () => {
    it('should move viewport into older data', () => {
      const viewport: TimeViewport = {
        startMs: 1000000,
        endMs: 1001000,
        followNow: true,
      };
      
      const result = panLeft(viewport);
      
      // 1000ms span * 0.25 = 250ms pan left
      expect(result.startMs).toBe(999750);  // 1000000 - 250
      expect(result.endMs).toBe(1000750);  // 1001000 - 250
      expect(result.followNow).toBe(false);
    });
  });

  describe('panRight', () => {
    it('should move viewport into newer data', () => {
      const viewport: TimeViewport = {
        startMs: 1000000,
        endMs: 1001000,
        followNow: true,
      };
      
      const result = panRight(viewport);
      
      // 1000ms span * 0.25 = 250ms pan right
      expect(result.startMs).toBe(1000250);  // 1000000 + 250
      expect(result.endMs).toBe(1001250);    // 1001000 + 250
      expect(result.followNow).toBe(false);
    });
  });

  describe('zoomIn', () => {
    it('should reduce visible span', () => {
      const viewport: TimeViewport = {
        startMs: 0,
        endMs: 200000, // 200 seconds, well above minimum
        followNow: true,
      };
      
      const result = zoomIn(viewport);
      const newSpan = result.endMs - result.startMs;
      
      // 200000 * 0.5 = 100000 (50% reduction)
      expect(newSpan).toBe(100000);
      expect(result.followNow).toBe(false);
    });

    it('should respect minimum 1 minute span', () => {
      const viewport: TimeViewport = {
        startMs: 0,
        endMs: 30000, // 30 seconds, below minimum
        followNow: true,
      };
      
      const result = zoomIn(viewport);
      const newSpan = result.endMs - result.startMs;
      
      expect(newSpan).toBe(60000); // Minimum 1 minute enforced
    });
  });

  describe('zoomOut', () => {
    it('should increase visible span', () => {
      const viewport: TimeViewport = {
        startMs: 0,
        endMs: 100000,
        followNow: true,
      };
      
      const result = zoomOut(viewport);
      const newSpan = result.endMs - result.startMs;
      
      // 100000 * 1.5 = 150000 (50% increase)
      expect(newSpan).toBe(150000);
      expect(result.followNow).toBe(false);
    });
  });

  describe('jumpToNow', () => {
    it('should re-enable follow mode and keep span', () => {
      const now = Date.now();
      const viewport: TimeViewport = {
        startMs: now - 300000,
        endMs: now - 100000,
        followNow: false,
      };
      
      const result = jumpToNow(viewport);
      const span = result.endMs - result.startMs;
      
      expect(span).toBe(200000); // Same span
      expect(result.followNow).toBe(true);
    });
  });

  describe('clampToRetained', () => {
    it('should clamp viewport that extends before oldest sample', () => {
      const now = Date.now();
      const viewport: TimeViewport = {
        startMs: now - 7200000, // 2 hours ago
        endMs: now - 3600000,   // 1 hour ago
        followNow: false,
      };
      
      const result = clampToRetained(viewport, 3600); // 1 hour retained
      
      // Should be clamped to oldest sample
      expect(result.startMs).toBe(now - 3600000);
    });

    it('should not modify valid viewport', () => {
      const now = Date.now();
      const viewport: TimeViewport = {
        startMs: now - 600000, // 10 minutes ago
        endMs: now - 300000,   // 5 minutes ago
        followNow: false,
      };
      
      const result = clampToRetained(viewport, 3600);
      
      expect(result.startMs).toBe(viewport.startMs);
      expect(result.endMs).toBe(viewport.endMs);
    });
  });

  describe('advanceToNow', () => {
    it('should update viewport when followNow is enabled', () => {
      const viewport: TimeViewport = {
        startMs: 1000000,
        endMs: 1001000,
        followNow: true,
      };
      
      const result = advanceToNow(viewport);
      
      expect(result.followNow).toBe(true);
      // Span should be preserved but anchored to current time
      expect(result.endMs - result.startMs).toBe(1000);
    });

    it('should not update viewport when followNow is disabled', () => {
      const viewport: TimeViewport = {
        startMs: 1000000,
        endMs: 1001000,
        followNow: false,
      };
      
      const result = advanceToNow(viewport);
      
      expect(result.startMs).toBe(1000000);
      expect(result.endMs).toBe(1001000);
    });
  });

  describe('isInViewport', () => {
    it('should return true for timestamp within viewport', () => {
      const viewport: TimeViewport = {
        startMs: 1000,
        endMs: 2000,
        followNow: false,
      };
      
      expect(isInViewport(1500, viewport)).toBe(true);
    });

    it('should return false for timestamp outside viewport', () => {
      const viewport: TimeViewport = {
        startMs: 1000,
        endMs: 2000,
        followNow: false,
      };
      
      expect(isInViewport(500, viewport)).toBe(false);
      expect(isInViewport(2500, viewport)).toBe(false);
    });

    it('should return true for timestamp at boundaries', () => {
      const viewport: TimeViewport = {
        startMs: 1000,
        endMs: 2000,
        followNow: false,
      };
      
      expect(isInViewport(1000, viewport)).toBe(true);
      expect(isInViewport(2000, viewport)).toBe(true);
    });
  });

  describe('getViewportSpanSeconds', () => {
    it('should return span in seconds', () => {
      const viewport: TimeViewport = {
        startMs: 0,
        endMs: 60000,
        followNow: false,
      };
      
      expect(getViewportSpanSeconds(viewport)).toBe(60);
    });
  });

  describe('formatViewportSpan', () => {
    it('should format seconds', () => {
      const viewport: TimeViewport = {
        startMs: 0,
        endMs: 30000,
        followNow: false,
      };
      
      expect(formatViewportSpan(viewport)).toBe('30s');
    });

    it('should format minutes', () => {
      const viewport: TimeViewport = {
        startMs: 0,
        endMs: 300000,
        followNow: false,
      };
      
      expect(formatViewportSpan(viewport)).toBe('5m');
    });

    it('should format hours', () => {
      const viewport: TimeViewport = {
        startMs: 0,
        endMs: 3600000,
        followNow: false,
      };
      
      expect(formatViewportSpan(viewport)).toBe('1.0h');
    });
  });

  describe('preset definitions', () => {
    it('should have correct ICMP presets', () => {
      expect(ICMP_PRESETS).toHaveLength(4);
      expect(ICMP_PRESETS[0]).toEqual({ label: '1m', seconds: 60 });
      expect(ICMP_PRESETS[1]).toEqual({ label: '5m', seconds: 300 });
      expect(ICMP_PRESETS[2]).toEqual({ label: '15m', seconds: 900 });
      expect(ICMP_PRESETS[3]).toEqual({ label: '60m', seconds: 3600 });
    });

    it('should have correct HTTP presets', () => {
      expect(HTTP_PRESETS).toHaveLength(4);
      expect(HTTP_PRESETS[0]).toEqual({ label: '15m', seconds: 900 });
      expect(HTTP_PRESETS[1]).toEqual({ label: '1h', seconds: 3600 });
      expect(HTTP_PRESETS[2]).toEqual({ label: '2h', seconds: 7200 });
      expect(HTTP_PRESETS[3]).toEqual({ label: '4h', seconds: 14400 });
    });
  });
});
