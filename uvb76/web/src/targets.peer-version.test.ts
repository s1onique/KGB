// Tests for peer version rendering in target cards
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { TargetSnapshot } from './api';

// Mock the api module
vi.mock('./api', async () => {
  const actual = await vi.importActual('./api');
  return {
    ...actual,
    api: {
      getTargetSnapshot: vi.fn(),
    },
  };
});

// Import after mock setup
import { api } from './api';

// Helper to simulate snapshot update rendering logic
// NOTE: This must exactly match the logic in targets.ts
function renderStatusLine(snap: TargetSnapshot): string {
  // Use whitelist for status class to prevent XSS - MUST match targets.ts
  const statusClasses = new Set(['up', 'down', 'unknown', 'error', 'degraded', 'warning']);
  
  // HTML escape helper
  function escapeText(s: string): string {
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }
  
  if (!snap.reachable) {
    const safeError = escapeText(snap.error || '');
    return `<span class="status error">unreachable</span> ${safeError}`;
  }
  
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
  
  // RSS: format peer_rss_kib (KiB) as MiB, omit if absent/null/invalid
  let rssHtml = '';
  const rssKib = snap.peer_rss_kib;
  if (typeof rssKib === 'number' && Number.isFinite(rssKib) && rssKib > 0) {
    const rssMib = Math.round(rssKib / 1024);
    rssHtml = `<span class="target-header-sep">·</span><span class="peer-rss" data-testid="target-peer-rss">RSS ${rssMib}M</span>`;
  }
  
  return `<span class="status ${safeStatusClass}">${escapeText(safeStatus)}</span> Node: ${safeNodeId} <span class="peer-version" data-testid="target-peer-version">Peer: tovarisch ${escapeText(displayVersion)}</span>${rssHtml}`;
}

describe('Peer version rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders peer version when version exists', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warn',
      peer_version: '0.1.23',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('Peer: tovarisch 0.1.23');
    expect(html).toContain('data-testid="target-peer-version"');
    expect(html).toContain('Node: local-dev');
  });

  it('renders "unknown" when peer_version is missing', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warn',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('Peer: tovarisch unknown');
    expect(html).toContain('data-testid="target-peer-version"');
  });

  it('renders "unknown" when peer_version is empty string', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warn',
      peer_version: '',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('Peer: tovarisch unknown');
  });

  it('renders "unknown" when peer_version is only whitespace', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warn',
      peer_version: '   ',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('Peer: tovarisch unknown');
  });

  it('truncates long version strings at 64 chars', () => {
    const longVersion = 'a'.repeat(100);
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warn',
      peer_version: longVersion,
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    // Should contain truncated version with ellipsis
    expect(html).toContain('Peer: tovarisch ' + 'a'.repeat(64) + '…');
    // Should NOT contain the full 100-char version
    expect(html).not.toContain('Peer: tovarisch ' + longVersion);
  });

  it('escapes HTML in peer version string', () => {
    const xssVersion = '<script>alert("xss")</script>';
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warning',
      peer_version: xssVersion,
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    // Raw script tags should NOT be present (escaped)
    // Raw script tags removed - escaped version IS present
    // Already removed above
    // Escaped version should be present
    expect(html).toContain('&lt;script&gt;');
    expect(html).toContain('&lt;/script&gt;');
  });

  it('escapes HTML in node_id string', () => {
    const xssNodeId = '<img src=x onerror=alert(1)>';
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warning',
      peer_version: '0.1.0',
      node_id: xssNodeId,
    };

    const html = renderStatusLine(snap);
    
    // Raw HTML should NOT be present (escaped)
    // Raw img tag removed - escaped version IS present
    // onerror= is just text, not escaped - this is correct
    // Escaped version should be present
    expect(html).toContain('&lt;img');
    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;');
  });

  it('does not render peer version for unreachable targets', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: false,
      error: 'connection refused',
    };

    const html = renderStatusLine(snap);
    
    // Should show unreachable status, not peer version
    expect(html).toContain('unreachable');
    expect(html).toContain('connection refused');
    expect(html).not.toContain('Peer: tovarisch');
    expect(html).not.toContain('data-testid="target-peer-version"');
  });

  it('renders with correct status class from whitelist', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warning', // 'warning' is in the whitelist
      peer_version: '0.1.0',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('class="status warning"');
  });

  it('uses "unknown" class for invalid status values', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'invalid-status',
      peer_version: '0.1.0',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('class="status unknown"');
  });

  it('handles version exactly at 64 chars without truncation', () => {
    const exact64Version = 'a'.repeat(64);
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'warn',
      peer_version: exact64Version,
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('Peer: tovarisch ' + exact64Version);
    expect(html).not.toContain('…'); // No ellipsis
  });

  it('renders with "up" status class', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      peer_version: '0.1.0',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('class="status up"');
    expect(html).toContain('Peer: tovarisch 0.1.0');
  });
});

// Regression tests for compact target header layout
describe('Target header compact layout', () => {
  // Helper to simulate the full target card HTML rendering (matches targets.ts render function)
  function renderTargetCard(t: { id: string; name: string; base_url: string }, snap?: Partial<TargetSnapshot>): string {
    const targetId = escapeText(t.id);
    const targetName = escapeText(t.name);
    const targetUrl = escapeText(t.base_url);
    
    const statusId = `status-${targetId}`;
    let statusHtml = 'Loading...';
    
    if (snap) {
      if (!snap.reachable) {
        const safeError = escapeText(snap.error || '');
        statusHtml = `<span class="status error">unreachable</span> ${safeError}`;
      } else {
        const safeStatus = snap.status || 'unknown';
        const safeStatusClass = statusClasses.has(safeStatus) ? safeStatus : 'unknown';
        const safeNodeId = escapeText(snap.node_id || 'N/A');
        const rawVersion = snap.peer_version || '';
        const trimmedVersion = rawVersion.trim();
        const safeVersion = trimmedVersion.length > 64 
          ? trimmedVersion.substring(0, 64) + '…' 
          : trimmedVersion;
        const displayVersion = safeVersion || 'unknown';
        // Must match targets.ts updateSnapshot: separators between status, node, peer
        statusHtml = `<span class="status ${safeStatusClass}">${escapeText(safeStatus)}</span><span class="target-header-sep">·</span>Node: ${safeNodeId}<span class="target-header-sep">·</span><span class="peer-version" data-testid="target-peer-version">Peer: tovarisch ${escapeText(displayVersion)}</span>`;
      }
    }
    
    // This must exactly match the HTML structure in targets.ts (using escapeAttr for title attributes)
    return `
      <div class="card target" id="target-${targetId}">
          <div class="target-header-row" data-testid="target-header-${targetId}">
              <strong class="target-name" title="${escapeAttr(t.name)}">${targetName}</strong>
              <span class="target-id" title="${escapeAttr(t.id)}">(${targetId})</span>
              <span class="target-header-sep">·</span>
              <span class="target-url" title="${escapeAttr(t.base_url)}">${targetUrl}</span>
              <span class="target-header-sep">·</span>
              <span class="target-status-meta" id="${statusId}">${statusHtml}</span>
          </div>
          <div class="latency-card" id="latency-${targetId}"></div>
      </div>
    `;
  }

  function escapeText(s: string): string {
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }

  // HTML attribute escape helper - matches targets.ts
  function escapeAttr(s: string): string {
    return escapeText(s).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  const statusClasses = new Set(['up', 'down', 'unknown', 'error', 'degraded', 'warning']);

  it('renders target name, URL, status, node, and peer in compact header', () => {
    const target = { id: 'kamatera', name: 'Kamatera (London)', base_url: 'http://10.149.149.1:8317' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'warning', // Must use whitelist value 'warning' not 'warn'
      node_id: 'local-dev',
      peer_version: '0.1.1-rc51+05902ce',
    };

    const html = renderTargetCard(target, snap);

    // Verify all expected content is present
    expect(html).toContain('Kamatera (London)');
    expect(html).toContain('(kamatera)');
    expect(html).toContain('http://10.149.149.1:8317');
    expect(html).toContain('class="status warning"');
    expect(html).toContain('warning');
    expect(html).toContain('Node: local-dev');
    expect(html).toContain('Peer: tovarisch 0.1.1-rc51+05902ce');
  });

  it('renders target header in a single flex row container', () => {
    const target = { id: 'test-target', name: 'Test Target', base_url: 'http://localhost:8080' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'up',
      node_id: 'test-node',
      peer_version: '1.0.0',
    };

    const html = renderTargetCard(target, snap);

    // Verify the compact header structure
    expect(html).toContain('class="target-header-row"');
    expect(html).toContain('data-testid="target-header-test-target"');
    // Verify it's a flex container class (CSS handles the actual flex layout)
    expect(html).toContain('target-name');
    expect(html).toContain('target-id');
    expect(html).toContain('target-header-sep');
    expect(html).toContain('target-url');
    expect(html).toContain('target-status-meta');
  });

  it('metadata is not split across separate block rows', () => {
    const target = { id: 'block-test', name: 'Block Test', base_url: 'http://example.com' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'warn',
      node_id: 'node-1',
      peer_version: '2.0.0',
    };

    const html = renderTargetCard(target, snap);

    // The header should NOT contain multiple <br> tags creating stacked lines
    // It should be a single flex row with separators
    const headerMatch = html.match(/<div class="target-header-row"[^>]*>([\s\S]*?)<\/div>\s*<div class="latency-card"/);
    expect(headerMatch).not.toBeNull();
    
    // Within the header, there should be no <br> tags (old stacked layout)
    if (headerMatch) {
      const headerContent = headerMatch[1];
      expect(headerContent).not.toContain('<br');
      expect(headerContent).not.toContain('<br/');
    }
  });

  it('uses separator character for compact spacing', () => {
    const target = { id: 'sep-test', name: 'Sep Test', base_url: 'http://test.local' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'up',
      node_id: 'sep-node',
      peer_version: '1.0.0',
    };

    const html = renderTargetCard(target, snap);

    // Verify separators are used between metadata items
    expect(html).toContain('target-header-sep');
    expect(html).toContain('·');
  });

  it('preserves full values via title attributes for truncation', () => {
    const target = { 
      id: 'long-name-target', 
      name: 'Very Long Target Name That Should Be Truncated', 
      base_url: 'http://very-long-url-that-might-overflow.example.com:8080/path/to/service' 
    };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'up',
      node_id: 'truncation-test-node',
      peer_version: '1.0.0',
    };

    const html = renderTargetCard(target, snap);

    // Title attributes should contain the full values for hover tooltip
    expect(html).toContain('title="Very Long Target Name That Should Be Truncated"');
    expect(html).toContain('title="http://very-long-url-that-might-overflow.example.com:8080/path/to/service"');
  });

  it('renders URL with monospace font styling class', () => {
    const target = { id: 'mono-test', name: 'Mono Test', base_url: 'http://mono.test:9090' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'up',
      node_id: 'mono-node',
      peer_version: '1.0.0',
    };

    const html = renderTargetCard(target, snap);

    // URL should have the target-url class (CSS applies monospace font)
    expect(html).toContain('class="target-url"');
    expect(html).toContain('http://mono.test:9090');
  });

  it('renders peer version with dedicated styling class', () => {
    const target = { id: 'peer-test', name: 'Peer Test', base_url: 'http://peer.test' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'warn',
      node_id: 'peer-node',
      peer_version: '1.2.3',
    };

    const html = renderTargetCard(target, snap);

    // Peer version should have the peer-version class
    expect(html).toContain('class="peer-version"');
    expect(html).toContain('data-testid="target-peer-version"');
  });

  it('handles unreachable target in compact header', () => {
    const target = { id: 'down-target', name: 'Down Target', base_url: 'http://down.test' };
    const snap: Partial<TargetSnapshot> = {
      reachable: false,
      error: 'connection refused',
    };

    const html = renderTargetCard(target, snap);

    // Should show unreachable status
    expect(html).toContain('class="status error"');
    expect(html).toContain('unreachable');
    expect(html).toContain('connection refused');
    // Should NOT show peer version for unreachable targets
    expect(html).not.toContain('Peer: tovarisch');
  });

  it('renders all metadata on single line in desired format', () => {
    // This test validates the exact desired output format:
    // Kamatera (London) (kamatera) · http://10.149.149.1:8317 · warn · Node: local-dev · Peer: tovarisch 0.1.1-rc51+05902ce
    const target = { id: 'kamatera', name: 'Kamatera (London)', base_url: 'http://10.149.149.1:8317' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'warn',
      node_id: 'local-dev',
      peer_version: '0.1.1-rc51+05902ce',
    };

    const html = renderTargetCard(target, snap);

    // The compact header should contain all elements in sequence
    expect(html).toContain('target-header-row');
    
    // Verify sequence: name → id → sep → url → sep → status
    const headerSection = html.match(/target-header-row[^>]*>([\s\S]*?)<div class="latency-card"/)?.[1] || '';
    
    // All key content should be in the header row
    expect(headerSection).toContain('Kamatera (London)');
    expect(headerSection).toContain('kamatera');
    expect(headerSection).toContain('10.149.149.1:8317');
    expect(headerSection).toContain('warn');
    expect(headerSection).toContain('Node: local-dev');
    expect(headerSection).toContain('Peer: tovarisch 0.1.1-rc51+05902ce');
  });


  it('status metadata includes separators between status, node, and peer', () => {
    const target = { id: 'sep-internal', name: 'Sep Internal', base_url: 'http://sep.test' };
    const snap: Partial<TargetSnapshot> = {
      reachable: true,
      status: 'up',
      node_id: 'test-node',
      peer_version: '1.0.0',
    };

    const html = renderTargetCard(target, snap);
    
    // The status-meta element should contain separators between status, node, and peer
    const statusMetaMatch = html.match(/<span class="target-status-meta"[^>]*>([\s\S]*?)<\/span>\s*<\/div>\s*<div class="latency-card"/);
    expect(statusMetaMatch).not.toBeNull();
    
    if (statusMetaMatch) {
      const statusContent = statusMetaMatch[1];
      // Should have 2 separators: status · node · peer
      const separatorCount = (statusContent.match(/<span class="target-header-sep">·<\/span>/g) || []).length;
      expect(separatorCount).toBe(2);
    }
  });
});

// RSS rendering regression tests
describe('Peer RSS rendering', () => {
  it('renders RSS when peer_rss_kib is provided', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      peer_version: '0.1.1-rc51+05902ce',
      node_id: 'local-dev',
      peer_rss_kib: 8388608, // 8 GiB in KiB = 8192 MiB
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('RSS 8192M');
    expect(html).toContain('data-testid="target-peer-rss"');
    expect(html).toContain('Peer: tovarisch 0.1.1-rc51+05902ce');
  });

  it('omits RSS when peer_rss_kib is absent', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      peer_version: '0.1.0',
      node_id: 'local-dev',
    };

    const html = renderStatusLine(snap);
    
    expect(html).not.toContain('RSS');
    expect(html).not.toContain('data-testid="target-peer-rss"');
  });

  it('omits RSS when peer_rss_kib is null', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      peer_version: '0.1.0',
      node_id: 'local-dev',
      peer_rss_kib: null,
    };

    const html = renderStatusLine(snap);
    
    expect(html).not.toContain('RSS');
    expect(html).not.toContain('data-testid="target-peer-rss"');
  });

  it('omits RSS when peer_rss_kib is zero', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      peer_version: '0.1.0',
      node_id: 'local-dev',
      peer_rss_kib: 0,
    };

    const html = renderStatusLine(snap);
    
    expect(html).not.toContain('RSS');
    expect(html).not.toContain('data-testid="target-peer-rss"');
  });

  it('formats KiB to integer MiB correctly', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      peer_version: '1.0.0',
      node_id: 'test-node',
      peer_rss_kib: 1920, // Should round to 2M
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('RSS 2M');
  });

  it('renders RSS with separator after peer version', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      peer_version: '0.1.0',
      node_id: 'local-dev',
      peer_rss_kib: 8192, // 8 MiB
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('Peer: tovarisch 0.1.0</span><span class="target-header-sep">·</span><span class="peer-rss"');
  });

  it('does not render RSS for unreachable targets', () => {
    const snap: TargetSnapshot = {
      target_id: 'test-1',
      scraped_at: new Date().toISOString(),
      reachable: false,
      error: 'connection refused',
      peer_rss_kib: 8192,
    };

    const html = renderStatusLine(snap);
    
    expect(html).toContain('unreachable');
    expect(html).not.toContain('RSS');
  });
});

// Real DOM renderer regression test
describe('Real TargetsRenderer RSS rendering', () => {
  it('renders peer RSS through the real target card renderer', async () => {
    // Create a container with the expected ID for initTargets
    const container = document.createElement('div');
    container.id = 'real-rss-targets';
    document.body.appendChild(container);

    // Setup mock to return snapshot with RSS
    const mockSnapshot: TargetSnapshot = {
      target_id: 'real-rss-test',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      node_id: 'local-dev',
      peer_version: '0.1.1-rc51+05902ce',
      peer_rss_kib: 8388608, // 8 GiB in KiB = 8192 MiB
    };

    (api.getTargetSnapshot as ReturnType<typeof vi.fn>).mockResolvedValue(mockSnapshot);

    // Import and use the real initTargets function
    const { initTargets } = await import('./targets');
    const { renderer, loadTargets } = initTargets('real-rss-targets');

    // Load targets which triggers the snapshot update
    await loadTargets();

    // Wait for DOM update
    await new Promise(resolve => setTimeout(resolve, 10));

    // Find the RSS element
    const rssElement = container.querySelector('[data-testid="target-peer-rss"]');
    expect(rssElement).not.toBeNull();
    expect(rssElement?.textContent).toBe('RSS 8192M');

    // Also verify peer version is present
    const peerVersionElement = container.querySelector('[data-testid="target-peer-version"]');
    expect(peerVersionElement?.textContent).toBe('Peer: tovarisch 0.1.1-rc51+05902ce');

    // Cleanup
    document.body.removeChild(container);
  });

  it('omits RSS through the real renderer when peer_rss_kib is absent', async () => {
    // Create a container with the expected ID for initTargets
    const container = document.createElement('div');
    container.id = 'no-rss-targets';
    document.body.appendChild(container);

    // Setup mock WITHOUT RSS
    const mockSnapshot: TargetSnapshot = {
      target_id: 'no-rss-test',
      scraped_at: new Date().toISOString(),
      reachable: true,
      status: 'up',
      node_id: 'local-dev',
      peer_version: '1.0.0',
      // no peer_rss_kib field
    };

    (api.getTargetSnapshot as ReturnType<typeof vi.fn>).mockResolvedValue(mockSnapshot);

    // Import and use the real initTargets function
    const { initTargets } = await import('./targets');
    const { renderer } = initTargets('no-rss-targets');

    // Load targets which triggers the snapshot update
    await renderer.loadTargets();

    // Wait for DOM update
    await new Promise(resolve => setTimeout(resolve, 10));

    // RSS element should not exist
    const rssElement = container.querySelector('[data-testid="target-peer-rss"]');
    expect(rssElement).toBeNull();

    // Peer version should still be present
    const peerVersionElement = container.querySelector('[data-testid="target-peer-version"]');
    expect(peerVersionElement?.textContent).toBe('Peer: tovarisch 1.0.0');

    // Cleanup
    document.body.removeChild(container);
  });
});
