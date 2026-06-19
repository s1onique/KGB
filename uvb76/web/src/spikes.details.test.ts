// DOM renderer tests: View details expand/collapse with XSS safety

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
  createNetworkDiag,
  createTcpSocket,
  createSpikeResponse,
  createSpike,
  createOkCapture,
  createErrorCapture,
  spikeResponseWithSuppressedCapture,
} from './spikes.render.fixtures';

describe('spikes DOM renderer: View details expand/collapse', () => {
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

  async function clickViewDetails(): Promise<void> {
    const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 10));
  }

  it('renders View details button for each capture', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const btn = container.querySelector('.view-details-btn');
    expect(btn).not.toBeNull();
    expect(btn?.textContent).toContain('View details');
  });

  it('hides capture details initially', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const details = container.querySelector('.capture-details');
    expect(details).not.toBeNull();
    expect(details?.getAttribute('style')).toContain('none');
  });

  it('shows capture details after clicking View details', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    const details = container.querySelector('.capture-details');
    expect(details?.getAttribute('style')).not.toContain('none');
  });

  it('toggles to Hide details after clicking', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    const btn = container.querySelector('.view-details-btn');
    expect(btn?.textContent).toContain('Hide details');
  });

  it('collapses details on second click', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails(); // expand
    await clickViewDetails(); // collapse
    
    const details = container.querySelector('.capture-details');
    expect(details?.getAttribute('style')).toContain('none');
    
    const btn = container.querySelector('.view-details-btn');
    expect(btn?.textContent).toContain('View details');
  });

  it('renders Source in details', async () => {
    const response = spikeResponseWithOkCapture({ source: 'test-peer' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('Source:');
    expect(container.textContent).toContain('test-peer');
  });

  it('renders Status in details', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('Status:');
    expect(container.textContent).toContain('ready');
  });

  it('renders Duration in details', async () => {
    const response = spikeResponseWithOkCapture({ duration_ms: 456 });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('Duration:');
    expect(container.textContent).toContain('456 ms');
  });

  it('renders Started timestamp in details', async () => {
    const response = spikeResponseWithOkCapture({ source: 'peer-1' });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('Started:');
  });

  it('renders Suppressed by cooldown in details', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [{
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
      }],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('Suppressed by cooldown:');
    expect(container.textContent).toContain('yes');
  });

  it('renders RTT in details for ok capture with network_diag', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 123.4, rto_ms: 500, retransmits: 2 })],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('RTT:');
    expect(container.textContent).toContain('123.4 ms');
  });

  it('renders RTO in details for ok capture with network_diag', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 123.4, rto_ms: 500, retransmits: 2 })],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('RTO:');
    expect(container.textContent).toContain('500 ms');
  });

  it('renders retransmits in details', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 123.4, rto_ms: 500, retransmits: 2 })],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('retransmits:');
    expect(container.textContent).toContain('2');
  });

  it('renders unacked in details', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 123.4, rto_ms: 500, retransmits: 2, unacked: 5 })],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('unacked:');
    expect(container.textContent).toContain('5');
  });

  it('renders cwnd in details', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 123.4, rto_ms: 500, retransmits: 2, cwnd: 10 })],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('cwnd:');
    expect(container.textContent).toContain('10');
  });

  it('renders compact safe error for error capture', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createErrorCapture({ error: 'connection refused', source: 'peer-1' })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('Error:');
    expect(container.textContent).toContain('connection refused');
  });

  it('shows suppressed for network_diag status when suppressed_by_cooldown', async () => {
    const response = spikeResponseWithSuppressedCapture({
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket()],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    await clickViewDetails();
    
    expect(container.textContent).toContain('suppressed');
  });
});

describe('spikes DOM renderer: XSS safety with special source characters', () => {
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

  it('source with quotes still renders safely', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer"1' })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Source should be HTML-escaped in display
    expect(container.innerHTML).not.toContain('onclick=');
    expect(container.innerHTML).not.toContain('onerror=');
  });

  it('source with onclick still renders safely', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer" onclick="alert(1)' })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // onclick attribute should not be exploitable - quotes are escaped
    expect(container.innerHTML).not.toContain('onclick="alert(1)"');
    // alert(1) text may appear as safe escaped text in the DOM
    expect(container.innerHTML).not.toContain('="alert(1)"');
  });

  it('source with angle brackets still renders safely', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: '<script>alert(1)</script>' })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Script tag should be escaped as <script> (not executable)
    expect(container.innerHTML).not.toContain('<script>');
    expect(container.innerHTML).toContain('&lt;script&gt;');
  });

  it('source with quotes still allows View details to work', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer"1' })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    const btn = container.querySelector('.view-details-btn');
    expect(btn).not.toBeNull();
    
    // Click should work without errors
    btn?.click();
    await new Promise(r => setTimeout(r, 10));
    
    // Details should be visible
    const details = container.querySelector('.capture-details');
    expect(details?.getAttribute('style')).not.toContain('none');
  });
});
