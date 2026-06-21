/**
 * Timestamp parsing and formatting utilities for UVB-76.
 *
 * CRITICAL RULE: Never mix local time rendering with UTC labels.
 * - Either render local time and label it as local
 * - Or render UTC via toISOString() / getUTC* methods
 *
 * This module ensures all API timestamps are parsed as UTC instants
 * and rendered as UTC with explicit "UTC" label.
 */

/**
 * Parse an API timestamp string, rejecting timezone-less formats.
 *
 * Accepts only explicit RFC3339-like instants with T separator:
 * - "2026-06-20T21:09:59Z"
 * - "2026-06-20T21:09:59.123Z"
 * - "2026-06-20T21:09:59+02:00"
 * - "2026-06-20T21:09:59-05:00"
 *
 * Returns null for:
 * - null/undefined input
 * - space-separated formats even with timezone: "2026-06-20 21:09:59Z"
 * - timezone-less formats: "2026-06-20T21:09:59"
 * - invalid date strings
 */
export function parseApiInstant(value: string | null | undefined): Date | null {
  if (!value) return null;

  // Full-shape RFC3339 regex requiring T separator and explicit timezone.
  // This prevents space-separated forms from slipping through even with
  // timezone suffix (e.g., "2026-06-20 21:09:59Z" is rejected).
  const explicitInstantPattern =
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[zZ]|[+-]\d{2}:\d{2})$/;

  if (!explicitInstantPattern.test(value)) {
    return null;
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;

  return date;
}

/**
 * Format a UTC instant as a human-readable string with explicit UTC label.
 *
 * Output format: "2026-06-20 21:09:59 UTC"
 *
 * Uses toISOString() to extract UTC values, never browser-local getters.
 */
export function formatUtcInstant(value: string | null | undefined): string {
  const date = parseApiInstant(value);
  if (!date) return 'invalid timestamp';

  // Use toISOString() to get UTC values, never getHours/getMinutes/etc.
  const isoString = date.toISOString();

  // Convert "2026-06-20T21:09:59.123Z" to "2026-06-20 21:09:59 UTC"
  // Handle both with and without sub-second precision
  return isoString
    .replace('T', ' ')
    .replace(/\.\d{3}Z$/, ' UTC') // .123Z -> UTC
    .replace(/Z$/, ' UTC'); // bare Z -> UTC
}

/**
 * Format a UTC instant for compact display (time only).
 *
 * Output format: "21:09:59 UTC"
 *
 * Uses toISOString() to extract UTC values.
 */
export function formatUtcTime(value: string | null | undefined): string {
  const date = parseApiInstant(value);
  if (!date) return '—';

  // Extract UTC time components from ISO string
  const isoString = date.toISOString();
  // Format: "2026-06-20T21:09:59.123Z" -> extract "21:09:59"
  const match = isoString.match(/T(\d{2}:\d{2}:\d{2})/);
  if (!match) return '—';

  return match[1] + ' UTC';
}

/**
 * Format remaining cooldown as a human-readable duration.
 *
 * If remainingMs is provided, formats it directly.
 * Otherwise computes from nextEligible - now.
 *
 * Output format: "32327 ms" or "32.3 s" for values >= 1000ms
 */
export function formatRemainingCooldown(
  remainingMs: number | null | undefined,
  nextEligible: string | null | undefined = null
): string {
  // Use provided remainingMs if available
  let remaining: number | null = remainingMs ?? null;

  // Otherwise compute from nextEligible if provided
  if (remaining === null && nextEligible) {
    const nextDate = parseApiInstant(nextEligible);
    if (nextDate) {
      remaining = nextDate.getTime() - Date.now();
      if (remaining < 0) remaining = 0;
    }
  }

  if (remaining === null) return '—';

  // Format with adaptive units
  if (remaining >= 1000) {
    return `${(remaining / 1000).toFixed(1)} s`;
  }

  return `${remaining} ms`;
}

/**
 * Validate that a string is a full RFC3339-like instant.
 *
 * Returns true only for complete timestamps with T separator AND explicit timezone.
 * This matches parseApiInstant() behavior.
 */
export function hasExplicitTimezone(value: string | null | undefined): boolean {
  if (!value) return false;
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[zZ]|[+-]\d{2}:\d{2})$/.test(value);
}

/**
 * Convert an offset timestamp to UTC instant for comparison.
 *
 * Input: "2026-06-20T23:09:59+02:00"
 * Output: "2026-06-20T21:09:59Z" (as Date)
 */
export function toUTCInstant(value: string | null | undefined): Date | null {
  const date = parseApiInstant(value);
  return date; // Already normalized by parseApiInstant
}

// ---------------------------------------------------------------------------
// Local timezone formatters (for consistent user-facing display)
// ---------------------------------------------------------------------------

/**
 * Options for local timezone formatters.
 * timeZone defaults to browser/runtime timezone when undefined.
 */
export interface LocalFormatterOptions {
  /** IANA timezone (e.g., "Europe/Helsinki"). Defaults to browser timezone. */
  timeZone?: string;
}

/**
 * Format a UTC instant as a full date-time string in local/browser timezone.
 *
 * Output format: "2026-06-21 11:39:56" (no UTC suffix)
 *
 * Uses Intl.DateTimeFormat with formatToParts for deterministic output.
 * Tests can pass explicit timeZone for deterministic expectations.
 *
 * @param value - RFC3339 timestamp string
 * @param options - Optional timeZone override (for testing)
 * @returns Formatted date-time or "—" for invalid/missing input
 */
export function formatLocalDateTime(
  value: string | null | undefined,
  options: LocalFormatterOptions = {}
): string {
  const date = parseApiInstant(value);
  if (!date) return '—';

  const formatter = new Intl.DateTimeFormat('en-GB', {
    timeZone: options.timeZone, // undefined = browser default
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });

  const parts = formatter.formatToParts(date);
  const year = parts.find(p => p.type === 'year')?.value ?? '????';
  const month = parts.find(p => p.type === 'month')?.value ?? '??';
  const day = parts.find(p => p.type === 'day')?.value ?? '??';
  const hour = parts.find(p => p.type === 'hour')?.value ?? '??';
  const minute = parts.find(p => p.type === 'minute')?.value ?? '??';
  const second = parts.find(p => p.type === 'second')?.value ?? '??';

  return `${year}-${month}-${day} ${hour}:${minute}:${second}`;
}

/**
 * Format a UTC instant as a time-only string in local/browser timezone.
 *
 * Output format: "11:39:56" (no UTC suffix)
 *
 * Uses Intl.DateTimeFormat with formatToParts for deterministic output.
 * Tests can pass explicit timeZone for deterministic expectations.
 *
 * @param value - RFC3339 timestamp string
 * @param options - Optional timeZone override (for testing)
 * @returns Formatted time or "—" for invalid/missing input
 */
export function formatLocalTime(
  value: string | null | undefined,
  options: LocalFormatterOptions = {}
): string {
  const date = parseApiInstant(value);
  if (!date) return '—';

  const formatter = new Intl.DateTimeFormat('en-GB', {
    timeZone: options.timeZone, // undefined = browser default
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });

  const parts = formatter.formatToParts(date);
  const hour = parts.find(p => p.type === 'hour')?.value ?? '??';
  const minute = parts.find(p => p.type === 'minute')?.value ?? '??';
  const second = parts.find(p => p.type === 'second')?.value ?? '??';

  return `${hour}:${minute}:${second}`;
}
