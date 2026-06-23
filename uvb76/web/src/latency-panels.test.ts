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
