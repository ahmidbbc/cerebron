"""
Cerebron Python SDK.
Supports direct HTTP calls to the Cerebron API with typed dataclasses.
MCP consumption is handled via the standard MCP client using the /mcp endpoint.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any, Optional
from urllib import request as urllib_request
from urllib.error import HTTPError
from urllib.parse import urlencode


class CerebronError(Exception):
    """Raised when the Cerebron API returns an error response."""

    def __init__(self, status_code: int, message: str) -> None:
        super().__init__(f"Cerebron API error {status_code}: {message}")
        self.status_code = status_code


@dataclass
class Signal:
    source: str
    service: str
    type: str
    summary: str
    severity: str
    timestamp: str
    count: int = 0
    metadata: dict[str, str] = field(default_factory=dict)


@dataclass
class SignalGroup:
    service: str
    window_start: str
    window_end: str
    source_count: int
    highest_severity: str
    summary: str
    signals: list[Signal] = field(default_factory=list)


@dataclass
class Deployment:
    id: str
    source: str
    service: str
    environment: str
    version: str
    commit: str
    author: str
    branch: str
    status: str
    started_at: str
    finished_at: str
    url: str


@dataclass
class DeploymentContext:
    recent_deployments: list[Deployment] = field(default_factory=list)
    suspect_deployments: list[Deployment] = field(default_factory=list)
    rollback_candidates: list[Deployment] = field(default_factory=list)


@dataclass
class IncidentAnalysis:
    service: str
    time_range: str
    model_version: str
    groups: list[SignalGroup]
    summary: str
    confidence: float
    deployment_context: Optional[DeploymentContext] = None


@dataclass
class StoredIncident:
    id: str
    fingerprint: str
    service: str
    analysis: IncidentAnalysis
    created_at: str
    recurrence_count: int


@dataclass
class SimilarIncidentsResponse:
    related: list[StoredIncident]
    exact_match: Optional[StoredIncident] = None


@dataclass
class ServiceTrend:
    service: str
    incident_count: int
    recurrence_total: int
    frequency_per_day: float
    dominant_severity: str
    severity_trend: str
    first_seen: str
    last_seen: str


@dataclass
class IncidentTrends:
    services: list[ServiceTrend]
    degrading_count: int
    stable_count: int
    improving_count: int
    observation_days: float


@dataclass
class DependencyEdge:
    source: str
    target: str


@dataclass
class ServiceDependencies:
    service: str
    upstreams: list[str]
    downstreams: list[str]
    blast_radius: list[str]
    all_edges: list[DependencyEdge]


@dataclass
class CausalHint:
    rule: str
    confidence: float
    evidence: str


@dataclass
class CausalAnalysis:
    service: str
    hints: list[CausalHint]


@dataclass
class RecentDeploymentsResponse:
    deployments: list[Deployment]


@dataclass
class IncidentHistoryResponse:
    incidents: list[StoredIncident]
    total: int


class CerebronClient:
    """HTTP client for the Cerebron API. All methods return typed dataclasses."""

    def __init__(self, base_url: str, timeout: float = 30.0) -> None:
        self._base_url = base_url.rstrip("/")
        self._timeout = timeout

    @property
    def mcp_endpoint(self) -> str:
        """MCP endpoint URL — pass to any standard MCP client."""
        return f"{self._base_url}/mcp"

    def analyze_incident(
        self,
        service: str | None = None,
        services: list[str] | None = None,
        time_range: str | None = None,
    ) -> IncidentAnalysis:
        body: dict[str, Any] = {}
        if service:
            body["service"] = service
        if services:
            body["services"] = services
        if time_range:
            body["time_range"] = time_range
        data = self._post("/api/v1/incidents/analyze", body)
        return _parse_incident_analysis(data)

    def find_similar_incidents(
        self,
        fingerprint: str = "",
        service: str = "",
        limit: int = 10,
    ) -> SimilarIncidentsResponse:
        params: dict[str, Any] = {"limit": limit}
        if fingerprint:
            params["fingerprint"] = fingerprint
        if service:
            params["service"] = service
        data = self._get("/api/v1/incidents/similar", params)
        related = [_parse_stored_incident(i) for i in data.get("related", [])]
        exact = _parse_stored_incident(data["exact_match"]) if data.get("exact_match") else None
        return SimilarIncidentsResponse(related=related, exact_match=exact)

    def detect_incident_trends(self, service: str = "") -> IncidentTrends:
        params = {"service": service} if service else {}
        data = self._get("/api/v1/incidents/trends", params)
        return IncidentTrends(
            services=[_parse_service_trend(s) for s in data.get("services", [])],
            degrading_count=data.get("degrading_count", 0),
            stable_count=data.get("stable_count", 0),
            improving_count=data.get("improving_count", 0),
            observation_days=data.get("observation_days", 0.0),
        )

    def get_service_dependencies(self, service: str) -> ServiceDependencies:
        data = self._get("/api/v1/services/dependencies", {"service": service})
        return ServiceDependencies(
            service=data.get("service", ""),
            upstreams=data.get("upstreams", []),
            downstreams=data.get("downstreams", []),
            blast_radius=data.get("blast_radius", []),
            all_edges=[DependencyEdge(**e) for e in data.get("all_edges", [])],
        )

    def analyze_causal_hints(self, analysis: IncidentAnalysis) -> CausalAnalysis:
        data = self._post("/api/v1/incidents/causal-hints", _incident_analysis_to_dict(analysis))
        return CausalAnalysis(
            service=data.get("service", ""),
            hints=[CausalHint(**h) for h in data.get("hints", [])],
        )

    def get_recent_deployments(
        self,
        service: str,
        environment: str = "",
        limit: int = 20,
    ) -> RecentDeploymentsResponse:
        params: dict[str, Any] = {"service": service, "limit": limit}
        if environment:
            params["environment"] = environment
        data = self._get("/api/v1/deployments", params)
        return RecentDeploymentsResponse(
            deployments=[_parse_deployment(d) for d in data.get("deployments", [])]
        )

    def get_incident_history(self, service: str, limit: int = 50) -> IncidentHistoryResponse:
        data = self._get("/api/v1/incidents/history", {"service": service, "limit": limit})
        return IncidentHistoryResponse(
            incidents=[_parse_stored_incident(i) for i in data.get("incidents", [])],
            total=data.get("total", 0),
        )

    def _get(self, path: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        url = self._base_url + path
        if params:
            url += "?" + urlencode(params)
        return self._request("GET", url, None)

    def _post(self, path: str, body: dict[str, Any]) -> dict[str, Any]:
        return self._request("POST", self._base_url + path, body)

    def _request(self, method: str, url: str, body: dict[str, Any] | None) -> dict[str, Any]:
        data = json.dumps(body).encode() if body is not None else None
        req = urllib_request.Request(
            url,
            data=data,
            method=method,
            headers={"Content-Type": "application/json"},
        )
        try:
            with urllib_request.urlopen(req, timeout=self._timeout) as resp:
                return json.loads(resp.read())
        except HTTPError as exc:
            body_bytes = exc.read()
            message = f"HTTP {exc.code}"
            try:
                err = json.loads(body_bytes)
                if err.get("message"):
                    message = err["message"]
            except (json.JSONDecodeError, AttributeError):
                pass
            raise CerebronError(exc.code, message) from exc


def _parse_deployment(d: dict[str, Any]) -> Deployment:
    return Deployment(
        id=d.get("id", ""),
        source=d.get("source", ""),
        service=d.get("service", ""),
        environment=d.get("environment", ""),
        version=d.get("version", ""),
        commit=d.get("commit", ""),
        author=d.get("author", ""),
        branch=d.get("branch", ""),
        status=d.get("status", ""),
        started_at=d.get("started_at", ""),
        finished_at=d.get("finished_at", ""),
        url=d.get("url", ""),
    )


def _parse_signal(s: dict[str, Any]) -> Signal:
    return Signal(
        source=s.get("source", ""),
        service=s.get("service", ""),
        type=s.get("type", ""),
        summary=s.get("summary", ""),
        severity=s.get("severity", ""),
        timestamp=s.get("timestamp", ""),
        count=s.get("count", 0),
        metadata=s.get("metadata", {}),
    )


def _parse_signal_group(g: dict[str, Any]) -> SignalGroup:
    return SignalGroup(
        service=g.get("service", ""),
        window_start=g.get("window_start", ""),
        window_end=g.get("window_end", ""),
        source_count=g.get("source_count", 0),
        highest_severity=g.get("highest_severity", ""),
        summary=g.get("summary", ""),
        signals=[_parse_signal(s) for s in g.get("signals", [])],
    )


def _parse_incident_analysis(d: dict[str, Any]) -> IncidentAnalysis:
    dc = d.get("deployment_context")
    return IncidentAnalysis(
        service=d.get("service", ""),
        time_range=d.get("time_range", ""),
        model_version=d.get("model_version", ""),
        groups=[_parse_signal_group(g) for g in d.get("groups", [])],
        summary=d.get("summary", ""),
        confidence=d.get("confidence", 0.0),
        deployment_context=DeploymentContext(
            recent_deployments=[_parse_deployment(x) for x in dc.get("recent_deployments", [])],
            suspect_deployments=[_parse_deployment(x) for x in dc.get("suspect_deployments", [])],
            rollback_candidates=[_parse_deployment(x) for x in dc.get("rollback_candidates", [])],
        ) if dc else None,
    )


def _parse_stored_incident(d: dict[str, Any]) -> StoredIncident:
    return StoredIncident(
        id=d.get("id", ""),
        fingerprint=d.get("fingerprint", ""),
        service=d.get("service", ""),
        analysis=_parse_incident_analysis(d.get("analysis", {})),
        created_at=d.get("created_at", ""),
        recurrence_count=d.get("recurrence_count", 0),
    )


def _parse_service_trend(s: dict[str, Any]) -> ServiceTrend:
    return ServiceTrend(
        service=s.get("service", ""),
        incident_count=s.get("incident_count", 0),
        recurrence_total=s.get("recurrence_total", 0),
        frequency_per_day=s.get("frequency_per_day", 0.0),
        dominant_severity=s.get("dominant_severity", ""),
        severity_trend=s.get("severity_trend", ""),
        first_seen=s.get("first_seen", ""),
        last_seen=s.get("last_seen", ""),
    )


def _incident_analysis_to_dict(a: IncidentAnalysis) -> dict[str, Any]:
    return {
        "service": a.service,
        "time_range": a.time_range,
        "model_version": a.model_version,
        "groups": [],
        "summary": a.summary,
        "confidence": a.confidence,
    }
