import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  latencyScaleStorageKey,
  readLatencyScalePreset,
  writeLatencyScalePreset,
  presetToSeconds,
  normalizePresetLabel,
  type LatencyScalePreset,
} from './graphScaleStorage';

describe('latency graph scale storage', () => {
  // Mock localStorage for testing
  let mockStorage: Map<string, string>;
  let mockGetItem: ReturnType<typeof vi.fn>;
  let mockSetItem: ReturnType<typeof vi.fn>;
  let mockRemoveItem: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    mockStorage = new Map();
    
    mockGetItem = vi.fn((key: string) => {
      return mockStorage.get(key) ?? null;
    });
    
    mockSetItem = vi.fn((key: string, value: string) => {
      mockStorage.set(key, value);
    });
    
    mockRemoveItem = vi.fn((key: string) => {
      mockStorage.delete(key);
    });
    
    // Replace window.localStorage with our mock
    Object.defineProperty(window, 'localStorage', {
      value: {
        getItem: mockGetItem,
        setItem: mockSetItem,
        removeItem: mockRemoveItem,
      },
      writable: true,
    });
  });

  afterEach(() => {
    mockStorage.clear();
    vi.restoreAllMocks();
  });

  describe('latencyScaleStorageKey', () => {
    it('should generate correct storage key for ICMP', () => {
      const key = latencyScaleStorageKey('target-1', 'icmp');
      expect(key).toBe('uvb76.latency.scale.icmp.target-1');
    });

    it('should generate correct storage key for HTTP', () => {
      const key = latencyScaleStorageKey('target-2', 'http');
      expect(key).toBe('uvb76.latency.scale.http.target-2');
    });
  });

  describe('readLatencyScalePreset', () => {
    it('should read a valid preset', () => {
      const key = latencyScaleStorageKey('target-1', 'icmp');
      mockStorage.set(key, '5m');
      
      const result = readLatencyScalePreset('target-1', 'icmp');
      expect(result).toBe('5m');
      expect(mockGetItem).toHaveBeenCalledWith(key);
    });

    it('should return null for unknown stored values', () => {
      const key = latencyScaleStorageKey('target-1', 'icmp');
      mockStorage.set(key, 'invalid-preset');
      
      const result = readLatencyScalePreset('target-1', 'icmp');
      expect(result).toBe(null);
    });

    it('should return null when no stored value exists', () => {
      const result = readLatencyScalePreset('nonexistent', 'icmp');
      expect(result).toBe(null);
    });

    it('should return null when localStorage getItem throws', () => {
      mockGetItem.mockImplementationOnce(() => {
        throw new Error('SecurityError');
      });
      
      const result = readLatencyScalePreset('target-1', 'icmp');
      expect(result).toBe(null);
    });

    it('should return null for empty string', () => {
      const key = latencyScaleStorageKey('target-1', 'icmp');
      mockStorage.set(key, '');
      
      const result = readLatencyScalePreset('target-1', 'icmp');
      expect(result).toBe(null);
    });

    it('should accept full preset', () => {
      const key = latencyScaleStorageKey('target-1', 'http');
      mockStorage.set(key, 'full');
      
      const result = readLatencyScalePreset('target-1', 'http');
      expect(result).toBe('full');
    });
  });

  describe('writeLatencyScalePreset', () => {
    it('should store a valid preset', () => {
      writeLatencyScalePreset('target-1', 'icmp', '15m');
      
      const key = latencyScaleStorageKey('target-1', 'icmp');
      expect(mockSetItem).toHaveBeenCalledWith(key, '15m');
    });

    it('should not store invalid preset', () => {
      writeLatencyScalePreset('target-1', 'icmp', 'invalid' as LatencyScalePreset);
      
      expect(mockSetItem).not.toHaveBeenCalled();
    });

    it('should not throw when localStorage setItem fails', () => {
      mockSetItem.mockImplementationOnce(() => {
        throw new Error('QuotaExceededError');
      });
      
      // Should not throw
      expect(() => writeLatencyScalePreset('target-1', 'icmp', '5m')).not.toThrow();
    });

    it('should store full preset', () => {
      writeLatencyScalePreset('target-1', 'http', 'full');
      
      const key = latencyScaleStorageKey('target-1', 'http');
      expect(mockSetItem).toHaveBeenCalledWith(key, 'full');
    });

    it('should store ICMP 60m preset', () => {
      writeLatencyScalePreset('target-1', 'icmp', '60m');
      
      const key = latencyScaleStorageKey('target-1', 'icmp');
      expect(mockSetItem).toHaveBeenCalledWith(key, '60m');
    });

    it('should store HTTP 1h preset', () => {
      writeLatencyScalePreset('target-1', 'http', '1h');
      
      const key = latencyScaleStorageKey('target-1', 'http');
      expect(mockSetItem).toHaveBeenCalledWith(key, '1h');
    });
  });

  describe('presetToSeconds', () => {
    it('should convert 1m to 60', () => {
      expect(presetToSeconds('1m')).toBe(60);
    });

    it('should convert 5m to 300', () => {
      expect(presetToSeconds('5m')).toBe(300);
    });

    it('should convert 15m to 900', () => {
      expect(presetToSeconds('15m')).toBe(900);
    });

    it('should convert 60m to 3600', () => {
      expect(presetToSeconds('60m')).toBe(3600);
    });

    it('should convert 1h to 3600', () => {
      expect(presetToSeconds('1h')).toBe(3600);
    });

    it('should convert 2h to 7200', () => {
      expect(presetToSeconds('2h')).toBe(7200);
    });

    it('should convert 4h to 14400', () => {
      expect(presetToSeconds('4h')).toBe(14400);
    });

    it('should return -1 for full preset', () => {
      expect(presetToSeconds('full')).toBe(-1);
    });
  });

  describe('normalizePresetLabel', () => {
    it('should return full as-is', () => {
      expect(normalizePresetLabel('full', 'icmp')).toBe('full');
      expect(normalizePresetLabel('full', 'http')).toBe('full');
    });

    it('should normalize 60m to 1h for HTTP', () => {
      expect(normalizePresetLabel('60m', 'http')).toBe('1h');
    });

    it('should normalize 1h to 60m for ICMP', () => {
      expect(normalizePresetLabel('1h', 'icmp')).toBe('60m');
    });

    it('should keep 15m unchanged for ICMP', () => {
      expect(normalizePresetLabel('15m', 'icmp')).toBe('15m');
    });

    it('should keep 15m unchanged for HTTP', () => {
      expect(normalizePresetLabel('15m', 'http')).toBe('15m');
    });

    it('should keep 5m unchanged', () => {
      expect(normalizePresetLabel('5m', 'icmp')).toBe('5m');
      expect(normalizePresetLabel('5m', 'http')).toBe('5m');
    });
  });

  describe('per-chart persistence', () => {
    it('should allow different scales for different targets', () => {
      writeLatencyScalePreset('target-1', 'icmp', '5m');
      writeLatencyScalePreset('target-2', 'icmp', '60m');
      
      expect(readLatencyScalePreset('target-1', 'icmp')).toBe('5m');
      expect(readLatencyScalePreset('target-2', 'icmp')).toBe('60m');
    });

    it('should allow different scales for different probe kinds', () => {
      writeLatencyScalePreset('target-1', 'http', '1h');
      writeLatencyScalePreset('target-1', 'icmp', '5m');
      
      expect(readLatencyScalePreset('target-1', 'http')).toBe('1h');
      expect(readLatencyScalePreset('target-1', 'icmp')).toBe('5m');
    });

    it('should use different storage keys for different probe kinds', () => {
      const httpKey = latencyScaleStorageKey('target-1', 'http');
      const icmpKey = latencyScaleStorageKey('target-1', 'icmp');
      
      expect(httpKey).not.toBe(icmpKey);
      expect(httpKey).toContain('http');
      expect(icmpKey).toContain('icmp');
    });
  });
});
