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

/**
 * Format a server start timestamp for display.
 *
 * Renders RFC3339 timestamp in local browser timezone as "dd.mm HH:MM:SS".
 * Returns "—" if the timestamp is missing or invalid.
 */
export function formatStartTime(rfc3339Timestamp: string | null | undefined): string {
  if (!rfc3339Timestamp) {
    return "—";
  }

  const date = new Date(rfc3339Timestamp);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }

  // Use Intl.DateTimeFormat for locale-aware formatting
  const formatter = new Intl.DateTimeFormat(undefined, {
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });

  // Format and replace separators to get "dd.mm HH:MM:SS"
  const parts = formatter.formatToParts(date);
  const day = parts.find((p) => p.type === "day")?.value ?? "??";
  const month = parts.find((p) => p.type === "month")?.value ?? "??";
  const hour = parts.find((p) => p.type === "hour")?.value ?? "??";
  const minute = parts.find((p) => p.type === "minute")?.value ?? "??";
  const second = parts.find((p) => p.type === "second")?.value ?? "??";

  return `${day}.${month} ${hour}:${minute}:${second}`;
}

/**
 * Format a spike timestamp for compact display.
 *
 * Renders RFC3339 timestamp in local timezone as "HH:MM:SS".
 * Returns "—" if the timestamp is missing or invalid.
 */
export function formatSpikeTime(rfc3339Timestamp: string | null | undefined): string {
  if (!rfc3339Timestamp) {
    return "—";
  }

  const date = new Date(rfc3339Timestamp);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }

  // Format as HH:MM:SS
  const formatter = new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });

  return formatter.format(date);
}

/**
 * Format a spike severity for display with safe text.
 *
 * Renders severity as uppercase with appropriate styling hint.
 */
export function formatSpikeSeverity(severity: string): string {
  switch (severity.toLowerCase()) {
    case 'critical':
      return 'CRITICAL';
    case 'warning':
      return 'warning';
    default:
      return severity.toUpperCase();
  }
}

/**
 * Format a spike reason for display with safe text.
 *
 * Converts snake_case reason to readable format.
 */
export function formatSpikeReason(reason: string): string {
  // Convert snake_case to readable format
  return reason
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
    .replace('10X', '10x');
}
