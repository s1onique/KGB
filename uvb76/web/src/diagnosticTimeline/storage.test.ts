// Diagnostic Timeline Storage Tests - Tests for localStorage persistence

import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY,
  SAFE_PAGE_SIZES,
  loadRowsPerPage,
  saveRowsPerPage,
  type SafePageSize,
} from './storage';

describe('storage module', () => {
  // Use a simple object to simulate localStorage
  let store: Record<string, string> = {};
  
  beforeEach(() => {
    store = {};
    // Mock localStorage
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: (key: string) => store[key] ?? null,
        setItem: (key: string, value: string) => { store[key] = value; },
        removeItem: (key: string) => { delete store[key]; },
        clear: () => { store = {}; },
      },
      writable: true,
    });
  });

  describe('DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY', () => {
    it('has correct key value', () => {
      expect(DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY).toBe('uvb76.diagnosticTimeline.rowsPerPage');
    });
  });

  describe('SAFE_PAGE_SIZES', () => {
    it('contains expected page size options', () => {
      expect(SAFE_PAGE_SIZES).toEqual([10, 25, 50, 100]);
    });
  });

  describe('loadRowsPerPage', () => {
    it('returns null when no value is stored', () => {
      expect(loadRowsPerPage()).toBeNull();
    });

    it('returns null when stored value is empty string', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '';
      expect(loadRowsPerPage()).toBeNull();
    });

    it('returns null when stored value is not a number', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = 'abc';
      expect(loadRowsPerPage()).toBeNull();
    });

    it('returns null when stored value is not a valid page size', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '20';
      expect(loadRowsPerPage()).toBeNull();
    });

    it('returns null when stored value is 0', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '0';
      expect(loadRowsPerPage()).toBeNull();
    });

    it('returns null when stored value is negative', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '-10';
      expect(loadRowsPerPage()).toBeNull();
    });

    it('returns 10 when stored value is "10"', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '10';
      expect(loadRowsPerPage()).toBe(10);
    });

    it('returns 25 when stored value is "25"', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '25';
      expect(loadRowsPerPage()).toBe(25);
    });

    it('returns 50 when stored value is "50"', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '50';
      expect(loadRowsPerPage()).toBe(50);
    });

    it('returns 100 when stored value is "100"', () => {
      store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY] = '100';
      expect(loadRowsPerPage()).toBe(100);
    });

    it('returns null when localStorage throws an error', () => {
      // Simulate localStorage being unavailable
      Object.defineProperty(globalThis, 'localStorage', {
        value: {
          getItem: () => { throw new Error('Storage unavailable'); },
        },
        writable: true,
      });

      expect(loadRowsPerPage()).toBeNull();
    });
  });

  describe('saveRowsPerPage', () => {
    it('saves page size as string to localStorage', () => {
      saveRowsPerPage(25);
      expect(store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY]).toBe('25');
    });

    it('saves all valid page sizes correctly', () => {
      for (const size of SAFE_PAGE_SIZES) {
        store = {};
        saveRowsPerPage(size as SafePageSize);
        expect(store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY]).toBe(String(size));
      }
    });

    it('does not throw when localStorage is unavailable', () => {
      Object.defineProperty(globalThis, 'localStorage', {
        value: {
          setItem: () => { throw new Error('Storage unavailable'); },
        },
        writable: true,
      });

      // Should not throw
      expect(() => saveRowsPerPage(25)).not.toThrow();
    });
  });

  describe('integration: save and load roundtrip', () => {
    it('correctly roundtrips all valid page sizes', () => {
      for (const size of SAFE_PAGE_SIZES) {
        store = {};
        saveRowsPerPage(size as SafePageSize);
        expect(store[DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY]).toBe(String(size));
        expect(loadRowsPerPage()).toBe(size);
      }
    });
  });
});
