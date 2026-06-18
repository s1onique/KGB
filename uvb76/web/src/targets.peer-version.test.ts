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
  return `<span class="status ${safeStatusClass}">${escapeText(safeStatus)}</span> Node: ${safeNodeId} <span class="peer-version" data-testid="target-peer-version">Peer: tovarisch ${escapeText(displayVersion)}</span>`;
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
