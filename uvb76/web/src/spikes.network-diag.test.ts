// DOM renderer tests: Network diagnostics missing warning

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
  createTcpSocket,
  createTcpAbsenceEvent,
} from './spikes.render.fixtures';

describe('spikes DOM renderer: network diagnostics missing warning', () => {
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

  it('renders Network diag: ok for ok capture with network_diag', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('ok');
    expect(container.textContent).not.toContain('missing');
  });

  it('renders Network diag: missing for ok capture without network_diag', async () => {
    // Create a capture with status 'ok' but no network_diag
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer-1', network_diag: undefined })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('missing');
  });

  it('renders missing network diag status for ok capture without network_diag', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer-1', network_diag: undefined })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // In table view, shows "Network diag: missing" in expanded details
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('missing');
  });

  it('does not render incomplete warning for suppressed capture', async () => {
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
    
    expect(container.textContent).not.toContain('Capture succeeded, but network diagnostics were not included');
  });

  it('does not render incomplete warning for error capture', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [{
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'error',
        error: 'connection refused',
      }],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).not.toContain('Capture succeeded, but network diagnostics were not included');
  });

  it('renders suppressed for network_diag status when suppressed_by_cooldown', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [{
        source: 'peer-1',
        base_url: 'http://10.0.0.1:8080',
        capture_started_at: '2026-06-18T12:00:00Z',
        status: 'ok',
        suppressed_by_cooldown: true,
        network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [createTcpSocket()] }),
      }],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Should show suppressed, not ok
    expect(container.textContent).toContain('Network diag:');
    expect(container.textContent).toContain('suppressed');
    expect(container.textContent).not.toContain('Network diag: ok');
  });

  it('renders Underlay TCP summary when network_diag present', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 50.0, rto_ms: 200, retransmits: 0 })],
      }),
    });
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    expect(container.textContent).toContain('Underlay TCP:');
  });

  it('renders no TCP summary when network_diag missing', async () => {
    const response = createSpikeResponse(createSpike({
      captures: [createOkCapture({ source: 'peer-1', network_diag: undefined })],
    }));
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Should not have Underlay TCP section in compact view when network_diag missing
    // The compact view shows Network diag: missing, but no Underlay TCP
    expect(container.textContent).not.toContain('Underlay TCP:');
  });
});

describe('spikes DOM renderer: TCP absence events', () => {
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

  it('renders absence explanation for empty underlay_tcp with tcp_absence_events', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
      tcp_absence_events: [createTcpAbsenceEvent({
        reason_code: 'no_matching_socket',
        source: 'tovarisch',
        expected_peer: 'wg0-peer-1',
        expected_port: 51820,
        probe_kind: 'http',
      })],
    });
    
    // Need to manually add tcp_absence_events to the capture
    const capture = response.spikes[0].captures![0];
    capture.tcp_absence_events = [createTcpAbsenceEvent({
      reason_code: 'no_matching_socket',
      source: 'tovarisch',
      expected_peer: 'wg0-peer-1',
      expected_port: 51820,
      probe_kind: 'http',
    })];
    
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Click View details to expand
    const viewDetailsBtn = container.querySelector('.view-details-btn');
    if (viewDetailsBtn) {
      viewDetailsBtn.dispatchEvent(new MouseEvent('click'));
    }
    
    expect(container.textContent).toContain('No TCP diagnostics captured');
    expect(container.textContent).toContain('no matching socket found');
    expect(container.textContent).toContain('tovarisch');
  });

  it('renders permission_denied absence reason', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    
    const capture = response.spikes[0].captures![0];
    capture.tcp_absence_events = [createTcpAbsenceEvent({
      reason_code: 'permission_denied',
      source: 'tovarisch',
      command_tool: 'ss',
      detail: 'Operation not permitted',
    })];
    
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Click View details to expand
    const viewDetailsBtn = container.querySelector('.view-details-btn');
    if (viewDetailsBtn) {
      viewDetailsBtn.dispatchEvent(new MouseEvent('click'));
    }
    
    expect(container.textContent).toContain('permission denied for diagnostic commands');
    expect(container.textContent).toContain('Tool: ss');
  });

  it('renders target_mapping_missing absence reason', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    
    const capture = response.spikes[0].captures![0];
    capture.tcp_absence_events = [createTcpAbsenceEvent({
      reason_code: 'target_mapping_missing',
      source: 'tovarisch',
    })];
    
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Click View details to expand
    const viewDetailsBtn = container.querySelector('.view-details-btn');
    if (viewDetailsBtn) {
      viewDetailsBtn.dispatchEvent(new MouseEvent('click'));
    }
    
    expect(container.textContent).toContain('target peer mapping not found');
  });

  it('renders multiple absence events', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({ status: 'ok', underlay_tcp: [] }),
    });
    
    const capture = response.spikes[0].captures![0];
    capture.tcp_absence_events = [
      createTcpAbsenceEvent({
        reason_code: 'no_matching_socket',
        source: 'tovarisch',
      }),
      createTcpAbsenceEvent({
        reason_code: 'command_failed',
        source: 'tovarisch',
        command_tool: 'ss',
        detail: 'exit code 1',
      }),
    ];
    
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Click View details to expand
    const viewDetailsBtn = container.querySelector('.view-details-btn');
    if (viewDetailsBtn) {
      viewDetailsBtn.dispatchEvent(new MouseEvent('click'));
    }
    
    expect(container.textContent).toContain('no matching socket found');
    expect(container.textContent).toContain('diagnostic command failed');
  });

  it('does not render absence explanation when underlay_tcp has data', async () => {
    const response = spikeResponseWithOkCapture({
      source: 'peer-1',
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 50.0, rto_ms: 200, retransmits: 0 })],
      }),
    });
    
    const capture = response.spikes[0].captures![0];
    capture.tcp_absence_events = [createTcpAbsenceEvent({
      reason_code: 'no_matching_socket',
      source: 'tovarisch',
    })];
    
    mockGetLatencySpikesWithCaptures.mockResolvedValue(response);
    await loadSpikeDiagnostics('test-target');
    
    // Click View details to expand
    const viewDetailsBtn = container.querySelector('.view-details-btn');
    if (viewDetailsBtn) {
      viewDetailsBtn.dispatchEvent(new MouseEvent('click'));
    }
    
    // Should show TCP socket data, not absence explanation
    expect(container.textContent).toContain('socket: xray');
    expect(container.textContent).toContain('state: ESTAB');
    expect(container.textContent).not.toContain('no matching socket found');
  });
});
