// Viewport management for time-series charts
// Handles zoom, pan, and preset window navigation

export interface TimeViewport {
  startMs: number;   // viewport start timestamp in ms
  endMs: number;     // viewport end timestamp in ms
  followNow: boolean; // if true, keep viewport anchored to latest data on refresh
}

// Preset window definitions in seconds
export interface PresetWindow {
  label: string;
  seconds: number;
}

// ICMP presets: 1m, 5m, 15m, 60m (the retained range)
export const ICMP_PRESETS: PresetWindow[] = [
  { label: '1m', seconds: 60 },
  { label: '5m', seconds: 300 },
  { label: '15m', seconds: 900 },
  { label: '60m', seconds: 3600 },
];

// HTTP presets: 15m, 1h, 2h, 4h (the retained range)
export const HTTP_PRESETS: PresetWindow[] = [
  { label: '15m', seconds: 900 },
  { label: '1h', seconds: 3600 },
  { label: '2h', seconds: 7200 },
  { label: '4h', seconds: 14400 },
];

// Pan fraction: how much of the current span to move
const PAN_FRACTION = 0.25;

// Zoom factor: how much to change the span
const ZOOM_FACTOR = 0.5;

// Create a viewport anchored to "now" with the given window size
export function createViewport(windowSeconds: number, retainedRangeSeconds: number): TimeViewport {
  const now = Date.now();
  const windowMs = windowSeconds * 1000;
  const retainedMs = retainedRangeSeconds * 1000;
  
  // Default to showing the window, but cap at retained range
  const effectiveWindow = Math.min(windowMs, retainedMs);
  
  return {
    startMs: now - effectiveWindow,
    endMs: now,
    followNow: true,
  };
}

// Create a viewport showing a specific preset
export function createPresetViewport(preset: PresetWindow, followNow: boolean = true): TimeViewport {
  const now = Date.now();
  const windowMs = preset.seconds * 1000;
  
  return {
    startMs: now - windowMs,
    endMs: now,
    followNow,
  };
}

// Create a viewport showing the full retained range
export function createFullViewport(retainedRangeSeconds: number): TimeViewport {
  const now = Date.now();
  const windowMs = retainedRangeSeconds * 1000;
  
  return {
    startMs: now - windowMs,
    endMs: now,
    followNow: false, // Don't auto-follow when showing full range
  };
}

// Pan the viewport left (into older data)
export function panLeft(viewport: TimeViewport): TimeViewport {
  const span = viewport.endMs - viewport.startMs;
  const panAmount = span * PAN_FRACTION;
  
  return {
    startMs: viewport.startMs - panAmount,
    endMs: viewport.endMs - panAmount,
    followNow: false,
  };
}

// Pan the viewport right (into newer data)
export function panRight(viewport: TimeViewport): TimeViewport {
  const span = viewport.endMs - viewport.startMs;
  const panAmount = span * PAN_FRACTION;
  
  return {
    startMs: viewport.startMs + panAmount,
    endMs: viewport.endMs + panAmount,
    followNow: false,
  };
}

// Zoom in (reduce visible span, centered on viewport center)
export function zoomIn(viewport: TimeViewport): TimeViewport {
  const span = viewport.endMs - viewport.startMs;
  const center = viewport.startMs + span / 2;
  const newSpan = Math.max(span * (1 - ZOOM_FACTOR), 60000); // Minimum 1 minute
  
  return {
    startMs: center - newSpan / 2,
    endMs: center + newSpan / 2,
    followNow: false,
  };
}

// Zoom out (increase visible span, centered on viewport center)
export function zoomOut(viewport: TimeViewport): TimeViewport {
  const span = viewport.endMs - viewport.startMs;
  const center = viewport.startMs + span / 2;
  const newSpan = span * (1 + ZOOM_FACTOR);
  
  return {
    startMs: center - newSpan / 2,
    endMs: center + newSpan / 2,
    followNow: false,
  };
}

// Jump to "now" - re-enable follow mode
export function jumpToNow(viewport: TimeViewport): TimeViewport {
  const span = viewport.endMs - viewport.startMs;
  const now = Date.now();
  
  return {
    startMs: now - span,
    endMs: now,
    followNow: true,
  };
}

// Clamp viewport to retained range
export function clampToRetained(
  viewport: TimeViewport,
  retainedRangeSeconds: number
): TimeViewport {
  const now = Date.now();
  const retainedMs = retainedRangeSeconds * 1000;
  const oldestMs = now - retainedMs;
  
  // If viewport extends before oldest sample, clamp
  let newStartMs = Math.max(viewport.startMs, oldestMs);
  let newEndMs = viewport.endMs;
  
  // If viewport extends past now, clamp
  if (newEndMs > now) {
    newEndMs = now;
  }
  
  // If span exceeds retained range, clamp
  if (newEndMs - newStartMs > retainedMs) {
    newStartMs = now - retainedMs;
    newEndMs = now;
  }
  
  return {
    startMs: newStartMs,
    endMs: newEndMs,
    followNow: viewport.followNow,
  };
}

// Advance viewport to "now" if followNow is enabled
export function advanceToNow(viewport: TimeViewport): TimeViewport {
  if (!viewport.followNow) {
    return viewport;
  }
  
  const span = viewport.endMs - viewport.startMs;
  const now = Date.now();
  
  return {
    startMs: now - span,
    endMs: now,
    followNow: true,
  };
}

// Check if a timestamp is within the viewport
export function isInViewport(ts: number, viewport: TimeViewport): boolean {
  return ts >= viewport.startMs && ts <= viewport.endMs;
}

// Get the span of the viewport in seconds
export function getViewportSpanSeconds(viewport: TimeViewport): number {
  return Math.round((viewport.endMs - viewport.startMs) / 1000);
}

// Format viewport span for display
export function formatViewportSpan(viewport: TimeViewport): string {
  const spanSeconds = getViewportSpanSeconds(viewport);
  
  if (spanSeconds < 60) {
    return `${spanSeconds}s`;
  } else if (spanSeconds < 3600) {
    const minutes = Math.round(spanSeconds / 60);
    return `${minutes}m`;
  } else {
    const hours = (spanSeconds / 3600).toFixed(1);
    return `${hours}h`;
  }
}

// Filter points to only those within the viewport
export function filterPointsToViewport<T extends { ts: string | number }>(
  points: T[],
  viewport: TimeViewport
): T[] {
  return points.filter(p => {
    const ts = typeof p.ts === 'string' ? new Date(p.ts).getTime() : p.ts;
    return isInViewport(ts, viewport);
  });
}

// Default retained ranges per probe kind
const DEFAULT_RETAINED_RANGES: Record<string, number> = {
  icmp: 3600,  // 60 minutes
  http: 14400, // 4 hours
};

// Default window sizes per probe kind (readable defaults)
const DEFAULT_WINDOWS: Record<string, number> = {
  icmp: 300,   // 5 minutes
  http: 900,   // 15 minutes
};

// Viewport state storage - module-level to share between targets.ts and latency.ts
const viewportState = new Map<string, TimeViewport>();

// Get viewport key for a target/kind combination
export function getViewportKey(targetId: string, kind: string): string {
  return `${targetId}:${kind}`;
}

// Get retained range for a probe kind
export function getRetainedRangeForKind(kind: string): number {
  return DEFAULT_RETAINED_RANGES[kind] ?? 3600;
}

// Get default window for a probe kind
export function getDefaultWindowForKind(kind: string): number {
  return DEFAULT_WINDOWS[kind] ?? 300;
}

// Get or create viewport for a target/kind combination
export function getOrCreateViewport(targetId: string, kind: string): TimeViewport {
  const key = getViewportKey(targetId, kind);
  let viewport = viewportState.get(key);
  
  if (!viewport) {
    const retainedRange = getRetainedRangeForKind(kind);
    const defaultWindow = getDefaultWindowForKind(kind);
    viewport = createViewport(defaultWindow, retainedRange);
    viewportState.set(key, viewport);
  }
  
  return viewport;
}

// Set viewport for a target/kind combination
export function setViewport(targetId: string, kind: string, viewport: TimeViewport): void {
  const key = getViewportKey(targetId, kind);
  viewportState.set(key, viewport);
}

// Get current viewport for a target/kind combination
export function getViewport(targetId: string, kind: string): TimeViewport | undefined {
  const key = getViewportKey(targetId, kind);
  return viewportState.get(key);
}

// Create viewport from preset seconds
export function createViewportFromPreset(presetSeconds: number, retainedRangeSeconds: number): TimeViewport {
  const now = Date.now();
  const windowMs = presetSeconds * 1000;
  const retainedMs = retainedRangeSeconds * 1000;
  
  // Use the preset window, but cap at retained range
  const effectiveWindow = Math.min(windowMs, retainedMs);
  
  return {
    startMs: now - effectiveWindow,
    endMs: now,
    followNow: true,
  };
}
