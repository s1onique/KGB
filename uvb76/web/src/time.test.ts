// Timestamp utilities tests
import { describe, it, expect } from 'vitest';
import {
  parseApiInstant,
  formatUtcInstant,
  formatUtcTime,
  formatRemainingCooldown,
  hasExplicitTimezone,
  toUTCInstant,
} from './time';

describe('parseApiInstant', () => {
  // Case A: Explicit UTC renders correctly
  it('accepts RFC3339 with Z suffix', () => {
    const result = parseApiInstant('2026-06-20T21:09:59Z');
    expect(result).not.toBeNull();
    expect(result?.toISOString()).toBe('2026-06-20T21:09:59.000Z');
  });

  it('accepts RFC3339Nano with Z suffix', () => {
    const result = parseApiInstant('2026-06-20T21:09:59.123456789Z');
    expect(result).not.toBeNull();
    expect(result?.getUTCMilliseconds()).toBe(123);
  });

  // Case B: Explicit offset preserves instant
  it('accepts RFC3339 with positive offset', () => {
    const result = parseApiInstant('2026-06-20T23:09:59+02:00');
    expect(result).not.toBeNull();
    // Should be 21:09:59 UTC
    expect(result?.toISOString()).toBe('2026-06-20T21:09:59.000Z');
  });

  it('accepts RFC3339 with negative offset', () => {
    const result = parseApiInstant('2026-06-20T02:09:59-05:00');
    expect(result).not.toBeNull();
    // Should be 07:09:59 UTC
    expect(result?.toISOString()).toBe('2026-06-20T07:09:59.000Z');
  });

  // Case C: Timezone-less timestamp rejected
  it('rejects timezone-less timestamp with T separator', () => {
    const result = parseApiInstant('2026-06-20T21:09:59');
    expect(result).toBeNull();
  });

  it('rejects timezone-less timestamp with space separator', () => {
    const result = parseApiInstant('2026-06-20 21:09:59');
    expect(result).toBeNull();
  });

  // Strict T-separator enforcement
  it('rejects space-separated format even with timezone suffix', () => {
    // Even though it has Z, the space separator means it's not RFC3339-like
    expect(parseApiInstant('2026-06-20 21:09:59Z')).toBeNull();
    expect(parseApiInstant('2026-06-20 21:09:59+02:00')).toBeNull();
  });

  it('rejects time-only strings', () => {
    expect(parseApiInstant('21:09:59')).toBeNull();
    expect(parseApiInstant('21:09:59Z')).toBeNull(); // no date part
  });

  // Edge cases
  it('returns null for null/undefined', () => {
    expect(parseApiInstant(null)).toBeNull();
    expect(parseApiInstant(undefined)).toBeNull();
    expect(parseApiInstant('')).toBeNull();
  });

  it('returns null for invalid date strings', () => {
    expect(parseApiInstant('not-a-date')).toBeNull();
    expect(parseApiInstant('2026-13-99T99:99:99Z')).toBeNull();
  });

  // Offset with sub-second precision
  it('handles offset with milliseconds', () => {
    const result = parseApiInstant('2026-06-20T23:09:59.500+02:00');
    expect(result).not.toBeNull();
    expect(result?.toISOString()).toBe('2026-06-20T21:09:59.500Z');
  });
});

describe('formatUtcInstant', () => {
  // Case A: Explicit UTC renders as UTC
  it('formats RFC3339 Z as UTC with space separator', () => {
    const result = formatUtcInstant('2026-06-20T21:09:59Z');
    expect(result).toBe('2026-06-20 21:09:59 UTC');
  });

  it('formats timestamp, strips sub-second precision for readability', () => {
    // formatUtcInstant strips sub-second precision for readability
    const result = formatUtcInstant('2026-06-20T21:09:59.123Z');
    expect(result).toBe('2026-06-20 21:09:59 UTC');
  });

  // Case B: Explicit offset preserved
  it('formats offset as UTC instant', () => {
    const result = formatUtcInstant('2026-06-20T23:09:59+02:00');
    expect(result).toBe('2026-06-20 21:09:59 UTC');
  });

  it('formats negative offset as UTC instant', () => {
    const result = formatUtcInstant('2026-06-20T02:09:59-05:00');
    expect(result).toBe('2026-06-20 07:09:59 UTC');
  });

  // Case C: Timezone-less rejected
  it('returns "invalid timestamp" for timezone-less input', () => {
    expect(formatUtcInstant('2026-06-20T21:09:59')).toBe('invalid timestamp');
    expect(formatUtcInstant('2026-06-20 21:09:59')).toBe('invalid timestamp');
  });

  // Edge cases
  it('returns "invalid timestamp" for null/undefined', () => {
    expect(formatUtcInstant(null)).toBe('invalid timestamp');
    expect(formatUtcInstant(undefined)).toBe('invalid timestamp');
    expect(formatUtcInstant('')).toBe('invalid timestamp');
  });
});

describe('formatUtcTime', () => {
  it('formats UTC time only', () => {
    const result = formatUtcTime('2026-06-20T21:09:59Z');
    expect(result).toBe('21:09:59 UTC');
  });

  it('formats offset as UTC time', () => {
    const result = formatUtcTime('2026-06-20T23:09:59+02:00');
    expect(result).toBe('21:09:59 UTC');
  });

  it('returns em dash for invalid input', () => {
    expect(formatUtcTime(null)).toBe('—');
    expect(formatUtcTime('not-a-date')).toBe('—');
    expect(formatUtcTime('2026-06-20T21:09:59')).toBe('—'); // no timezone
  });
});

describe('formatRemainingCooldown', () => {
  it('formats milliseconds for values < 1000', () => {
    expect(formatRemainingCooldown(500)).toBe('500 ms');
    expect(formatRemainingCooldown(0)).toBe('0 ms');
  });

  it('formats seconds for values >= 1000', () => {
    expect(formatRemainingCooldown(1000)).toBe('1.0 s');
    expect(formatRemainingCooldown(33000)).toBe('33.0 s');
    expect(formatRemainingCooldown(90000)).toBe('90.0 s');
  });

  it('returns em dash for null/undefined', () => {
    expect(formatRemainingCooldown(null)).toBe('—');
    expect(formatRemainingCooldown(undefined)).toBe('—');
  });

  // Screenshot regression test
  it('formats screenshot scenario correctly', () => {
    // remaining_cooldown_ms = 33000 should format as "33.0 s"
    const result = formatRemainingCooldown(33000);
    expect(result).toBe('33.0 s');
  });
});

describe('hasExplicitTimezone', () => {
  it('returns true for Z suffix with T separator', () => {
    expect(hasExplicitTimezone('2026-06-20T21:09:59Z')).toBe(true);
    expect(hasExplicitTimezone('2026-06-20T21:09:59.123z')).toBe(true);
  });

  it('returns true for offset with T separator', () => {
    expect(hasExplicitTimezone('2026-06-20T21:09:59+02:00')).toBe(true);
    expect(hasExplicitTimezone('2026-06-20T21:09:59-05:00')).toBe(true);
  });

  it('returns false for timezone-less (no T separator)', () => {
    expect(hasExplicitTimezone('2026-06-20T21:09:59')).toBe(false);
    expect(hasExplicitTimezone('2026-06-20 21:09:59')).toBe(false);
  });

  it('returns false for space-separated even with timezone suffix', () => {
    // Matches parseApiInstant() behavior: T separator is required
    expect(hasExplicitTimezone('2026-06-20 21:09:59Z')).toBe(false);
    expect(hasExplicitTimezone('2026-06-20 21:09:59+02:00')).toBe(false);
  });

  it('returns false for null/undefined/empty', () => {
    expect(hasExplicitTimezone(null)).toBe(false);
    expect(hasExplicitTimezone(undefined)).toBe(false);
    expect(hasExplicitTimezone('')).toBe(false);
  });
});

describe('toUTCInstant', () => {
  it('converts Z to Date', () => {
    const result = toUTCInstant('2026-06-20T21:09:59Z');
    expect(result).not.toBeNull();
    expect(result?.toISOString()).toBe('2026-06-20T21:09:59.000Z');
  });

  it('converts offset to UTC Date', () => {
    const result = toUTCInstant('2026-06-20T23:09:59+02:00');
    expect(result).not.toBeNull();
    expect(result?.toISOString()).toBe('2026-06-20T21:09:59.000Z');
  });

  it('returns null for timezone-less', () => {
    expect(toUTCInstant('2026-06-20T21:09:59')).toBeNull();
  });
});
