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
}

export const api = new ApiClient();
