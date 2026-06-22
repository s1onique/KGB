// API client module for UVB-76

export interface Target {
  id: string;
  name: string;
  base_url: string;
}

export interface TargetSnapshot {
  target_id: string;
  scraped_at: string;
  reachable: boolean;
  status?: string;
  peer_version?: string;
  node_id?: string;
  checks?: CheckResult[];
  error?: string;
  // RSS from tovarisch runtime telemetry (VmRSS in KiB, null on non-Linux)
  peer_rss_kib?: number | null;
}

export interface CheckResult {
  name: string;
  status: string;
  detail: string;
}

export interface LatencySummary {
  target_id: string;
  sample_count: number;
  error_count: number;
  min_latency_ms: number;
  max_latency_ms: number;
  avg_latency_ms: number;
  median_latency_ms: number;
  p50_latency_ms?: number;
  p90_latency_ms?: number;
  p95_latency_ms?: number;
  p99_latency_ms?: number;
}

// TargetLatencyResponse includes both HTTP and ICMP latency for a target
export interface TargetLatencyResponse {
  target_id: string;
  http?: LatencySummary;
  icmp?: LatencySummary;
}

export interface LatencySeries {
  target_id: string;
  probe_kind: string;
  probe_url: string;
  interval_seconds: number;
  query_range_seconds: number;       // requested range in API call
  range_seconds: number;            // effective range (clamped to retained)
  step_seconds: number;
  window_seconds: number;
  retained_range_seconds: number;
  sample_count: number;             // DEPRECATED: use retained_sample_count
  retained_sample_count: number;    // actual samples currently in buffer
  retained_sample_capacity: number; // max samples buffer can hold
  returned_point_count: number;     // number of chart points returned
  oldest_sample_ts?: string;
  newest_sample_ts?: string;
  points: PercentilePoint[];
}

export interface PercentilePoint {
  ts: string;
  sample_count: number;
  error_count: number;
  p50_ms?: number;
  p90_ms?: number;
  p95_ms?: number;
  p99_ms?: number;
}

// SpikeSample represents a single latency sample captured around a spike event.
export interface SpikeSample {
  ts: string;
  latency_ms: number;
  ok: boolean;
}

// SpikeThresholds documents which thresholds were configured when spike was detected.
export interface SpikeThresholds {
  warning_ms: number;
  critical_ms: number;
  relative_multiplier: number;
}

// SpikeEvent represents a detected latency spike event for evidence collection.
export interface SpikeEvent {
  event_id: string;
  target_id: string;
  kind: string;              // "http" or "icmp"
  severity: string;          // "warning" or "critical"
  sample_ts: string;         // timestamp of the spike sample
  latency_ms: number;        // the spike latency value
  rolling_median_ms: number; // median before spike
  reasons: string[];         // why this was flagged as spike
  thresholds: SpikeThresholds;
  previous_samples: SpikeSample[];
  scheduler_delay_ms?: number;
  http_status?: number;
  probe_error?: string;
  collected_at: string;      // when spike was recorded
}

// SpikeResponse represents the API response for spike events.
export interface SpikeResponse {
  spikes: SpikeEvent[];
  count: number;
}

// DiagCaptureStatus represents the status of a diagnostic capture.
export type DiagCaptureStatus = 'ok' | 'unavailable' | 'timeout' | 'error' | 'disabled' | 'no_peer_mapping';

// CaptureCooldownInfo holds metadata about why a spike was suppressed by cooldown.
// This provides auditable context for UI display.
export interface CaptureCooldownInfo {
  // Scope indicates the cooldown scope: "global", "per_target", "per_probe", or "per_target_and_probe".
  scope: string;
  // LastSuccessfulCaptureAt is the timestamp of the successful capture that started the cooldown.
  last_successful_capture_at?: string;
  // NextCaptureEligibleAt is when the next capture will be eligible.
  next_capture_eligible_at?: string;
  // RemainingCooldownMs is the remaining cooldown in milliseconds.
  remaining_cooldown_ms?: number;
  // CooldownKey is the key used for cooldown state (typically peer/source name).
  cooldown_key?: string;
  // DecisionNowAt is the timestamp when the cooldown decision was made.
  decision_now_at?: string;
  // AnchorVisible indicates whether the successful capture anchor is visible in the current response scope.
  // - true: anchor spike is retained and visible in current API response
  // - false: anchor spike is not visible (outside filter window, evicted, or suppressed)
  anchor_visible: boolean;
  // AnchorVisibilityReason explains why anchor_visible is false.
  // Empty when anchor_visible is true.
  // Values: "retained_visible", "outside_filter_window", "evicted_from_retention", "suppressed_cooldown"
  anchor_visibility_reason?: string;
  // SkippedAttemptUpdatesCooldown documents the cooldown semantics.
  skipped_attempt_updates_cooldown?: boolean;
  // CooldownSeconds is the configured cooldown duration.
  cooldown_seconds?: number;
  // AnchorCaptureID is the event ID of the anchor capture (for lookup).
  anchor_capture_id?: string;
  // AnchorTargetID is the target ID of the anchor capture.
  anchor_target_id?: string;
  // AnchorProbeKind is the probe kind of the anchor capture.
  anchor_probe_kind?: string;
  // SuppressedProbeKind is the probe kind of the spike that was suppressed by cooldown.
  suppressed_probe_kind?: string;
  // IsCrossProbeSuppression indicates the suppressed spike's probe kind differs from
  // the anchor capture's probe kind.
  is_cross_probe_suppression?: boolean;
}

// TcpSocketDiagData represents TCP socket diagnostic data from tovarisch.
export interface TcpSocketDiagData {
  name: string;
  state: string;
  local: string;
  remote: string;
  rtt_ms?: number;
  rttvar_ms?: number;
  rto_ms?: number;
  retransmits?: number;
  unacked?: number;
  cwnd?: number;
  send_queue_bytes?: number;
  recv_queue_bytes?: number;
  status: string;
}

// NetworkDiagData represents network diagnostic data from tovarisch.
export interface NetworkDiagData {
  started_at: string;
  status: string;
  wireguard?: {
    status: string;
    interfaces: Array<{
      name: string;
      status: string;
      peers: Array<{
        public_key: string;
        endpoint: string;
        allowed_ips: string;
        latest_handshake_at?: string;
        latest_handshake_age_seconds?: number;
        transfer_rx_bytes: number;
        transfer_tx_bytes: number;
      }>;
    }>;
  };
  interfaces?: Array<{
    name: string;
    operstate: string;
    carrier?: boolean;
    rx_bytes: number;
    tx_bytes: number;
    rx_packets: number;
    tx_packets: number;
    rx_errors: number;
    tx_errors: number;
    rx_dropped: number;
    tx_dropped: number;
  }>;
  routes?: Array<{
    target: string;
    interface: string;
    source: string;
    gateway?: string;
    status: string;
  }>;
  underlay_tcp: TcpSocketDiagData[];
  events?: Array<{
    ts: string;
    severity: string;
    source: string;
    message: string;
    fields?: string;
  }>;
}

// DiagCapture represents a diagnostic capture attached to a spike event.
export interface DiagCapture {
  source: string;
  base_url: string;
  capture_started_at: string;
  capture_finished_at?: string;
  duration_ms?: number;
  status: DiagCaptureStatus;
  error?: string;
  network_diag?: NetworkDiagData;
  suppressed_by_cooldown?: boolean;
  referenced_capture_id?: string;
  // Cooldown metadata if this capture was skipped due to cooldown.
  cooldown_info?: CaptureCooldownInfo;
  // Explicit capture status from backend (overrides legacy fields).
  capture_status?: string;
  // TCP absence events explaining why underlay_tcp was empty.
  tcp_absence_events?: TcpAbsenceEvent[];
}

// TcpAbsenceEvent explains why TCP diagnostics were absent from a successful capture.
export interface TcpAbsenceEvent {
  // Machine-readable reason code from TcpAbsenceReason enum.
  // Values: no_matching_socket, socket_closed_before_capture, command_failed,
  // not_configured, permission_denied, target_not_tcp, target_mapping_missing,
  // parse_failed, unsupported_platform
  reason_code: string;
  // Source indicates the diagnostic component that generated this event.
  source: string;
  // Expected peer/endpoint that was expected to match (if known).
  expected_peer?: string;
  // Port that was expected to match (if known).
  expected_port?: number;
  // Probe kind that triggered the capture (http/icmp).
  probe_kind?: string;
  // Tool/command that was attempted (e.g., "ss", "tcpdiag").
  command_tool?: string;
  // Number of sockets found but did not match filters.
  raw_match_count?: number;
  // Network namespace scope (if known).
  namespace?: string;
  // Additional context about the absence.
  detail?: string;
}

// SpikeEventWithCaptures represents a spike event with diagnostic captures.
export interface SpikeEventWithCaptures extends SpikeEvent {
  captures?: DiagCapture[];
}

// SpikeRetentionStats holds spike retention metadata for UI display.
export interface SpikeRetentionStats {
  retained_spike_count: number;
  visible_spike_count: number;
  protected_capture_count: number;
  purge_eligible_count: number;
  max_uncaptured_spikes: number;
}

// SpikeResponseWithCaptures represents the API response for spike events with diagnostic captures.
export interface SpikeResponseWithCaptures {
  spikes: SpikeEventWithCaptures[];
  count: number;
  retention: SpikeRetentionStats;
}

// AnchorStatus represents the availability status of an anchor capture.
export type AnchorStatus =
  | 'available'
  | 'artifact_missing'
  | 'metadata_only'
  | 'not_an_anchor_capture'
  | 'not_found';

// AnchorDegradationReason explains why degraded=true.
export type AnchorDegradationReason =
  | 'artifact_purged'
  | 'spike_event_evicted'
  | 'missing_provenance'
  | 'partial_metadata';

// AnchorCaptureResponse represents the response from the anchor capture lookup endpoint.
export interface AnchorCaptureResponse {
  // Capture is the full capture record if available.
  capture?: DiagCapture;
  // SpikeEvent is the spike event associated with the capture if available.
  spike_event?: SpikeEvent;
  // Anchor is the cooldown anchor metadata that justified the suppression.
  anchor?: CaptureCooldownAnchor;
  // Status indicates the availability state of the anchor.
  status: AnchorStatus;
  // Message provides human-readable context about availability.
  message?: string;
  // Degraded indicates the response has reduced information (artifact or spike missing).
  degraded: boolean;
  // DegradationReason explains why degraded=true.
  degradation_reason?: AnchorDegradationReason;
}

// CaptureCooldownAnchor represents the provenance of the successful capture that started a cooldown.
export interface CaptureCooldownAnchor {
  anchor_capture_id?: string;
  anchor_target_id?: string;
  anchor_probe_kind?: string;
  anchor_source?: string;
  anchor_created_at?: string;
  anchor_completed_at?: string;
  anchor_updated_by_status?: string;
  created_from?: string;
  is_warmup_anchor?: boolean;
}

// CooldownAnchorResponse represents the response from the peer cooldown anchor endpoint.
export interface CooldownAnchorResponse {
  anchor: CaptureCooldownAnchor;
  capture_exists: boolean;
  degraded: boolean;
  checked_at: string;
}

export interface AuthCheckResponse {
  authenticated: boolean;
  username?: string;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  success: boolean;
  error?: string;
}

// ServerStatus represents runtime status of the UVB-76 server.
export interface ServerStatus {
  started_at: string; // RFC3339 timestamp
}

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = '') {
    this.baseUrl = baseUrl;
  }

  private async fetch<T>(path: string, options?: RequestInit): Promise<T> {
    const response = await fetch(this.baseUrl + path, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options?.headers,
      },
    });
    if (!response.ok) {
      throw new Error(`API error: ${response.status}`);
    }
    return response.json();
  }

  async authCheck(): Promise<AuthCheckResponse> {
    return this.fetch<AuthCheckResponse>('/api/v1/auth/check');
  }

  async login(username: string, password: string): Promise<LoginResponse> {
    return this.fetch<LoginResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });
  }

  async logout(): Promise<void> {
    await this.fetch('/api/v1/auth/logout', { method: 'POST' });
  }

  async getTargets(): Promise<Target[]> {
    return this.fetch<Target[]>('/api/v1/targets');
  }

  async getTargetSnapshot(targetId: string): Promise<TargetSnapshot> {
    return this.fetch<TargetSnapshot>(`/api/v1/targets/${targetId}/snapshot`);
  }

  async getTargetLatency(targetId: string): Promise<TargetLatencyResponse> {
    return this.fetch<TargetLatencyResponse>(`/api/v1/targets/${targetId}/latency`);
  }

  async getLatencySeries(
    targetId: string,
    rangeSeconds: number = 3600,
    stepSeconds: number = 60,
    windowSeconds: number = 300
  ): Promise<LatencySeries> {
    return this.fetch<LatencySeries>(
      `/api/v1/latency/series?target_id=${targetId}&range_seconds=${rangeSeconds}&step_seconds=${stepSeconds}&window_seconds=${windowSeconds}`
    );
  }

  async getHTTPLatencySeries(
    targetId: string,
    rangeSeconds: number = 3600,
    stepSeconds: number = 60,
    windowSeconds: number = 300
  ): Promise<LatencySeries> {
    return this.fetch<LatencySeries>(
      `/api/v1/latency/series?target_id=${targetId}&probe_kind=http&range_seconds=${rangeSeconds}&step_seconds=${stepSeconds}&window_seconds=${windowSeconds}`
    );
  }

  async getICMPLatencySeries(
    targetId: string,
    rangeSeconds: number = 300,
    stepSeconds: number = 5,
    windowSeconds: number = 60
  ): Promise<LatencySeries> {
    return this.fetch<LatencySeries>(
      `/api/v1/latency/series?target_id=${targetId}&probe_kind=icmp&range_seconds=${rangeSeconds}&step_seconds=${stepSeconds}&window_seconds=${windowSeconds}`
    );
  }

  async getStatus(): Promise<ServerStatus> {
    return this.fetch<ServerStatus>('/api/v1/status');
  }

  async getLatencySpikes(
    targetId: string,
    kind: 'http' | 'icmp' = 'http',
    limit: number = 20
  ): Promise<SpikeResponse> {
    return this.fetch<SpikeResponse>(
      `/api/v1/latency/spikes?target_id=${encodeURIComponent(targetId)}&kind=${kind}&limit=${limit}`
    );
  }

  async getLatencySpikesWithCaptures(
    targetId: string,
    kind: 'http' | 'icmp' = 'http',
    limit: number = 10
  ): Promise<SpikeResponseWithCaptures> {
    return this.fetch<SpikeResponseWithCaptures>(
      `/api/v1/latency/spikes?target_id=${encodeURIComponent(targetId)}&kind=${kind}&include_captures=true&limit=${limit}`
    );
  }

  /**
   * Get the anchor capture details for a skipped cooldown spike.
   * This allows operators to inspect the prior successful capture that justified suppression,
   * even when the anchor spike is outside the current visible window.
   */
  async getAnchorCapture(captureId: string): Promise<AnchorCaptureResponse> {
    return this.fetch<AnchorCaptureResponse>(`/api/v1/captures/${encodeURIComponent(captureId)}/anchor`);
  }

  /**
   * Get the cooldown anchor for a specific peer.
   * This allows operators to see what capture started the current cooldown for a peer.
   */
  async getCooldownAnchorForPeer(peerName: string): Promise<CooldownAnchorResponse> {
    return this.fetch<CooldownAnchorResponse>(`/api/v1/diagnostics/cooldown/anchors/${encodeURIComponent(peerName)}`);
  }
}

export const api = new ApiClient();
