import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  THEME_STORAGE_KEY,
  THEMES,
  THEME_LABELS,
  readStoredTheme,
  applyTheme,
  applyStoredThemeOnLoad,
  setStoredTheme,
  type Theme,
} from './theme';

describe('theme module', () => {
  beforeEach(() => {
    // Mock localStorage
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    });

    // Mock document.documentElement.dataset using defineProperty
    const datasetMock: Record<string, string> = {};
    Object.defineProperty(document.documentElement, 'dataset', {
      value: datasetMock,
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('THEME_LABELS', () => {
    it('has labels for all themes', () => {
      expect(THEME_LABELS[THEMES.dark]).toBe('Dark');
      expect(THEME_LABELS[THEMES['rose-pine']]).toBe('Rose');
    });
  });

  describe('readStoredTheme', () => {
    it('returns dark when storage is empty', () => {
      vi.mocked(localStorage.getItem).mockReturnValue(null);
      expect(readStoredTheme()).toBe(THEMES.dark);
    });

    it('returns dark when storage has no value', () => {
      vi.mocked(localStorage.getItem).mockReturnValue('');
      expect(readStoredTheme()).toBe(THEMES.dark);
    });

    it('returns stored dark theme', () => {
      vi.mocked(localStorage.getItem).mockReturnValue('dark');
      expect(readStoredTheme()).toBe(THEMES.dark);
    });

    it('returns stored rose-pine theme', () => {
      vi.mocked(localStorage.getItem).mockReturnValue('rose-pine');
      expect(readStoredTheme()).toBe('rose-pine');
    });

    it('falls back to dark for corrupt stored value', () => {
      vi.mocked(localStorage.getItem).mockReturnValue('invalid-theme');
      expect(readStoredTheme()).toBe(THEMES.dark);
    });

    it('falls back to dark for unknown theme', () => {
      vi.mocked(localStorage.getItem).mockReturnValue('light');
      expect(readStoredTheme()).toBe(THEMES.dark);
    });

    it('handles localStorage read exception gracefully', () => {
      vi.mocked(localStorage.getItem).mockImplementation(() => {
        throw new Error('localStorage unavailable');
      });
      expect(readStoredTheme()).toBe(THEMES.dark);
    });
  });

  describe('applyTheme', () => {
    it('sets data-theme to dark', () => {
      applyTheme(THEMES.dark);
      expect(document.documentElement.dataset.theme).toBe('dark');
    });

    it('sets data-theme to rose-pine', () => {
      applyTheme(THEMES['rose-pine']);
      expect(document.documentElement.dataset.theme).toBe('rose-pine');
    });
  });

  describe('applyStoredThemeOnLoad', () => {
    it('applies dark when storage is empty', () => {
      vi.mocked(localStorage.getItem).mockReturnValue(null);
      applyStoredThemeOnLoad();
      expect(document.documentElement.dataset.theme).toBe('dark');
    });

    it('applies stored rose-pine theme', () => {
      vi.mocked(localStorage.getItem).mockReturnValue('rose-pine');
      applyStoredThemeOnLoad();
      expect(document.documentElement.dataset.theme).toBe('rose-pine');
    });

    it('applies dark when stored value is corrupt', () => {
      vi.mocked(localStorage.getItem).mockReturnValue('broken');
      applyStoredThemeOnLoad();
      expect(document.documentElement.dataset.theme).toBe('dark');
    });
  });

  describe('setStoredTheme', () => {
    it('applies theme and persists to localStorage', () => {
      setStoredTheme(THEMES['rose-pine']);
      expect(document.documentElement.dataset.theme).toBe('rose-pine');
      expect(localStorage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, 'rose-pine');
    });

    it('applies dark and persists', () => {
      setStoredTheme(THEMES.dark);
      expect(document.documentElement.dataset.theme).toBe('dark');
      expect(localStorage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, 'dark');
    });

    it('still applies theme when localStorage write fails', () => {
      vi.mocked(localStorage.setItem).mockImplementation(() => {
        throw new Error('Storage full');
      });
      setStoredTheme(THEMES['rose-pine']);
      expect(document.documentElement.dataset.theme).toBe('rose-pine');
    });
  });
});
