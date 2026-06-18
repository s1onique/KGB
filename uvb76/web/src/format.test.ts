import { describe, expect, it } from 'vitest';
import { formatLatencyMs } from './format';

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
