import { describe, expect, it } from 'vitest';
import { formatLatencyMs, formatStartTime } from './format';

describe('formatLatencyMs', () => {
  it('renders milliseconds with one decimal place', () => {
    expect(formatLatencyMs(49.3)).toBe('49.3 ms');
    expect(formatLatencyMs(425.9)).toBe('425.9 ms');
  });

  it('renders seconds for values >= 1000ms', () => {
    expect(formatLatencyMs(1000)).toBe('1.00 s');
    expect(formatLatencyMs(2971.6)).toBe('2.97 s');
    expect(formatLatencyMs(2798.1)).toBe('2.80 s');
  });

  it('renders dash for non-finite or missing values', () => {
    expect(formatLatencyMs(undefined)).toBe('—');
    expect(formatLatencyMs(null)).toBe('—');
    expect(formatLatencyMs(NaN)).toBe('—');
    expect(formatLatencyMs(Infinity)).toBe('—');
    expect(formatLatencyMs(-Infinity)).toBe('—');
  });

  it('handles boundary values correctly', () => {
    expect(formatLatencyMs(999)).toBe('999.0 ms');
    expect(formatLatencyMs(1000)).toBe('1.00 s');
    expect(formatLatencyMs(0)).toBe('0.0 ms');
    expect(formatLatencyMs(0.5)).toBe('0.5 ms');
  });

  it('handles negative values', () => {
    expect(formatLatencyMs(-1000)).toBe('-1.00 s');
    expect(formatLatencyMs(-49.3)).toBe('-49.3 ms');
  });
});

describe('formatStartTime', () => {
  it('renders RFC3339 timestamp in dd.mm HH:MM:SS format', () => {
    // Use a fixed date to avoid timezone-dependent test failures
    const timestamp = '2026-06-18T08:50:12Z';
    const result = formatStartTime(timestamp);
    // Should produce dd.mm HH:MM:SS format (timezone may vary output)
    expect(result).toMatch(/^\d{2}\.\d{2} \d{2}:\d{2}:\d{2}$/);
  });

  it('renders dash for missing timestamp', () => {
    expect(formatStartTime(null)).toBe('—');
    expect(formatStartTime(undefined)).toBe('—');
    expect(formatStartTime('')).toBe('—');
  });

  it('renders dash for invalid timestamp', () => {
    expect(formatStartTime('not-a-timestamp')).toBe('—');
    expect(formatStartTime('2026-13-45T99:99:99Z')).toBe('—');
  });

  it('handles various valid RFC3339 formats', () => {
    // With timezone offset
    expect(formatStartTime('2026-06-18T08:50:12+03:00')).toMatch(/^\d{2}\.\d{2} \d{2}:\d{2}:\d{2}$/);
    // With milliseconds
    expect(formatStartTime('2026-06-18T08:50:12.123Z')).toMatch(/^\d{2}\.\d{2} \d{2}:\d{2}:\d{2}$/);
  });
});
