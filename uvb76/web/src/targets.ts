// Targets rendering module
import { api, type Target, type TargetSnapshot } from './api';
import {
  ICMP_PRESETS,
  HTTP_PRESETS,
  type TimeViewport,
  type PresetWindow,
  createPresetViewport,
  createFullViewport,
  createViewportFromPreset,
  panLeft,
  panRight,
  zoomIn,
  zoomOut,
  jumpToNow,
  clampToRetained,
  getViewportKey,
  getOrCreateViewport,
  setViewport,
  getRetainedRangeForKind,
} from './viewport';
import { updateChartViewport } from './chart';
import {
  readLatencyScalePreset,
  writeLatencyScalePreset,
  presetToSeconds,
  type LatencyScalePreset,
} from './graphScaleStorage';

// HTML escape helper for XSS protection - uses textContent pattern to safely escape all HTML
function escapeText(s: string): string {
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

// HTML attribute escape helper - escapes text for use in HTML attributes
function escapeAttr(s: string): string {
  return escapeText(s).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

// Whitelist of allowed CSS classes for status to prevent XSS
const statusClasses = new Set(['up', 'down', 'unknown', 'error', 'degraded', 'warning']);

// Graph controls HTML for a latency section
function graphControlsHTML(targetId: string, kind: 'http' | 'icmp'): string {
  const presets = kind === 'icmp' ? ICMP_PRESETS : HTTP_PRESETS;
  const presetsHTML = presets.map(p => 
    `<button class="graph-control-btn preset-btn" data-target="${escapeText(targetId)}" data-kind="${kind}" data-preset="${p.seconds}">${escapeText(p.label)}</button>`
  ).join('');
  
  return `
    <div class="graph-controls" id="controls-${kind}-${escapeText(targetId)}">
      <button class="graph-control-btn nav-btn" data-target="${escapeText(targetId)}" data-kind="${kind}" data-action="pan-left" title="Pan left">&#9664;</button>
      <button class="graph-control-btn zoom-btn" data-target="${escapeText(targetId)}" data-kind="${kind}" data-action="zoom-out" title="Zoom out">-</button>
      ${presetsHTML}
      <button class="graph-control-btn zoom-btn" data-target="${escapeText(targetId)}" data-kind="${kind}" data-action="zoom-in" title="Zoom in">+</button>
      <button class="graph-control-btn nav-btn" data-target="${escapeText(targetId)}" data-kind="${kind}" data-action="pan-right" title="Pan right">&#9654;</button>
      <button class="graph-control-btn action-btn" data-target="${escapeText(targetId)}" data-kind="${kind}" data-action="now" title="Jump to now">Now</button>
      <button class="graph-control-btn action-btn" data-target="${escapeText(targetId)}" data-kind="${kind}" data-action="full" title="Show full retained range">Full</button>
    </div>
  `;
}

// Latency section HTML template with graph controls
function latencySectionHTML(targetId: string, kind: 'http' | 'icmp', title: string): string {
  const kindId = kind;
  return `
      <div class="latency-section" id="latency-${kindId}-${escapeText(targetId)}">
          <div class="latency-meta" id="meta-${kindId}-${escapeText(targetId)}">Loading ${title}...</div>
          <div class="percentile-stats" id="stats-${kindId}-${escapeText(targetId)}"></div>
          <div class="graph-container" id="graph-container-${kindId}-${escapeText(targetId)}">
              <div class="graph-header">
                  <span class="graph-title">${escapeText(title)} (ms)</span>
                  <div class="graph-legend">
                      <span class="legend-item"><span class="legend-dot p50"></span>p50</span>
                      <span class="legend-item"><span class="legend-dot p90"></span>p90</span>
                      <span class="legend-item"><span class="legend-dot p95"></span>p95</span>
                      <span class="legend-item"><span class="legend-dot p99"></span>p99</span>
                  </div>
              </div>
              ${graphControlsHTML(targetId, kind)}
              <div class="latency-chart-wrap">
                  <canvas class="latency-chart" id="chart-${kindId}-${escapeText(targetId)}"></canvas>
                  <div class="latency-empty hidden" id="chart-empty-${kindId}-${escapeText(targetId)}">
                      No finite latency series points yet
                  </div>
              </div>
              <div class="graph-subtitle">Trailing windows over retained range</div>
              <div class="sample-count" id="samples-${kindId}-${escapeText(targetId)}"></div>
          </div>
          <div class="low-sample-warning hidden" id="warning-${kindId}-${escapeText(targetId)}">
              Low sample count; tail percentiles are approximate.
          </div>
      </div>
  `;
}

// Handle graph control action
function handleGraphControl(targetId: string, kind: string, action: string, retainedRangeSeconds: number): void {
  const viewport = getOrCreateViewport(targetId, kind);
  const canvas = document.getElementById(`chart-${kind}-${targetId}`) as HTMLCanvasElement;
  if (!canvas) return;

  let newViewport: TimeViewport;

  switch (action) {
    case 'pan-left':
      newViewport = panLeft(viewport);
      break;
    case 'pan-right':
      newViewport = panRight(viewport);
      break;
    case 'zoom-in':
      newViewport = zoomIn(viewport);
      break;
    case 'zoom-out':
      newViewport = zoomOut(viewport);
      break;
    case 'now':
      newViewport = jumpToNow(viewport);
      break;
    case 'full':
      newViewport = createFullViewport(retainedRangeSeconds);
      // Persist 'full' selection
      writeLatencyScalePreset(targetId, kind, 'full');
      break;
    default:
      return;
  }

  // Clamp to retained range and save
  newViewport = clampToRetained(newViewport, retainedRangeSeconds);
  setViewport(targetId, kind, newViewport);

  // Update chart viewport
  updateChartViewport(canvas, newViewport);
}

// Handle preset selection
function handlePresetSelection(targetId: string, kind: string, presetSeconds: number, retainedRangeSeconds: number): void {
  const preset: PresetWindow = { label: '', seconds: presetSeconds };
  const viewport = createPresetViewport(preset, true);
  const clampedViewport = clampToRetained(viewport, retainedRangeSeconds);
  setViewport(targetId, kind, clampedViewport);

  // Persist the scale selection - derive label from seconds
  const storedPreset = derivePresetLabel(presetSeconds, kind);
  if (storedPreset) {
    writeLatencyScalePreset(targetId, kind, storedPreset);
  }

  const canvas = document.getElementById(`chart-${kind}-${targetId}`) as HTMLCanvasElement;
  if (canvas) {
    updateChartViewport(canvas, clampedViewport);
  }
}

// Derive a storage preset label from seconds and kind
function derivePresetLabel(seconds: number, kind: string): LatencyScalePreset | null {
  const mapping: Record<string, LatencyScalePreset> = {
    '60': '1m',
    '300': '5m',
    '900': kind === 'icmp' ? '15m' : '15m',
    '3600': kind === 'icmp' ? '60m' : '1h',
    '7200': '2h',
    '14400': '4h',
  };
  return mapping[String(seconds)] || null;
}

// Apply stored scale preset to initialize viewport correctly
export function applyStoredScalePreset(targetId: string, kind: string, retainedRangeSeconds: number): void {
  const storedPreset = readLatencyScalePreset(targetId, kind);
  if (!storedPreset) return;

  // Full is special - handled separately
  if (storedPreset === 'full') {
    const viewport = createFullViewport(retainedRangeSeconds);
    setViewport(targetId, kind, viewport);
    return;
  }

  // Convert stored preset to seconds
  const seconds = presetToSeconds(storedPreset);
  if (seconds > 0) {
    const viewport = createViewportFromPreset(seconds, retainedRangeSeconds);
    setViewport(targetId, kind, viewport);
  }
}

// Setup graph control event listeners
export function setupGraphControls(): void {
  document.addEventListener('click', (e) => {
    const target = e.target as HTMLElement;
    if (!target.classList.contains('graph-control-btn')) return;

    const targetId = target.dataset.target;
    const kind = target.dataset.kind as 'http' | 'icmp';
    const action = target.dataset.action;
    const preset = target.dataset.preset;

    if (!targetId || !kind) return;

    // Default retained ranges
    const retainedRangeSeconds = kind === 'icmp' ? 3600 : 14400;

    if (preset) {
      handlePresetSelection(targetId, kind, parseInt(preset, 10), retainedRangeSeconds);
    } else if (action) {
      handleGraphControl(targetId, kind, action, retainedRangeSeconds);
    }
  });
}

export interface TargetsRenderer {
  render(targets: Target[]): void;
  updateSnapshot(targetId: string): Promise<void>;
}

function createTargetsRenderer(container: HTMLElement): TargetsRenderer {
  function render(targets: Target[]): void {
    container.innerHTML = targets
      .map(
        (t) => `
      <div class="card target" id="target-${escapeText(t.id)}">
          <div class="target-header-row" data-testid="target-header-${escapeText(t.id)}">
              <strong class="target-name" title="${escapeAttr(t.name)}">${escapeText(t.name)}</strong>
              <span class="target-id" title="${escapeAttr(t.id)}">(${escapeText(t.id)})</span>
              <span class="target-header-sep">·</span>
              <span class="target-url" title="${escapeAttr(t.base_url)}">${escapeText(t.base_url)}</span>
              <span class="target-header-sep">·</span>
              <span class="target-status-meta" id="status-${escapeText(t.id)}">Loading...</span>
          </div>
          <div class="latency-card" id="latency-${escapeText(t.id)}">
              ${latencySectionHTML(t.id, 'http', 'HTTP Status Probe Latency')}
              ${latencySectionHTML(t.id, 'icmp', 'ICMP Ping Latency')}
          </div>
          <div class="spike-diag-card" id="spike-diag-${escapeText(t.id)}">
              <div class="spike-diag-loading">Loading spike diagnostics...</div>
          </div>
      </div>
    `
      )
      .join('');
  }

  async function updateSnapshot(targetId: string): Promise<void> {
    const el = document.getElementById(`status-${targetId}`);
    if (!el) return;

    try {
      const snap: TargetSnapshot = await api.getTargetSnapshot(targetId);
      el.parentElement?.classList.toggle('reachable', snap.reachable);
      el.parentElement?.classList.toggle('unreachable', !snap.reachable);

      if (snap.reachable) {
        // Use whitelist for status class to prevent XSS
        const safeStatus = snap.status || 'unknown';
        const safeStatusClass = statusClasses.has(safeStatus) ? safeStatus : 'unknown';
        const safeNodeId = escapeText(snap.node_id || 'N/A');
        // Peer version: sanitize, cap at 64 chars, show "unknown" fallback
        const rawVersion = snap.peer_version || '';
        const trimmedVersion = rawVersion.trim();
        const safeVersion = trimmedVersion.length > 64 
          ? trimmedVersion.substring(0, 64) + '…' 
          : trimmedVersion;
        const displayVersion = safeVersion || 'unknown';
        el.innerHTML = `<span class="status ${safeStatusClass}">${escapeText(safeStatus)}</span><span class="target-header-sep">·</span>Node: ${safeNodeId}<span class="target-header-sep">·</span><span class="peer-version" data-testid="target-peer-version">Peer: tovarisch ${escapeText(displayVersion)}</span>`;
      } else {
        const safeError = escapeText(snap.error || '');
        el.innerHTML = `<span class="status error">unreachable</span> ${safeError}`;
      }
    } catch {
      el.innerHTML = '<span class="status unknown">error</span>';
    }
  }

  return { render, updateSnapshot };
}

export function initTargets(containerId: string): {
  renderer: TargetsRenderer;
  loadTargets: () => Promise<void>;
  refreshLatency: () => Promise<void>;
} {
  const container = document.getElementById(containerId);
  if (!container) {
    throw new Error(`Container ${containerId} not found`);
  }

  const renderer = createTargetsRenderer(container);
  let targets: Target[] = [];

  async function loadTargets(): Promise<void> {
    try {
      targets = await api.getTargets();
      renderer.render(targets);

      // Load initial data for each target
      for (const t of targets) {
        renderer.updateSnapshot(t.id);
      }
    } catch (e) {
      console.error('Failed to load targets:', e);
    }
  }

  async function refreshLatency(): Promise<void> {
    for (const t of targets) {
      renderer.updateSnapshot(t.id);
    }
  }

  return { renderer, loadTargets, refreshLatency };
}
