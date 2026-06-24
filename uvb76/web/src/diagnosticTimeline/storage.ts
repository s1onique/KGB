// Diagnostic Timeline Storage - localStorage persistence for user preferences

/**
 * Stable localStorage key for persisting rows-per-page preference.
 * Using "uvb76" prefix to namespace within the browser origin.
 */
export const DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY = 'uvb76.diagnosticTimeline.rowsPerPage';

/**
 * Known safe page size values.
 * These are the only values accepted for storage persistence.
 */
export const SAFE_PAGE_SIZES = [10, 25, 50, 100] as const;
export type SafePageSize = typeof SAFE_PAGE_SIZES[number];

/**
 * Read rows-per-page from localStorage.
 * 
 * @returns The saved page size, or null if not found/invalid/unavailable
 */
export function loadRowsPerPage(): SafePageSize | null {
  try {
    const saved = localStorage.getItem(DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY);
    if (saved === null) {
      return null;
    }
    
    const parsed = parseInt(saved, 10);
    if (isNaN(parsed)) {
      return null;
    }
    
    // Only accept known safe values
    if (SAFE_PAGE_SIZES.includes(parsed as SafePageSize)) {
      return parsed as SafePageSize;
    }
    
    return null;
  } catch {
    // localStorage may be unavailable in private browsing or restricted contexts
    return null;
  }
}

/**
 * Save rows-per-page to localStorage.
 * 
 * @param pageSize - The page size to persist (must be a valid SafePageSize)
 */
export function saveRowsPerPage(pageSize: SafePageSize): void {
  try {
    localStorage.setItem(DIAGNOSTIC_TIMELINE_ROWS_PER_PAGE_KEY, String(pageSize));
  } catch {
    // Silently fail - storage may be unavailable
    // This is acceptable as the app continues to function without persistence
  }
}
