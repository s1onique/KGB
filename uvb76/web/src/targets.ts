// Targets rendering module
import { api, type Target, type TargetSnapshot } from './api';

// HTML escape helper for XSS protection - uses textContent pattern to safely escape all HTML
function escapeText(s: string): string {
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

// Whitelist of allowed CSS classes for status to prevent XSS
const statusClasses = new Set(['up', 'down', 'unknown', 'error', 'degraded', 'warning']);

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
          <strong>${escapeText(t.name)}</strong> (${escapeText(t.id)})
          <br><span class="meta">${escapeText(t.base_url)}</span>
          <div class="meta" id="status-${escapeText(t.id)}">Loading...</div>
          <div class="latency-card" id="latency-${escapeText(t.id)}">
              <div class="latency-meta" id="meta-${escapeText(t.id)}">Loading latency...</div>
              <div class="percentile-stats" id="stats-${escapeText(t.id)}"></div>
              <div class="graph-container" id="graph-container-${escapeText(t.id)}">
                  <div class="graph-header">
                      <span class="graph-title">HTTP Status Probe Latency (ms)</span>
                      <div class="graph-legend">
                          <span class="legend-item"><span class="legend-dot p50"></span>p50</span>
                          <span class="legend-item"><span class="legend-dot p90"></span>p90</span>
                          <span class="legend-item"><span class="legend-dot p95"></span>p95</span>
                          <span class="legend-item"><span class="legend-dot p99"></span>p99</span>
                      </div>
                  </div>
                  <div class="latency-chart-wrap">
                      <canvas class="latency-chart" id="chart-${escapeText(t.id)}"></canvas>
                      <div class="latency-empty hidden" id="chart-empty-${escapeText(t.id)}">
                          No finite latency series points yet
                      </div>
                  </div>
                  <div class="graph-subtitle">Trailing 300s windows over retained range</div>
              </div>
              <div class="low-sample-warning hidden" id="warning-${escapeText(t.id)}">
                  Low sample count; tail percentiles are approximate.
              </div>
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
        el.innerHTML = `<span class="status ${safeStatusClass}">${escapeText(safeStatus)}</span> Node: ${safeNodeId}`;
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
