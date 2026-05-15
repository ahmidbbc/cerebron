/**
 * Cerebron TypeScript SDK.
 * Supports direct HTTP calls to the Cerebron API with typed responses.
 * MCP consumption is handled via the standard MCP client using the /mcp endpoint.
 */

export interface Signal {
  source: string;
  service: string;
  type: string;
  summary: string;
  severity: "low" | "medium" | "high";
  timestamp: string;
  count?: number;
  metadata?: Record<string, string>;
}

export interface SignalGroup {
  service: string;
  window_start: string;
  window_end: string;
  source_count: number;
  highest_severity: "low" | "medium" | "high";
  summary: string;
  signals: Signal[];
}

export interface Deployment {
  id: string;
  source: string;
  service: string;
  environment: string;
  version: string;
  commit: string;
  author: string;
  branch: string;
  status: "success" | "failure" | "in_progress" | "rolled_back" | "unknown";
  started_at: string;
  finished_at: string;
  url: string;
}

export interface DeploymentContext {
  recent_deployments: Deployment[];
  suspect_deployments: Deployment[];
  rollback_candidates: Deployment[];
}

export interface IncidentAnalysis {
  service: string;
  time_range: string;
  model_version: string;
  groups: SignalGroup[];
  summary: string;
  confidence: number;
  deployment_context?: DeploymentContext;
}

export interface StoredIncident {
  id: string;
  fingerprint: string;
  service: string;
  analysis: IncidentAnalysis;
  created_at: string;
  recurrence_count: number;
}

export interface SimilarIncidentsResponse {
  exact_match?: StoredIncident;
  related: StoredIncident[];
}

export interface ServiceTrend {
  service: string;
  incident_count: number;
  recurrence_total: number;
  frequency_per_day: number;
  dominant_severity: "low" | "medium" | "high";
  severity_trend: "worsening" | "stable" | "improving";
  first_seen: string;
  last_seen: string;
}

export interface IncidentTrends {
  services: ServiceTrend[];
  degrading_count: number;
  stable_count: number;
  improving_count: number;
  observation_days: number;
}

export interface DependencyEdge {
  source: string;
  target: string;
}

export interface ServiceDependencies {
  service: string;
  upstreams: string[];
  downstreams: string[];
  blast_radius: string[];
  all_edges: DependencyEdge[];
}

export interface CausalHint {
  rule: string;
  confidence: number;
  evidence: string;
}

export interface CausalAnalysis {
  service: string;
  hints: CausalHint[];
}

export interface RecentDeploymentsResponse {
  deployments: Deployment[];
}

export interface IncidentHistoryResponse {
  incidents: StoredIncident[];
  total: number;
}

export interface AnalyzeIncidentRequest {
  service?: string;
  services?: string[];
  time_range?: string;
}

export interface CerebronClientOptions {
  baseURL: string;
  timeout?: number;
}

export class CerebronError extends Error {
  constructor(
    public readonly statusCode: number,
    message: string,
  ) {
    super(message);
    this.name = "CerebronError";
  }
}

export class CerebronClient {
  private readonly baseURL: string;
  private readonly timeout: number;

  constructor(options: CerebronClientOptions) {
    this.baseURL = options.baseURL.replace(/\/$/, "");
    this.timeout = options.timeout ?? 30_000;
  }

  async analyzeIncident(req: AnalyzeIncidentRequest): Promise<IncidentAnalysis> {
    return this.post<IncidentAnalysis>("/api/v1/incidents/analyze", req);
  }

  async findSimilarIncidents(
    fingerprint: string,
    service: string,
    limit = 10,
  ): Promise<SimilarIncidentsResponse> {
    const params = new URLSearchParams();
    if (fingerprint) params.set("fingerprint", fingerprint);
    if (service) params.set("service", service);
    params.set("limit", String(limit));
    return this.get<SimilarIncidentsResponse>(`/api/v1/incidents/similar?${params}`);
  }

  async detectIncidentTrends(service?: string): Promise<IncidentTrends> {
    const path = service
      ? `/api/v1/incidents/trends?service=${encodeURIComponent(service)}`
      : "/api/v1/incidents/trends";
    return this.get<IncidentTrends>(path);
  }

  async getServiceDependencies(service: string): Promise<ServiceDependencies> {
    return this.get<ServiceDependencies>(
      `/api/v1/services/dependencies?service=${encodeURIComponent(service)}`,
    );
  }

  async analyzeCausalHints(analysis: IncidentAnalysis): Promise<CausalAnalysis> {
    return this.post<CausalAnalysis>("/api/v1/incidents/causal-hints", analysis);
  }

  async getRecentDeployments(
    service: string,
    environment?: string,
    limit = 20,
  ): Promise<RecentDeploymentsResponse> {
    const params = new URLSearchParams({ service, limit: String(limit) });
    if (environment) params.set("environment", environment);
    return this.get<RecentDeploymentsResponse>(`/api/v1/deployments?${params}`);
  }

  async getIncidentHistory(service: string, limit = 50): Promise<IncidentHistoryResponse> {
    const params = new URLSearchParams({ service, limit: String(limit) });
    return this.get<IncidentHistoryResponse>(`/api/v1/incidents/history?${params}`);
  }

  /** MCP endpoint URL — pass to any standard MCP client. */
  get mcpEndpoint(): string {
    return `${this.baseURL}/mcp`;
  }

  private async get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path, undefined);
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  private async request<T>(method: string, path: string, body: unknown): Promise<T> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const resp = await fetch(`${this.baseURL}${path}`, {
        method,
        headers: { "Content-Type": "application/json" },
        body: body !== undefined ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      if (!resp.ok) {
        let message = `HTTP ${resp.status}`;
        try {
          const err = (await resp.json()) as { message?: string };
          if (err.message) message = err.message;
        } catch {
          // ignore parse error
        }
        throw new CerebronError(resp.status, message);
      }

      return resp.json() as Promise<T>;
    } finally {
      clearTimeout(timer);
    }
  }
}
