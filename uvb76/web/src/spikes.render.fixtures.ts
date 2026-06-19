// Shared fixtures and helpers for spike diagnostics DOM renderer tests
// This file is NOT test-discovered (not a .test.ts file)
//
// NOTE: Mock setup must be done in each .test.ts file before importing loadSpikeDiagnostics.
// See spikes.render.empty.test.ts for the correct pattern.

import type { SpikeResponseWithCaptures, DiagCapture, TcpSocketDiagData, NetworkDiagData, SpikeRetentionStats } from './api';

// Default retention stats for tests
export const defaultRetention: SpikeRetentionStats = {
  retained_spike_count: 1,
  visible_spike_count: 1,
  protected_capture_count: 1,
  purge_eligible_count: 0,
  max_uncaptured_spikes: 200,
};

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

/** Create a minimal spike event with the given overrides */
export function createSpike(overrides: Partial<{
  event_id: string;
  target_id: string;
  kind: string;
  severity: string;
  latency_ms: number;
  captures: DiagCapture[];
}> = {}): SpikeResponseWithCaptures['spikes'][0] {
  return {
    event_id: 'evt-1',
    target_id: 'test-target',
    kind: 'http',
    severity: 'warning',
    sample_ts: '2026-06-18T12:00:00Z',
    latency_ms: 1234,
    rolling_median_ms: 100,
    reasons: [],
    thresholds: { warning_ms: 500, critical_ms: 1000, relative_multiplier: 10 },
    previous_samples: [],
    collected_at: '2026-06-18T12:00:00Z',
    ...overrides,
  };
}

/** Create a spike response with a single spike */
export function createSpikeResponse(spike: ReturnType<typeof createSpike>, retention?: Partial<SpikeRetentionStats>): SpikeResponseWithCaptures {
  return {
    spikes: [spike],
    count: 1,
    retention: { ...defaultRetention, ...retention },
  };
}

/** Create an ok capture with the given overrides */
export function createOkCapture(overrides: Partial<DiagCapture> = {}): DiagCapture {
  return {
    source: 'peer-1',
    base_url: 'http://10.0.0.1:8080',
    capture_started_at: '2026-06-18T12:00:00Z',
    status: 'ok',
    ...overrides,
  };
}

/** Create an error capture with the given overrides */
export function createErrorCapture(overrides: Partial<DiagCapture> & { error: string }): DiagCapture {
  return {
    source: 'peer-1',
    base_url: 'http://10.0.0.1:8080',
    capture_started_at: '2026-06-18T12:00:00Z',
    status: 'error',
    error: 'connection refused',
    ...overrides,
  };
}

/** Create a network_diag object with the given overrides */
export function createNetworkDiag(overrides: Partial<NetworkDiagData> = {}): NetworkDiagData {
  return {
    started_at: '2026-06-18T12:00:00Z',
    status: 'ok',
    underlay_tcp: [],
    ...overrides,
  };
}

/** Create an underlay TCP socket with the given overrides */
export function createTcpSocket(overrides: Partial<TcpSocketDiagData> = {}): TcpSocketDiagData {
  return {
    name: 'xray',
    state: 'ESTAB',
    local: 'redacted:443',
    remote: 'redacted:443',
    rtt_ms: 123.4,
    rto_ms: 456,
    retransmits: 7,
    unacked: 3,
    cwnd: 10,
    status: 'ok',
    ...overrides,
  };
}

/** Create a spike response with an ok capture and optional network_diag */
export function spikeResponseWithOkCapture(options: {
  source?: string;
  latency_ms?: number;
  duration_ms?: number;
  network_diag?: NetworkDiagData;
} = {}): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpike({
    latency_ms: options.latency_ms ?? 1234,
    captures: [createOkCapture({
      source: options.source ?? 'tovarisch-peer',
      duration_ms: options.duration_ms,
      network_diag: options.network_diag,
    })],
  }));
}

/** Create a spike response with an error capture */
export function spikeResponseWithErrorCapture(options: {
  error: string;
  truncated?: boolean;
}): SpikeResponseWithCaptures {
  const error = options.truncated ? 'A'.repeat(100) : options.error;
  return createSpikeResponse(createSpike({
    captures: [createErrorCapture({ error })],
  }));
}

/** Create a spike response with a suppressed capture and network_diag */
export function spikeResponseWithSuppressedCapture(options: {
  latency_ms?: number;
  network_diag?: NetworkDiagData;
} = {}): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpike({
    latency_ms: options.latency_ms ?? 800,
    captures: [{
      source: 'peer-1',
      base_url: 'http://10.0.0.1:8080',
      capture_started_at: '2026-06-18T12:00:00Z',
      status: 'ok',
      suppressed_by_cooldown: true,
      network_diag: options.network_diag ?? createNetworkDiag({
        underlay_tcp: [createTcpSocket({ rtt_ms: 50.0, rto_ms: 200, retransmits: 0 })],
      }),
    }],
  }));
}

/** Create a spike response with an XSS payload in the capture source */
export function spikeResponseWithXssSource(xssPayload: string): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpike({
    captures: [createOkCapture({ source: xssPayload })],
  }));
}

/** Create a spike response with an XSS payload in the error message */
export function spikeResponseWithXssError(xssPayload: string): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpike({
    captures: [createErrorCapture({ error: xssPayload })],
  }));
}

/** Create a spike response with an XSS payload in the TCP socket name */
export function spikeResponseWithXssSocket(xssPayload: string): SpikeResponseWithCaptures {
  return createSpikeResponse(createSpike({
    captures: [createOkCapture({
      network_diag: createNetworkDiag({
        underlay_tcp: [createTcpSocket({ name: xssPayload })],
      }),
    })],
  }));
}

/** Create a full xray TCP spike response (used by tcp.test.ts) */
export function createXrayTcpSpikeResponse(): SpikeResponseWithCaptures {
  return spikeResponseWithOkCapture({
    source: 'peer-1',
    network_diag: createNetworkDiag({
      underlay_tcp: [createTcpSocket()],
    }),
  });
}
