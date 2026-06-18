/**
 * Adaptive latency value formatter.
 *
 * Renders milliseconds with adaptive unit selection:
 * - Values >= 1000ms display as seconds (e.g., "2.97 s")
 * - Values < 1000ms display as milliseconds (e.g., "49.3 ms")
 * - Non-finite or missing values render as "—"
 */
export function formatLatencyMs(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "—";
  }

  if (Math.abs(value) >= 1000) {
    return `${(value / 1000).toFixed(2)} s`;
  }

  return `${value.toFixed(1)} ms`;
}
