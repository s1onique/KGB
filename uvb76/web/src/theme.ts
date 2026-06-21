// Theme management module for UVB-76
// Supports dark (default) and rose-pine themes with localStorage persistence

export const THEME_STORAGE_KEY = 'uvb76-theme';

export const THEMES = {
  dark: 'dark',
  'rose-pine': 'rose-pine',
} as const;

export type Theme = (typeof THEMES)[keyof typeof THEMES];

export const THEME_LABELS: Record<Theme, string> = {
  [THEMES.dark]: 'Dark',
  [THEMES['rose-pine']]: 'Rose',
};

const VALID_THEMES = new Set(Object.values(THEMES));

/**
 * Safely read stored theme from localStorage.
 * Falls back to 'dark' for invalid or missing values.
 */
export function readStoredTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored && VALID_THEMES.has(stored as Theme)) {
      return stored as Theme;
    }
  } catch {
    // localStorage unavailable - fall through to default
  }
  return THEMES.dark;
}

/**
 * Apply theme by setting document.documentElement.dataset.theme
 */
export function applyTheme(theme: Theme): void {
  try {
    document.documentElement.dataset.theme = theme;
  } catch {
    // Setting dataset may fail in SSR or non-browser environments
  }
}

/**
 * Apply stored theme immediately on page load (before UI renders).
 * Call this at the earliest entrypoint or via inline script in HTML.
 */
export function applyStoredThemeOnLoad(): void {
  const theme = readStoredTheme();
  applyTheme(theme);
}

/**
 * Persist theme to localStorage and apply it.
 * localStorage failures are silent (theme still applies to DOM).
 */
export function setStoredTheme(theme: Theme): void {
  applyTheme(theme);
  try {
    localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // localStorage write failed - theme still applied via applyTheme
  }
}
