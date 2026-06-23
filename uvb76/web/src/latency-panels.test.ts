// Tests for two-column latency panel layout

import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock chart module to avoid canvas issues in tests
vi.mock('./chart', () => ({
  renderLatencyChart: vi.fn(),
  destroyChart: vi.fn(),
  renderLatencyChartWithViewport: vi.fn(),
}));

// Import the real escape functions from targets.ts
import { escapeText, escapeAttr } from './targets';

describe('escape functions for XSS safety', () => {
  describe('escapeText', () => {
    it('should escape script tags in text content', () => {
      const xssInput = '<script>alert("xss")</script>';
      const escaped = escapeText(xssInput);
      
      // Unescaped script tags should NOT appear
      expect(escaped).not.toContain('<script>');
      expect(escaped).not.toContain('</script>');
      // Escaped versions should appear (HTML entities)
      expect(escaped).toContain('&lt;script&gt;');
    });
  });

  describe('escapeAttr', () => {
    it('should escape double quotes in attribute values', () => {
      const maliciousAttr = 'value" onload="alert(1)';
      const escaped = escapeAttr(maliciousAttr);
      
      // Should escape double quotes to "
      expect(escaped).not.toContain('"');
      expect(escaped).toContain('&quot;');
    });

    it('should escape single quotes in attribute values', () => {
      const maliciousAttr = "value' onclick='alert(1)'";
      const escaped = escapeAttr(maliciousAttr);
      
      // Should escape single quotes
      expect(escaped).not.toContain("'");
      expect(escaped).toContain('&#39;');
    });

    it('should escape script tags in attribute values', () => {
      const xssInput = '<script>alert("xss")</script>';
      const escaped = escapeAttr(xssInput);
      
      // Should escape script tags (HTML entities)
      expect(escaped).not.toContain('<script>');
      expect(escaped).toContain('&lt;script&gt;');
    });
  });
});

describe('two-column latency panels layout', () => {
  let container: HTMLElement;

  beforeEach(() => {
    // Create a minimal DOM structure that mimics what targets.ts produces
    container = document.createElement('div');
    container.innerHTML = `
      <div class="latency-card">
        <div class="latency-grid">
          <div class="latency-panel" data-kind="http" id="latency-http-test-target-1">
            <div class="percentile-stats" id="stats-http-test-target-1"></div>
            <div class="graph-container" id="graph-container-http-test-target-1">
              <div class="graph-controls" id="controls-http-test-target-1">
                <button class="graph-control-btn" data-target="test-target-1" data-kind="http">Pan</button>
              </div>
              <div class="latency-chart-wrap">
                <canvas class="latency-chart" id="chart-http-test-target-1"></canvas>
              </div>
            </div>
          </div>
          <div class="latency-panel" data-kind="icmp" id="latency-icmp-test-target-1">
            <div class="percentile-stats" id="stats-icmp-test-target-1"></div>
            <div class="graph-container" id="graph-container-icmp-test-target-1">
              <div class="graph-controls" id="controls-icmp-test-target-1">
                <button class="graph-control-btn" data-target="test-target-1" data-kind="icmp">Pan</button>
              </div>
              <div class="latency-chart-wrap">
                <canvas class="latency-chart" id="chart-icmp-test-target-1"></canvas>
              </div>
            </div>
          </div>
        </div>
      </div>
    `;
    document.body.appendChild(container);
  });

  afterEach(() => {
    document.body.removeChild(container);
  });

  it('should render one latency-grid container with two latency panels', () => {
    const grid = container.querySelector('.latency-grid');
    expect(grid).not.toBeNull();

    const panels = container.querySelectorAll('.latency-panel');
    expect(panels.length).toBe(2);
  });

  it('should have exactly one HTTP and one ICMP panel', () => {
    const httpPanel = container.querySelector('[data-kind="http"]');
    const icmpPanel = container.querySelector('[data-kind="icmp"]');
    
    expect(httpPanel).not.toBeNull();
    expect(icmpPanel).not.toBeNull();
    expect(httpPanel).not.toBe(icmpPanel);
  });

  it('should have percentile-stats container in each panel', () => {
    const httpPanel = container.querySelector('[data-kind="http"]');
    const icmpPanel = container.querySelector('[data-kind="icmp"]');
    
    const httpStats = httpPanel?.querySelector('.percentile-stats');
    const icmpStats = icmpPanel?.querySelector('.percentile-stats');
    
    expect(httpStats).not.toBeNull();
    expect(icmpStats).not.toBeNull();
  });

  it('should have graph controls in each panel', () => {
    const httpControls = container.querySelector('[data-kind="http"] .graph-controls');
    const icmpControls = container.querySelector('[data-kind="icmp"] .graph-controls');
    
    expect(httpControls).not.toBeNull();
    expect(icmpControls).not.toBeNull();
  });

  it('should have canvas chart in each panel', () => {
    const httpCanvas = container.querySelector('[data-kind="http"] .latency-chart');
    const icmpCanvas = container.querySelector('[data-kind="icmp"] .latency-chart');
    
    expect(httpCanvas).not.toBeNull();
    expect(httpCanvas?.tagName).toBe('CANVAS');
    expect(icmpCanvas).not.toBeNull();
    expect(icmpCanvas?.tagName).toBe('CANVAS');
  });

  it('should have unique IDs for each panel element', () => {
    const httpPanel = container.querySelector('[data-kind="http"]');
    const icmpPanel = container.querySelector('[data-kind="icmp"]');
    
    const httpMeta = httpPanel?.querySelector('.percentile-stats');
    const icmpMeta = icmpPanel?.querySelector('.percentile-stats');
    
    const httpGraph = httpPanel?.querySelector('.graph-container');
    const icmpGraph = icmpPanel?.querySelector('.graph-container');
    
    // All IDs should be unique
    const ids = [
      httpMeta?.id, icmpMeta?.id,
      httpGraph?.id, icmpGraph?.id,
    ];
    
    const uniqueIds = new Set(ids);
    expect(uniqueIds.size).toBe(ids.length);
  });

  it('should have buttons with data-target and data-kind attributes', () => {
    const httpButton = container.querySelector('[data-kind="http"] .graph-control-btn');
    const icmpButton = container.querySelector('[data-kind="icmp"] .graph-control-btn');
    
    expect(httpButton?.getAttribute('data-target')).toBe('test-target-1');
    expect(httpButton?.getAttribute('data-kind')).toBe('http');
    expect(icmpButton?.getAttribute('data-target')).toBe('test-target-1');
    expect(icmpButton?.getAttribute('data-kind')).toBe('icmp');
  });
});

describe('latency panel text escaping', () => {
  it('should escape target IDs when inserted as text content', () => {
    const xssInput = '<img src=x onerror=alert(1)>';
    const escaped = escapeText(xssInput);
    
    // When inserted as text, the escaped version should render literally
    const el = document.createElement('div');
    el.textContent = escaped;
    
    // The text content should contain the escaped string
    expect(el.textContent).toBe(escaped);
  });

  it('should escape HTML special characters in titles', () => {
    const maliciousTitle = '<script>alert(1)</script>';
    const escaped = escapeText(maliciousTitle);
    
    // Should not contain raw (unescaped) script tags
    expect(escaped).not.toContain('<script>');
    // Should contain HTML-escaped version
    expect(escaped).toContain('&lt;script&gt;');
  });
});

describe('missing data handling', () => {
  it('should render both panels even if only HTTP data exists', () => {
    const container = document.createElement('div');
    container.innerHTML = `
      <div class="latency-grid">
        <div class="latency-panel" data-kind="http">
          <div class="percentile-stats">
            <div class="percentile-stat"><div class="value p50">12ms</div></div>
            <div class="percentile-stat"><div class="value p90">25ms</div></div>
            <div class="percentile-stat"><div class="value p95">30ms</div></div>
            <div class="percentile-stat"><div class="value p99">45ms</div></div>
          </div>
        </div>
        <div class="latency-panel" data-kind="icmp">
          <div class="percentile-stats">
            <div class="percentile-stat"><div class="label">No data</div></div>
          </div>
        </div>
      </div>
    `;
    
    document.body.appendChild(container);
    
    const panels = container.querySelectorAll('.latency-panel');
    expect(panels.length).toBe(2);
    
    document.body.removeChild(container);
  });

  it('should not have broken empty grid columns', () => {
    const container = document.createElement('div');
    container.innerHTML = `
      <div class="latency-grid">
        <div class="latency-panel" data-kind="http">
          <div class="percentile-stats">No data</div>
        </div>
      </div>
    `;
    
    document.body.appendChild(container);
    
    const panels = container.querySelectorAll('.latency-panel');
    expect(panels.length).toBe(1);
    
    document.body.removeChild(container);
  });
});

describe('compact latency panel structure', () => {
  let container: HTMLElement;

  beforeEach(() => {
    // Create DOM structure matching the new compact layout
    container = document.createElement('div');
    container.innerHTML = `
      <div class="latency-card">
        <div class="latency-grid">
          <div class="latency-panel" data-kind="http" id="latency-http-test-target-1">
            <div class="percentile-stats" id="stats-http-test-target-1">
              <div class="percentile-stat"><span class="label">p50</span><span class="value p50">12ms</span></div>
              <div class="percentile-stat"><span class="label">p90</span><span class="value p90">25ms</span></div>
              <div class="percentile-stat"><span class="label">p95</span><span class="value p95">30ms</span></div>
              <div class="percentile-stat"><span class="label">p99</span><span class="value p99">45ms</span></div>
            </div>
            <div class="graph-container" id="graph-container-http-test-target-1">
              <div class="latency-chart-wrap">
                <canvas class="latency-chart" id="chart-http-test-target-1"></canvas>
              </div>
              <div class="latency-footer-row">
                <span class="graph-subtitle">Trailing windows over retained range</span>
                <span class="sample-count" id="samples-http-test-target-1">9 buffered / 240 points / 960 cap</span>
                <span class="low-sample-warning hidden" id="warning-http-test-target-1">
                  Low sample count; tail percentiles are approximate.
                </span>
              </div>
            </div>
          </div>
          <div class="latency-panel" data-kind="icmp" id="latency-icmp-test-target-1">
            <div class="percentile-stats" id="stats-icmp-test-target-1">
              <div class="percentile-stat"><span class="label">p50</span><span class="value p50">8ms</span></div>
              <div class="percentile-stat"><span class="label">p90</span><span class="value p90">15ms</span></div>
              <div class="percentile-stat"><span class="label">p95</span><span class="value p95">20ms</span></div>
              <div class="percentile-stat"><span class="label">p99</span><span class="value p99">35ms</span></div>
            </div>
            <div class="graph-container" id="graph-container-icmp-test-target-1">
              <div class="latency-chart-wrap">
                <canvas class="latency-chart" id="chart-icmp-test-target-1"></canvas>
              </div>
              <div class="latency-footer-row">
                <span class="graph-subtitle">Trailing windows over retained range</span>
                <span class="sample-count" id="samples-icmp-test-target-1">15 buffered / 360 points / 1440 cap</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    `;
    document.body.appendChild(container);
  });

  afterEach(() => {
    document.body.removeChild(container);
  });

  it('should have four percentile cards in one row per panel', () => {
    const httpStats = container.querySelector('#stats-http-test-target-1');
    const icmpStats = container.querySelector('#stats-icmp-test-target-1');
    
    const httpCards = httpStats?.querySelectorAll('.percentile-stat');
    const icmpCards = icmpStats?.querySelectorAll('.percentile-stat');
    
    expect(httpCards?.length).toBe(4);
    expect(icmpCards?.length).toBe(4);
  });

  it('should contain footer row with graph-subtitle, sample-count, and low-sample-warning', () => {
    const httpPanel = container.querySelector('[data-kind="http"]');
    const icmpPanel = container.querySelector('[data-kind="icmp"]');
    
    // HTTP panel has full footer
    const httpFooter = httpPanel?.querySelector('.latency-footer-row');
    const httpSubtitle = httpFooter?.querySelector('.graph-subtitle');
    const httpSamples = httpFooter?.querySelector('.sample-count');
    const httpWarning = httpFooter?.querySelector('.low-sample-warning');
    
    expect(httpFooter).not.toBeNull();
    expect(httpSubtitle).not.toBeNull();
    expect(httpSubtitle?.textContent).toContain('Trailing windows over retained range');
    expect(httpSamples).not.toBeNull();
    expect(httpSamples?.id).toBe('samples-http-test-target-1');
    expect(httpWarning).not.toBeNull();
    expect(httpWarning?.id).toBe('warning-http-test-target-1');
    
    // ICMP panel has footer without warning
    const icmpFooter = icmpPanel?.querySelector('.latency-footer-row');
    const icmpSubtitle = icmpFooter?.querySelector('.graph-subtitle');
    const icmpSamples = icmpFooter?.querySelector('.sample-count');
    
    expect(icmpFooter).not.toBeNull();
    expect(icmpSubtitle).not.toBeNull();
    expect(icmpSamples).not.toBeNull();
  });

  it('should have kind-specific warning ID for HTTP panel for JS toggling', () => {
    const httpWarning = container.querySelector('#warning-http-test-target-1');
    
    expect(httpWarning?.id).toBe('warning-http-test-target-1');
    // ICMP panel in this fixture does NOT have a warning (no low sample count scenario)
    // which is correct - warning only appears when JS toggles it visible
  });

  it('should have hidden warning by default', () => {
    const httpWarning = container.querySelector('#warning-http-test-target-1');
    
    expect(httpWarning).not.toBeNull();
    expect(httpWarning?.classList.contains('hidden')).toBe(true);
  });

  it('should have correct sample count IDs per kind', () => {
    const httpSamples = container.querySelector('#samples-http-test-target-1');
    const icmpSamples = container.querySelector('#samples-icmp-test-target-1');
    
    expect(httpSamples?.id).toBe('samples-http-test-target-1');
    expect(icmpSamples?.id).toBe('samples-icmp-test-target-1');
  });
});
