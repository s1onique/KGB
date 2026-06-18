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
  version?: string;
  node_id?: string;
  checks?: CheckResult[];
  error?: string;
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
  range_seconds: number;
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
}

export const api = new ApiClient();
