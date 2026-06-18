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

// Latency section HTML template
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
              ${latencySectionHTML(t.id, 'http', 'HTTP Status Probe Latency')}
              ${latencySectionHTML(t.id, 'icmp', 'ICMP Ping Latency')}
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
