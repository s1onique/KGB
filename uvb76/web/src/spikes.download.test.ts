// DOM renderer tests: JSON download functionality with proper Blob verification

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

const mockGetLatencySpikesWithCaptures = vi.fn();

vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: (...args: unknown[]) =>
      mockGetLatencySpikesWithCaptures(...args),
  },
}));

import { loadSpikeDiagnostics } from './spikes';
import {
  spikeResponseWithOkCapture,
  createSpikeResponse,
  createSpike,
  createOkCapture,
  createNetworkDiag,
} from './spikes.render.fixtures';

describe('spikes DOM renderer: JSON download functionality', () => {
  let container: HTMLDivElement;
  let createdBlobs: Blob[] = [];
  let createdFilenames: string[] = [];

  beforeEach(() => {
    vi.clearAllMocks();
    container = document.createElement('div');
    container.id = 'spike-diag-test-target';
    document.body.appendChild(container);

    // Reset tracking arrays
    createdBlobs = [];
    createdFilenames = [];

    // Mock URL.createObjectURL on globalThis to work with jsdom
    const mockCreateObjectURL = (blob: Blob) => {
      createdBlobs.push(blob);
      return `blob:mock-${Date.now()}`;
    };
    const mockRevokeObjectURL = () => {};

    // Always set up on globalThis for jsdom compatibility
    (globalThis as Record<string, unknown>).URL = {
      createObjectURL: mockCreateObjectURL,
      revokeObjectURL: mockRevokeObjectURL,
    } as unknown as typeof URL;

    // Mock document.createElement to capture download filenames
    const originalCreateElement = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      const el = originalCreateElement(tagName);
      
      if (tagName.toLowerCase() === 'a') {
        // Capture download attribute when set via property
        Object.defineProperty(el, 'download', {
          set: (value: string) => {
            createdFilenames.push(value);
          },
          get: () => '',
        });
        
        // Override click to prevent actual navigation
        el.click = vi.fn();
      }
      
      return el;
    });
  });

  afterEach(() => {
    container.remove();
    vi.restoreAllMocks();
  });

  async function clickDownloadCapture(): Promise<void> {
    const btn = container.querySelector('.download-capture-btn') as HTMLButtonElement;
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 10));
  }

  async function clickDownloadSpike(): Promise<void> {
    const btn = container.querySelector('.download-spike-btn') as HTMLButtonElement;
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 10));
  }

  it('renders Download capture JSON button', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const btn = container.querySelector('.download-capture-btn');
    expect(btn).not.toBeNull();
    expect(btn?.textContent).toContain('Download capture JSON');
  });

  it('renders Download spike bundle button when spike has captures', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const btn = container.querySelector('.download-spike-btn');
    expect(btn).not.toBeNull();
    expect(btn?.textContent).toContain('Download spike bundle');
  });

  it('clicking Download spike bundle creates a JSON Blob', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadSpike();
    
    expect(createdBlobs.length).toBeGreaterThan(0);
    expect(createdBlobs[0].type).toBe('application/json');
  });

  it('spike bundle JSON has export_kind = uvb76_spike_diagnostics_bundle', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadSpike();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.export_kind).toBe('uvb76_spike_diagnostics_bundle');
  });

  it('spike bundle JSON includes target_id', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadSpike();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.target_id).toBe('test-target');
  });

  it('spike bundle JSON includes spike object with event_id', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadSpike();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.spike).toBeDefined();
    expect(data.spike.event_id).toBeDefined();
    expect(typeof data.spike.event_id).toBe('string');
  });

  it('spike bundle JSON includes captures array', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadSpike();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.captures).toBeDefined();
    expect(Array.isArray(data.captures)).toBe(true);
    expect(data.captures.length).toBeGreaterThan(0);
  });

  it('spike bundle filename is sanitized', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadSpike();
    
    expect(createdFilenames.length).toBeGreaterThan(0);
    const filename = createdFilenames[0];
    
    expect(filename).not.toContain('/');
    expect(filename).not.toContain('\\');
    expect(filename).not.toContain(':');
    expect(filename).not.toContain('?');
    expect(filename).not.toContain('&');
  });

  it('clicking Download capture JSON creates a JSON Blob', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    expect(createdBlobs.length).toBeGreaterThan(0);
    expect(createdBlobs[0].type).toBe('application/json');
  });

  it('JSON Blob contains export_kind = uvb76_diagnostic_capture', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.export_kind).toBe('uvb76_diagnostic_capture');
  });

  it('JSON Blob contains target_id', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.target_id).toBe('test-target');
  });

  it('JSON Blob contains spike_event_id', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.spike_event_id).toBe('evt-1');
  });

  it('JSON Blob contains capture object', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    expect(data.capture).toBeDefined();
    expect(data.capture.source).toBe('peer-1');
    expect(data.capture.status).toBe('ok');
  });

  it('generated filename is sanitized (no slashes)', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    expect(createdFilenames.length).toBeGreaterThan(0);
    const filename = createdFilenames[0];
    
    expect(filename).not.toContain('/');
    expect(filename).not.toContain('\\');
    expect(filename).not.toContain(':');
    expect(filename).not.toContain('?');
    expect(filename).not.toContain('&');
  });

  it('filename starts with uvb76-capture- and ends with .json', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    const filename = createdFilenames[0];
    expect(filename).toMatch(/^uvb76-capture-/);
    expect(filename).toMatch(/\.json$/);
  });

  it('source with special characters still downloads correct capture by index', async () => {
    // Create response with XSS-like source containing quotes
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer-"1', network_diag: undefined })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickDownloadCapture();
    
    // Should have created a blob successfully
    expect(createdBlobs.length).toBeGreaterThan(0);
    const blob = createdBlobs[0];
    const text = await blob.text();
    const data = JSON.parse(text);
    
    // Verify correct capture was downloaded (by index, not source)
    expect(data.capture.source).toBe('peer-"1');
    expect(data.capture.status).toBe('ok');
  });

  it('two captures from same source download correct capture by index', async () => {
    // Create response with two captures from same source
    const response = createSpikeResponse(createSpike({
      captures: [
        createOkCapture({ source: 'peer-1', duration_ms: 100 }),
        createOkCapture({ source: 'peer-1', duration_ms: 200 }),
      ],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Click first capture download
    const buttons = container.querySelectorAll('.download-capture-btn');
    (buttons[0] as HTMLButtonElement).click();
    await new Promise(r => setTimeout(r, 10));
    
    expect(createdBlobs.length).toBe(1);
    let blob = createdBlobs[0];
    let text = await blob.text();
    let data = JSON.parse(text);
    expect(data.capture.duration_ms).toBe(100);
    
    // Click second capture download
    (buttons[1] as HTMLButtonElement).click();
    await new Promise(r => setTimeout(r, 10));
    
    expect(createdBlobs.length).toBe(2);
    blob = createdBlobs[1];
    text = await blob.text();
    data = JSON.parse(text);
    expect(data.capture.duration_ms).toBe(200);
  });
});
