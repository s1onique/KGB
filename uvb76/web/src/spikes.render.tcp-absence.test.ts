// DOM renderer tests: TCP absence explanation rendering

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
import { createSpike, createSpikeResponse, createOkCapture, createNetworkDiag } from './spikes.render.fixtures';
import type { SpikeResponseWithCaptures, TcpAbsenceEvent } from './api';

describe('spikes DOM renderer: TCP absence explanation rendering', () => {
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

  // Helper to create a spike response with TCP absence events
  function createTcpAbsenceSpikeResponse(
    tcpAbsenceEvents: TcpAbsenceEvent[]
  ): SpikeResponseWithCaptures {
    return createSpikeResponse(
      createSpike({
        captures: [
          createOkCapture({
            source: 'kamatera-tovarisch',
            network_diag: createNetworkDiag({
              underlay_tcp: [], // Empty - triggers TCP absence rendering
            }),
          }),
        ],
      })
    );
  }

  // Helper to inject tcp_absence_events into the capture
  function injectTcpAbsenceEvents(response: SpikeResponseWithCaptures, events: TcpAbsenceEvent[]): SpikeResponseWithCaptures {
    if (response.spikes[0]?.captures?.[0]) {
      response.spikes[0].captures[0].tcp_absence_events = events;
    }
    return response;
  }

  describe('TCP diagnostics disabled by config', () => {
    it('renders structured message for underlay_tcp_disabled reason code', async () => {
      const events: TcpAbsenceEvent[] = [
        {
          reason_code: 'underlay_tcp_disabled',
          source: 'underlay_tcp',
          expected_peer: 'kamatera-tovarisch',
        },
      ];

      let response = createTcpAbsenceSpikeResponse(events);
      response = injectTcpAbsenceEvents(response, events);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);

      await loadSpikeDiagnostics('test-target');

      // Click to expand details
      const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
      if (btn) btn.click();
      await new Promise(r => setTimeout(r, 10));

      const text = container.textContent || '';

      // Verify readable reason message is rendered
      expect(text).toContain('underlay TCP diagnostics disabled by config');

      // Verify expected peer is a distinct rendered string
      expect(text).toContain('Expected peer:');
      expect(text).toContain('kamatera-tovarisch');
      expect(text).toContain('Expected peer: kamatera-tovarisch');
    });

    it('does NOT render concatenated bad strings', async () => {
      const events: TcpAbsenceEvent[] = [
        {
          reason_code: 'underlay_tcp_disabled',
          source: 'underlay_tcp',
          expected_peer: 'kamatera-tovarisch',
        },
      ];

      let response = createTcpAbsenceSpikeResponse(events);
      response = injectTcpAbsenceEvents(response, events);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);

      await loadSpikeDiagnostics('test-target');

      // Click to expand details
      const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
      if (btn) btn.click();
      await new Promise(r => setTimeout(r, 10));

      const text = container.textContent || '';

      // These concatenated strings should NOT appear
      expect(text).not.toContain('kamatera-tovarischunderlay');
      expect(text).not.toContain('expected_peer: kamatera-tovarischunderlay');
      expect(text).not.toContain('kamatera-tovarisch underlay TCP diagnostics disabled by config');
    });

    it('renders Expected peer as distinct row', async () => {
      const events: TcpAbsenceEvent[] = [
        {
          reason_code: 'underlay_tcp_disabled',
          source: 'underlay_tcp',
          expected_peer: 'kamatera-tovarisch',
        },
      ];

      let response = createTcpAbsenceSpikeResponse(events);
      response = injectTcpAbsenceEvents(response, events);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);

      await loadSpikeDiagnostics('test-target');

      // Click to expand details
      const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
      if (btn) btn.click();
      await new Promise(r => setTimeout(r, 10));

      // Check for the structured row element
      const expectedPeerRow = container.querySelector('.tcp-absence-detail-row');
      expect(expectedPeerRow).not.toBeNull();

      // Verify the content
      const expectedPeerLabel = container.querySelector('.tcp-absence-label');
      expect(expectedPeerLabel?.textContent).toBe('Expected peer:');

      const expectedPeerValue = container.querySelector('.tcp-absence-value');
      expect(expectedPeerValue?.textContent).toBe('kamatera-tovarisch');
    });

    it('renders reason as distinct row', async () => {
      const events: TcpAbsenceEvent[] = [
        {
          reason_code: 'underlay_tcp_disabled',
          source: 'underlay_tcp',
          expected_peer: 'kamatera-tovarisch',
        },
      ];

      let response = createTcpAbsenceSpikeResponse(events);
      response = injectTcpAbsenceEvents(response, events);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);

      await loadSpikeDiagnostics('test-target');

      // Click to expand details
      const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
      if (btn) btn.click();
      await new Promise(r => setTimeout(r, 10));

      // Check for the reason row element
      const reasonRow = container.querySelector('.tcp-absence-reason-row');
      expect(reasonRow).not.toBeNull();

      const reasonText = container.querySelector('.tcp-absence-reason');
      expect(reasonText?.textContent).toContain('underlay TCP diagnostics disabled by config');
    });
  });

  describe('not_configured reason code', () => {
    it('renders human-readable message for not_configured', async () => {
      const events: TcpAbsenceEvent[] = [
        {
          reason_code: 'not_configured',
          source: 'underlay_tcp',
          expected_peer: 'test-peer',
        },
      ];

      let response = createTcpAbsenceSpikeResponse(events);
      response = injectTcpAbsenceEvents(response, events);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);

      await loadSpikeDiagnostics('test-target');

      // Click to expand details
      const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
      if (btn) btn.click();
      await new Promise(r => setTimeout(r, 10));

      const text = container.textContent || '';

      // Verify human-readable message
      expect(text).toContain('TCP diagnostics are disabled for this peer');
      expect(text).toContain('Expected peer:');
      expect(text).toContain('test-peer');
    });
  });

  describe('parse_failed regression', () => {
    it('renders safely for malformed TCP diagnostic output', async () => {
      const events: TcpAbsenceEvent[] = [
        {
          reason_code: 'parse_failed',
          source: 'underlay_tcp',
          detail: 'unexpected output format',
        },
      ];

      let response = createTcpAbsenceSpikeResponse(events);
      response = injectTcpAbsenceEvents(response, events);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);

      await loadSpikeDiagnostics('test-target');

      // Click to expand details
      const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
      if (btn) btn.click();
      await new Promise(r => setTimeout(r, 10));

      const text = container.textContent || '';

      // Verify it renders without concatenation issues
      expect(text).toContain('failed to parse diagnostic output');
      expect(text).toContain('unexpected output format');
    });
  });

  describe('no_matching_socket regression', () => {
    it('renders correctly for no matching socket', async () => {
      const events: TcpAbsenceEvent[] = [
        {
          reason_code: 'no_matching_socket',
          source: 'underlay_tcp',
          expected_peer: 'kamatera-tovarisch',
          expected_port: 51820,
          probe_kind: 'http',
        },
      ];

      let response = createTcpAbsenceSpikeResponse(events);
      response = injectTcpAbsenceEvents(response, events);
      mockGetLatencySpikesWithCaptures.mockResolvedValue(response);

      await loadSpikeDiagnostics('test-target');

      // Click to expand details
      const btn = container.querySelector('.view-details-btn') as HTMLButtonElement;
      if (btn) btn.click();
      await new Promise(r => setTimeout(r, 10));

      const text = container.textContent || '';

      // Verify structured rendering
      expect(text).toContain('no matching socket found');
      expect(text).toContain('Expected peer: kamatera-tovarisch');
      expect(text).toContain('Expected port: 51820');
      expect(text).toContain('Probe: http');
    });
  });
});
