// Latency rendering module for HTTP and ICMP latency graphs
import { api, type LatencySummary, type LatencySeries, type PercentilePoint, type TargetLatencyResponse, type SpikeResponse } from './api';
import { renderLatencyChart, destroyChart, renderLatencyChartWithViewport } from './chart';
import { formatLatencyMs } from './format';
import { getOrCreateViewport, getViewport, clampToRetained } from './viewport';
import { applyStoredScalePreset } from './targets';

// Presentation-only label mapping (does not affect API/enum names)
function probeKindToLabel(kind: 'http' | 'icmp'): string {
  switch (kind) {
    case 'http':
      return 'HTTP status';
    case 'icmp':
      return 'ICMP ping';
  }
}

// Check if series has any finite percentile values
function hasFinitePercentiles(points: PercentilePoint[]): boolean {
  return points.some((p) => {
    const v = p as Record<string, unknown>;
    const vals = [
      v.p50_ms, v.p90_ms, v.p95_ms, v.p99_ms,
      v.p50_latency_ms, v.p90_latency_ms, v.p95_latency_ms, v.p99_latency_ms,
    ];
    return vals.some((x) => typeof x === 'number' && Number.isFinite(x as number));
  });
}

// Render stats for a latency section
function renderStats(statsEl: HTMLElement, summary: LatencySummary | undefined): void {
  if (!summary || summary.sample_count === 0) {
    statsEl.innerHTML = '<div class="percentile-stat"><div class="label">No data</div></div>';
    return;
  }

  statsEl.innerHTML = `
    <div class="percentile-stat">
        <div class="label">Latest p50</div>
        <div class="value p50">${formatLatencyMs(summary.p50_latency_ms)}</div>
    </div>
    <div class="percentile-stat">
        <div class="label">Latest p90</div>
        <div class="value p90">${formatLatencyMs(summary.p90_latency_ms)}</div>
    </div>
    <div class="percentile-stat">
        <div class="label">Latest p95</div>
        <div class="value p95">${formatLatencyMs(summary.p95_latency_ms)}</div>
    </div>
    <div class="percentile-stat">
        <div class="label">Latest p99</div>
        <div class="value p99">${formatLatencyMs(summary.p99_latency_ms)}</div>
    </div>
  `;
}

// Format retained range for display
function formatRetainedRange(seconds: number): string {
  if (seconds >= 3600) {
    const hours = seconds / 3600;
    return hours === Math.floor(hours) ? `${hours}h` : `${hours.toFixed(1)}h`;
  } else {
    return `${Math.round(seconds / 60)}m`;
  }
}

// Render metadata for a latency section
function renderMeta(metaEl: HTMLElement, series: LatencySeries | undefined, probeLabel: string): void {
  if (!series) {
    metaEl.innerHTML = '<span>No data</span>';
    return;
  }

  const retainedSec = series.retained_range_seconds;
  const retainedLabel = Number.isFinite(retainedSec) ? `${formatRetainedRange(retainedSec)} retained` : 'retention unknown';
  metaEl.innerHTML = `
    <span><span class="label">Probe:</span> ${probeLabel}</span>
    <span><span class="label">Interval:</span> every ${series.interval_seconds}s</span>
    <span><span class="label">Window:</span> ${series.window_seconds}s trailing</span>
    <span><span class="label">Retained:</span> ${retainedLabel}</span>
  `;
}

// Render a single latency section (HTTP or ICMP)
async function renderLatencySection(
  targetId: string,
  kind: 'http' | 'icmp',
  metaEl: HTMLElement,
  statsEl: HTMLElement,
  chartEl: HTMLCanvasElement,
  emptyEl: HTMLElement,
  warningEl: HTMLElement,
  samplesEl: HTMLElement,
  probeLabel: string
): Promise<void> {
  try {
    // Fetch both summary and series data
    // ICMP: 3600s range (1 hour), 5s step, 60s window to match retained capacity
    // HTTP: 14400s range (4 hours), 60s step, 300s window to match retained capacity
    const [latency, series] = await Promise.all([
      api.getTargetLatency(targetId),
      kind === 'http'
        ? api.getHTTPLatencySeries(targetId, 14400, 60, 300)
        : api.getICMPLatencySeries(targetId, 3600, 5, 60),
    ]);

    const summary = kind === 'http' ? latency.http : latency.icmp;
    const sectionSeries = series.probe_kind === kind ? series : undefined;

    if (!summary || summary.sample_count === 0) {
      metaEl.innerHTML = `<span>${probeLabel}: No latency data</span>`;
      statsEl.innerHTML = '<div class="percentile-stat"><div class="label">No data</div></div>';
      destroyChart(chartEl);
      warningEl?.classList.add('hidden');
      samplesEl.textContent = '';
      return;
    }

    // Update metadata
    renderMeta(metaEl, sectionSeries, probeLabel);

    // Update sample count display
    // Concepts:
    // - buffered: samples in the backend ring buffer
    // - points: chart points returned (aggregated time windows)
    // - cap: maximum ring buffer capacity
    const bufferCount = sectionSeries.retained_sample_count;
    const capacity = sectionSeries.retained_sample_capacity;
    const visibleCount = sectionSeries.returned_point_count;
    const queryRange = sectionSeries.query_range_seconds;
    const effectiveRange = sectionSeries.range_seconds;
    
    if (sectionSeries) {
      
      if (capacity > 0) {
        // Show query range info if it differs from effective range (clamped)
        if (queryRange !== effectiveRange && queryRange > 0) {
          samplesEl.textContent = `${bufferCount} buffered / ${visibleCount} points / ${capacity} cap (${effectiveRange}s of ${queryRange}s)`;
        } else {
          samplesEl.textContent = `${bufferCount} buffered / ${visibleCount} points / ${capacity} cap`;
        }
      } else {
        samplesEl.textContent = `${bufferCount} buffered / ${visibleCount} points`;
      }
    } else {
      samplesEl.textContent = `${summary.sample_count} samples`;
    }

    // Update percentile stats
    renderStats(statsEl, summary);

    // Draw chart using viewport-aware renderer
    if (sectionSeries?.points && sectionSeries.points.length > 0) {
      // Check if we have any finite percentile values
      if (hasFinitePercentiles(sectionSeries.points)) {
        // Show canvas, hide empty overlay
        emptyEl?.classList.add('hidden');
        chartEl.classList.remove('hidden');

        // Get retained range for this probe kind
        const retainedRange = sectionSeries.retained_range_seconds || 
          (kind === 'icmp' ? 3600 : 14400);
        
        // Apply stored scale preset only if no viewport exists yet (first render)
        // This prevents overwriting user's pan/zoom on auto-refresh
        if (!getViewport(targetId, kind)) {
          applyStoredScalePreset(targetId, kind, retainedRange);
        }
        
        // Get or create viewport for this target/kind
        const viewport = getOrCreateViewport(targetId, kind);
        
        // Clamp viewport to retained range
        const clampedViewport = clampToRetained(viewport, retainedRange);
        
        // Render chart with viewport bounds
        renderLatencyChartWithViewport(chartEl, sectionSeries.points, clampedViewport);

        // Show warning if sample count is low
        const latestPoint = sectionSeries.points[sectionSeries.points.length - 1];
        if (latestPoint && latestPoint.sample_count < 10) {
          warningEl?.classList.remove('hidden');
        } else {
          warningEl?.classList.add('hidden');
        }
      } else {
        // Summary has data but series has no finite percentile points yet
        // Preserve canvas in DOM - use overlay approach for recovery
        destroyChart(chartEl);
        chartEl.classList.add('hidden');
        emptyEl?.classList.remove('hidden');
        warningEl?.classList.add('hidden');
      }
    } else {
      destroyChart(chartEl);
      emptyEl?.classList.add('hidden');
      warningEl?.classList.add('hidden');
    }
  } catch (e) {
    console.error(`Failed to load ${kind} latency for`, targetId, e);
    metaEl.innerHTML = `<span>Error loading ${kind}</span>`;
  }
}

export interface LatencyRenderer {
  loadAndRender(targetId: string): Promise<void>;
}

function createLatencyRenderer(): LatencyRenderer {
  async function loadAndRender(targetId: string): Promise<void> {
    // HTTP section elements
    const httpMetaEl = document.getElementById(`meta-http-${targetId}`);
    const httpStatsEl = document.getElementById(`stats-http-${targetId}`);
    const httpChartEl = document.getElementById(`chart-http-${targetId}`) as HTMLCanvasElement;
    const httpEmptyEl = document.getElementById(`chart-empty-http-${targetId}`);
    const httpWarningEl = document.getElementById(`warning-http-${targetId}`);
    const httpSamplesEl = document.getElementById(`samples-http-${targetId}`);

    // ICMP section elements
    const icmpMetaEl = document.getElementById(`meta-icmp-${targetId}`);
    const icmpStatsEl = document.getElementById(`stats-icmp-${targetId}`);
    const icmpChartEl = document.getElementById(`chart-icmp-${targetId}`) as HTMLCanvasElement;
    const icmpEmptyEl = document.getElementById(`chart-empty-icmp-${targetId}`);
    const icmpWarningEl = document.getElementById(`warning-icmp-${targetId}`);
    const icmpSamplesEl = document.getElementById(`samples-icmp-${targetId}`);

    if (!httpMetaEl || !httpStatsEl || !httpChartEl) return;

    // Render both sections
    await Promise.all([
      renderLatencySection(
        targetId, 'http',
        httpMetaEl, httpStatsEl, httpChartEl, httpEmptyEl!, httpWarningEl!, httpSamplesEl!,
        probeKindToLabel('http')
      ),
      renderLatencySection(
        targetId, 'icmp',
        icmpMetaEl!, icmpStatsEl!, icmpChartEl, icmpEmptyEl!, icmpWarningEl!, icmpSamplesEl!,
        probeKindToLabel('icmp')
      ),
    ]);
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
