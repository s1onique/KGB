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

function parsePoint(p: PercentilePoint): { ts: number; p50?: number; p90?: number; p95?: number; p99?: number } {
  return {
    ts: new Date(p.ts).getTime(),
    p50: p.p50_ms,
    p90: p.p90_ms,
    p95: p.p95_ms,
    p99: p.p99_ms,
  };
}

export function renderLatencyChart(canvas: HTMLCanvasElement, points: PercentilePoint[]): void {
  // Destroy existing chart if any
  destroyChart(canvas);

  // Parse data points - use epoch ms for x-axis (linear scale)
  const parsedPoints = points.map(parsePoint);
  const labels = parsedPoints.map((p) => p.ts);
  const p50Data = parsedPoints.map((p) => (Number.isFinite(p.p50) ? { x: p.ts, y: p.p50 } : null));
  const p90Data = parsedPoints.map((p) => (Number.isFinite(p.p90) ? { x: p.ts, y: p.p90 } : null));
  const p95Data = parsedPoints.map((p) => (Number.isFinite(p.p95) ? { x: p.ts, y: p.p95 } : null));
  const p99Data = parsedPoints.map((p) => (Number.isFinite(p.p99) ? { x: p.ts, y: p.p99 } : null));

  // Create datasets
  const datasets = [
    {
      label: 'p50',
      data: p50Data,
      borderColor: colors.p50.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      pointRadius: 0,
      tension: 0.1,
    },
    {
      label: 'p90',
      data: p90Data,
      borderColor: colors.p90.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      pointRadius: 0,
      tension: 0.1,
    },
    {
      label: 'p95',
      data: p95Data,
      borderColor: colors.p95.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      pointRadius: 0,
      tension: 0.1,
    },
    {
      label: 'p99',
      data: p99Data,
      borderColor: colors.p99.border,
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      pointRadius: 0,
      tension: 0.1,
    },
  ];

  // Create chart
  const chart = new Chart(canvas, {
    type: 'line',
    data: { labels, datasets },
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
            label: (context) => {
              const value = context.parsed.y;
              if (!Number.isFinite(value)) return `${context.dataset.label}: —`;
              return `${context.dataset.label}: ${value.toFixed(1)}ms`;
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
          ticks: {
            color: colors.text,
            callback: (value) => `${value}ms`,
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
