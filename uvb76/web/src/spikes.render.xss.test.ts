// DOM renderer tests: XSS safety

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// Mock the api module
const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) =>
      mockGetLatencySpikesWithCaptures(...args),
  },
}));

// Import after mock setup
import { loadSpikeDiagnostics } from './spikes';
import {
  spikeResponseWithXssSource,
  spikeResponseWithXssError,
  spikeResponseWithXssSocket,
} from './spikes.render.fixtures';

describe('spikes DOM renderer: XSS safety', () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    vi.clearAllMocks();
    container = document.createElement('div');
    container.id = 'spike-diag-test-target';
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
  });

  it('escapes HTML-like text in capture source', async () => {
    const response = spikeResponseWithXssSource('<script>alert(1)</script>');
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // The literal text should be visible in textContent
    expect(container.textContent).toContain('<script>alert(1)</script>');
    // But no actual script element should exist
    expect(container.querySelector('script')).toBeNull();
  });

  it('escapes HTML-like text in error message', async () => {
    const response = spikeResponseWithXssError('<img src=x onerror=alert(1)>');
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // The literal text should be visible in textContent
    expect(container.textContent).toContain('<img src=x onerror=alert(1)>');
    // But no actual img element should exist
    expect(container.querySelector('img')).toBeNull();
  });

  it('escapes HTML-like text in TCP socket name', async () => {
    const response = spikeResponseWithXssSocket('<b>bold</b>');
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    // The literal text should be visible in textContent
    expect(container.textContent).toContain('<b>bold</b>');
    // But no actual b element should exist
    expect(container.querySelector('b')).toBeNull();
  });
});
