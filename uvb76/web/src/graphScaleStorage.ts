// Graph scale persistence module using localStorage
// Persists selected latency graph scale (1m, 5m, 15m, 60m, 1h, 2h, 4h, Full) per target/kind

export type LatencyScalePreset = "1m" | "5m" | "15m" | "60m" | "1h" | "2h" | "4h" | "full";

const STORAGE_PREFIX = "uvb76.latency.scale.";

// Map preset labels to seconds (for ICMP and HTTP)
const PRESET_TO_SECONDS: Record<Exclude<LatencyScalePreset, "full">, number> = {
  "1m": 60,
  "5m": 300,
  "15m": 900,
  "60m": 3600,
  "1h": 3600,
  "2h": 7200,
  "4h": 14400,
};

const VALID_PRESETS = new Set<LatencyScalePreset>([
  "1m",
  "5m",
  "15m",
  "60m",
  "1h",
  "2h",
  "4h",
  "full",
]);

export function latencyScaleStorageKey(targetId: string, kind: string): string {
  return `${STORAGE_PREFIX}${kind}.${targetId}`;
}

export function readLatencyScalePreset(
  targetId: string,
  kind: string,
): LatencyScalePreset | null {
  try {
    const value = window.localStorage.getItem(latencyScaleStorageKey(targetId, kind));
    if (value !== null && VALID_PRESETS.has(value as LatencyScalePreset)) {
      return value as LatencyScalePreset;
    }
  } catch {
    // localStorage unavailable (private mode, disabled, quota exceeded)
    return null;
  }

  return null;
}

export function writeLatencyScalePreset(
  targetId: string,
  kind: string,
  preset: LatencyScalePreset,
): void {
  if (!VALID_PRESETS.has(preset)) {
    return;
  }

  try {
    window.localStorage.setItem(latencyScaleStorageKey(targetId, kind), preset);
  } catch {
    // Storage is best-effort only. Never break chart controls.
  }
}

export function presetToSeconds(preset: LatencyScalePreset): number {
  if (preset === "full") {
    return -1; // Signal to use full retained range
  }
  return PRESET_TO_SECONDS[preset];
}

// Normalize preset labels - ICMP uses "60m" but HTTP uses "1h" for same duration
export function normalizePresetLabel(preset: LatencyScalePreset, kind: string): string {
  if (preset === "full") return "full";
  
  // For ICMP, 3600 seconds = "60m"; for HTTP, 3600 seconds = "1h"
  if (preset === "60m" && kind === "http") return "1h";
  if (preset === "1h" && kind === "icmp") return "60m";
  
  return preset;
}
