// Chart rendering module using Chart.js
import {
  Chart,
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  Title,
  Tooltip,
  Legend,
  Filler,
} from 'chart.js';
import type { PercentilePoint } from './api';
import type { TimeViewport } from './viewport';
import { isInViewport } from './viewport';

// Register Chart.js components
Chart.register(
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  Title,
  Tooltip,
  Legend,
  Filler
);

interface ChartInstance {
  chart: Chart | null;
  canvas: HTMLCanvasElement;
  fullPoints: PercentilePoint[];  // Store full retained dataset for viewport navigation
}

const charts = new Map<HTMLCanvasElement, ChartInstance>();

// Dark theme colors
const colors = {
  p50: { border: '#3fb950', background: 'rgba(63, 185, 80, 0.1)' },
  p90: { border: '#58a6ff', background: 'rgba(88, 166, 255, 0.1)' },
  p95: { border: '#a371f7', background: 'rgba(163, 113, 247, 0.1)' },
  p99: { border: '#f0883e', background: 'rgba(240, 136, 62, 0.1)' },
  grid: '#30363d',
  text: '#8b949e',
};

// Raw point may use either short (p50_ms) or long (p50_latency_ms) field names
type RawPercentilePoint = PercentilePoint & {
  p50_latency_ms?: number | null;
  p90_latency_ms?: number | null;
  p95_latency_ms?: number | null;
  p99_latency_ms?: number | null;
};

function finiteNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

// Normalize percentile field names - accept both short and long variants
function percentileValue(
  point: RawPercentilePoint,
  shortName: 'p50_ms' | 'p90_ms' | 'p95_ms' | 'p99_ms',
  longName: 'p50_latency_ms' | 'p90_latency_ms' | 'p95_latency_ms' | 'p99_latency_ms'
): number | null {
  return finiteNumber(point[shortName]) ?? finiteNumber(point[longName]);
}

export interface ParsedPoint {
  ts: number;
  sampleCount: number;
  p50: number | null;
  p90: number | null;
  p95: number | null;
  p99: number | null;
}

export function parsePoint(p: RawPercentilePoint): ParsedPoint {
  return {
    ts: new Date(p.ts).getTime(),
    sampleCount: finiteNumber(p.sample_count) ?? 0,
    p50: percentileValue(p, 'p50_ms', 'p50_latency_ms'),
    p90: percentileValue(p, 'p90_ms', 'p90_latency_ms'),
    p95: percentileValue(p, 'p95_ms', 'p95_latency_ms'),
    p99: percentileValue(p, 'p99_ms', 'p99_latency_ms'),
  };
}

// Compute a nice max value for y-axis given max latency
export function niceLatencyMax(max: number): number {
  if (!Number.isFinite(max) || max <= 0) return 100;
  if (max <= 100) return 100;
  if (max <= 500) return 500;
  if (max <= 1000) return 1000;
  if (max <= 2500) return 2500;
  if (max <= 5000) return 5000;
  return Math.ceil(max / 5000) * 5000;
}

function formatMs(v: number): string {
  if (v < 1) return '<1 ms';
  if (v < 1000) return v.toFixed(0) + ' ms';
  return (v / 1000).toFixed(2) + ' s';
}

// Extract finite {x, y} points for a given percentile key, sorted by timestamp
export function pointsFor(parsedPoints: ParsedPoint[], key: keyof Pick<ParsedPoint, 'p50' | 'p90' | 'p95' | 'p99'>): { x: number; y: number }[] {
  return parsedPoints
    .filter((p) => Number.isFinite(p.ts) && p[key] !== null)
    .sort((a, b) => a.ts - b.ts)
    .map((p) => ({ x: p.ts, y: p[key] as number }));
}

// Build percentile datasets for Chart.js
export function buildPercentileDatasets(parsedPoints: ParsedPoint[]) {
  return [
    {
      label: 'p50',
      data: pointsFor(parsedPoints, 'p50'),
      borderColor: colors.p50.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      tension: 0.1,
      spanGaps: true,
    },
    {
      label: 'p90',
      data: pointsFor(parsedPoints, 'p90'),
      borderColor: colors.p90.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      tension: 0.1,
      spanGaps: true,
    },
    {
      label: 'p95',
      data: pointsFor(parsedPoints, 'p95'),
      borderColor: colors.p95.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      tension: 0.1,
      spanGaps: true,
    },
    {
      label: 'p99',
      data: pointsFor(parsedPoints, 'p99'),
      borderColor: colors.p99.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      tension: 0.1,
      spanGaps: true,
    },
  ];
}

// Debug guard - gate behind constant
const DEBUG_CHART = false;

export function renderLatencyChart(canvas: HTMLCanvasElement, points: PercentilePoint[]): void {
  // Destroy existing chart if any
  destroyChart(canvas);

  if (!points || points.length === 0) {
    return;
  }

  // Parse data points
  const parsedPoints = points.map(parsePoint);

  // Build datasets from finite points only
  const datasets = buildPercentileDatasets(parsedPoints);

  // Compute all finite y values to set y-axis scale
  const allY = datasets.flatMap((d) => d.data.map((p) => p.y));
  const maxY = allY.length > 0 ? Math.max(...allY) : 0;

  // Debug logging - only if no finite y-values found
  if (DEBUG_CHART && points.length > 0) {
    console.log('[chart] First raw point:', JSON.stringify(points[0]));
    console.log('[chart] First parsed point:', JSON.stringify(parsedPoints[0]));
    console.log('[chart] All y-values:', allY.slice(0, 10), '...');
    console.log('[chart] Max y:', maxY);
  }

  // Determine if we have any data to show
  if (allY.length === 0) {
    if (DEBUG_CHART) {
      console.warn('[chart] No finite latency series points found');
    }
    return;
  }

  // Determine point radius based on sample count
  const latestPoint = parsedPoints[parsedPoints.length - 1];
  const pointRadius = latestPoint && latestPoint.sampleCount < 10 ? 2 : 0;
  const pointHoverRadius = 4;

  // Add point styling to all datasets
  for (const dataset of datasets) {
    dataset.pointRadius = pointRadius;
    dataset.pointHoverRadius = pointHoverRadius;
  }

  // Create chart - use datasets only, no labels (Chart.js supports {x, y} objects)
  const chart = new Chart(canvas, {
    type: 'line',
    data: { datasets },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      interaction: {
        mode: 'index',
        intersect: false,
      },
      plugins: {
        legend: {
          display: false,
        },
        tooltip: {
          enabled: true,
          backgroundColor: '#161b22',
          titleColor: '#c9d1d9',
          bodyColor: '#c9d1d9',
          borderColor: '#30363d',
          borderWidth: 1,
          padding: 10,
          displayColors: true,
          callbacks: {
            title: (items) => {
              const x = items[0]?.parsed.x;
              return Number.isFinite(x)
                ? new Date(x).toLocaleString()
                : '';
            },
            label: (context) => {
              const value = context.parsed.y;
              return Number.isFinite(value)
                ? `${context.dataset.label}: ${formatMs(value)}`
                : `${context.dataset.label}: —`;
            },
          },
        },
      },
      scales: {
        x: {
          type: 'linear',
          ticks: {
            color: colors.text,
            maxTicksLimit: 8,
            callback: (value) => {
              const date = new Date(value as number);
              return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
            },
          },
          grid: {
            color: colors.grid,
          },
        },
        y: {
          beginAtZero: true,
          suggestedMax: niceLatencyMax(maxY),
          ticks: {
            color: colors.text,
            callback: (value) => formatMs(Number(value)),
          },
          grid: {
            color: colors.grid,
          },
        },
      },
    },
  });

  charts.set(canvas, { chart, canvas });
}

export function destroyChart(canvas: HTMLCanvasElement): void {
  const instance = charts.get(canvas);
  if (instance?.chart) {
    instance.chart.destroy();
    charts.delete(canvas);
  }
}

// Render chart with viewport bounds (x-axis min/max)
export function renderLatencyChartWithViewport(
  canvas: HTMLCanvasElement,
  points: PercentilePoint[],
  viewport: TimeViewport
): void {
  // Destroy existing chart if any
  destroyChart(canvas);

  if (!points || points.length === 0) {
    return;
  }

  // Parse data points
  const parsedPoints = points.map(parsePoint);

  // Filter points to viewport for y-axis scaling
  const visiblePoints = parsedPoints.filter(p => isInViewport(p.ts, viewport));

  // Build datasets from finite points
  const datasets = buildPercentileDatasets(visiblePoints);

  // Compute y-axis scale from VISIBLE points only (not hidden retained data)
  const allY = datasets.flatMap((d) => d.data.map((p) => p.y));
  const maxY = allY.length > 0 ? Math.max(...allY) : 0;

  // Determine if we have any data to show
  if (allY.length === 0) {
    if (DEBUG_CHART) {
      console.warn('[chart] No finite latency series points found in viewport');
    }
    return;
  }

  // Determine point radius based on sample count
  const latestVisiblePoint = visiblePoints[visiblePoints.length - 1];
  const pointRadius = latestVisiblePoint && latestVisiblePoint.sampleCount < 10 ? 2 : 0;
  const pointHoverRadius = 4;

  // Add point styling to all datasets
  for (const dataset of datasets) {
    dataset.pointRadius = pointRadius;
    dataset.pointHoverRadius = pointHoverRadius;
  }

  // Create chart with viewport bounds on x-axis
  const chart = new Chart(canvas, {
    type: 'line',
    data: { datasets },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      interaction: {
        mode: 'index',
        intersect: false,
      },
      plugins: {
        legend: {
          display: false,
        },
        tooltip: {
          enabled: true,
          backgroundColor: '#161b22',
          titleColor: '#c9d1d9',
          bodyColor: '#c9d1d9',
          borderColor: '#30363d',
          borderWidth: 1,
          padding: 10,
          displayColors: true,
          callbacks: {
            title: (items) => {
              const x = items[0]?.parsed.x;
              return Number.isFinite(x)
                ? new Date(x).toLocaleString()
                : '';
            },
            label: (context) => {
              const value = context.parsed.y;
              return Number.isFinite(value)
                ? `${context.dataset.label}: ${formatMs(value)}`
                : `${context.dataset.label}: —`;
            },
          },
        },
      },
      scales: {
        x: {
          type: 'linear',
          // Apply viewport bounds
          min: viewport.startMs,
          max: viewport.endMs,
          ticks: {
            color: colors.text,
            maxTicksLimit: 8,
            callback: (value) => {
              const date = new Date(value as number);
              return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
            },
          },
          grid: {
            color: colors.grid,
          },
        },
        y: {
          beginAtZero: true,
          suggestedMax: niceLatencyMax(maxY),
          ticks: {
            color: colors.text,
            callback: (value) => formatMs(Number(value)),
          },
          grid: {
            color: colors.grid,
          },
        },
      },
    },
  });

  // Store full points for viewport navigation
  charts.set(canvas, { chart, canvas, fullPoints: points });
}

// Update chart viewport bounds and recompute visible data (for navigation controls)
export function updateChartViewport(canvas: HTMLCanvasElement, viewport: TimeViewport): void {
  const instance = charts.get(canvas);
  if (!instance?.chart || !instance.fullPoints) {
    return;
  }

  const chart = instance.chart;
  const xScale = chart.scales['x'];
  const yScale = chart.scales['y'];
  
  // Parse full points and filter to new viewport
  const parsedPoints = instance.fullPoints.map(parsePoint);
  const visiblePoints = parsedPoints.filter(p => isInViewport(p.ts, viewport));
  
  // Rebuild datasets for visible points
  const datasets = buildPercentileDatasets(visiblePoints);
  
  // Compute y-axis scale from VISIBLE points only
  const allY = datasets.flatMap((d) => d.data.map((p) => p.y));
  const maxY = allY.length > 0 ? Math.max(...allY) : 0;
  
  // Determine point radius
  const latestVisiblePoint = visiblePoints[visiblePoints.length - 1];
  const pointRadius = latestVisiblePoint && latestVisiblePoint.sampleCount < 10 ? 2 : 0;
  
  // Update datasets
  chart.data.datasets = datasets;
  for (const dataset of chart.data.datasets) {
    dataset.pointRadius = pointRadius;
    dataset.pointHoverRadius = 4;
  }
  
  // Update x-axis bounds
  if (xScale) {
    xScale.options.min = viewport.startMs;
    xScale.options.max = viewport.endMs;
  }
  
  // Update y-axis scale from visible data
  if (yScale) {
    yScale.options.suggestedMax = niceLatencyMax(maxY);
  }
  
  chart.update('none'); // No animation for instant response
}
