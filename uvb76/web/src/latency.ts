// Latency rendering module
import { api, type LatencySummary, type LatencySeries, type PercentilePoint } from './api';
import { renderLatencyChart, destroyChart } from './chart';

function formatMs(v: number | undefined): string {
  return v !== undefined && Number.isFinite(v) ? v.toFixed(1) + 'ms' : '—';
}

export interface LatencyRenderer {
  loadAndRender(targetId: string): Promise<void>;
}

function createLatencyRenderer(): LatencyRenderer {
  async function loadAndRender(targetId: string): Promise<void> {
    const metaEl = document.getElementById(`meta-${targetId}`);
    const statsEl = document.getElementById(`stats-${targetId}`);
    const chartEl = document.getElementById(`chart-${targetId}`) as HTMLCanvasElement;
    const warningEl = document.getElementById(`warning-${targetId}`);

    if (!metaEl || !statsEl || !chartEl) return;

    try {
      // Fetch both summary and series data
      const [summary, series] = await Promise.all([
        api.getTargetLatency(targetId),
        api.getLatencySeries(targetId),
      ]);

      if (summary.sample_count === 0) {
        metaEl.innerHTML = '<span>No latency data</span>';
        statsEl.innerHTML = '<div class="percentile-stat"><div class="label">No data</div></div>';
        destroyChart(chartEl);
        warningEl?.classList.add('hidden');
        return;
      }

      // Update metadata
      const retainedSec = series.retained_range_seconds;
      const retainedLabel = Number.isFinite(retainedSec) ? `${Math.round(retainedSec / 60)}m retained` : 'retention unknown';
      metaEl.innerHTML = `
        <span><span class="label">Probe:</span> HTTP status probe</span>
        <span><span class="label">Interval:</span> every ${series.interval_seconds}s</span>
        <span><span class="label">Window:</span> ${series.window_seconds}s trailing</span>
        <span><span class="label">Retained:</span> ${retainedLabel}</span>
        <span><span class="label">Samples:</span> ${summary.sample_count}</span>
      `;

      // Update percentile stats
      statsEl.innerHTML = `
        <div class="percentile-stat">
            <div class="label">Latest p50</div>
            <div class="value p50">${formatMs(summary.p50_latency_ms)}</div>
        </div>
        <div class="percentile-stat">
            <div class="label">Latest p90</div>
            <div class="value p90">${formatMs(summary.p90_latency_ms)}</div>
        </div>
        <div class="percentile-stat">
            <div class="label">Latest p95</div>
            <div class="value p95">${formatMs(summary.p95_latency_ms)}</div>
        </div>
        <div class="percentile-stat">
            <div class="label">Latest p99</div>
            <div class="value p99">${formatMs(summary.p99_latency_ms)}</div>
        </div>
      `;

      // Draw chart
      if (series.points && series.points.length > 0) {
        renderLatencyChart(chartEl, series.points);

        // Show warning if sample count is low
        const latestPoint = series.points[series.points.length - 1];
        if (latestPoint && latestPoint.sample_count < 10) {
          warningEl?.classList.remove('hidden');
        } else {
          warningEl?.classList.add('hidden');
        }
      } else {
        destroyChart(chartEl);
        warningEl?.classList.add('hidden');
      }
    } catch (e) {
      console.error('Failed to load latency for', targetId, e);
      metaEl.innerHTML = '<span>Error loading</span>';
    }
  }

  return { loadAndRender };
}

export function createLatencyRenderers(targetIds: string[]): Map<string, LatencyRenderer> {
  const renderer = createLatencyRenderer();
  const renderers = new Map<string, LatencyRenderer>();

  for (const targetId of targetIds) {
    renderers.set(targetId, {
      loadAndRender: () => renderer.loadAndRender(targetId),
    });
  }

  return renderers;
}

export async function loadLatencyForTarget(targetId: string): Promise<void> {
  const renderer = createLatencyRenderer();
  await renderer.loadAndRender(targetId);
}
