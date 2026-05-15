"""
Cerebron context provider for Codex / OpenAI Codex agents.
Fetches incident context and injects it as system context before code generation.

Usage:
    from cerebron_context import enrich_with_incident_context
    context = enrich_with_incident_context("my-service")
    # Pass context as additional system message to the Codex API.
"""

from __future__ import annotations

import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "sdk", "python"))

from cerebron import CerebronClient, CerebronError


def enrich_with_incident_context(
    service: str,
    cerebron_url: str = "http://localhost:8080",
    lookback_hours: int = 1,
) -> str:
    """Return a text block describing recent incidents and deployments for service.

    Designed to be injected as a system message or additional context block
    before sending a Codex completion request.
    """
    client = CerebronClient(cerebron_url)
    lines: list[str] = [f"# Cerebron context for service: {service}"]

    try:
        analysis = client.analyze_incident(service=service, time_range=f"{lookback_hours}h")
        lines.append(f"\n## Incident analysis (confidence: {analysis.confidence:.2f})")
        lines.append(analysis.summary)
        if analysis.groups:
            lines.append(f"Signal groups: {len(analysis.groups)}")
    except CerebronError as exc:
        lines.append(f"\n## Incident analysis unavailable: {exc}")

    try:
        deployments = client.get_recent_deployments(service=service)
        if deployments.deployments:
            lines.append(f"\n## Recent deployments ({len(deployments.deployments)})")
            for d in deployments.deployments[:3]:
                lines.append(f"- {d.version} by {d.author} ({d.status}) at {d.started_at}")
    except CerebronError:
        pass

    try:
        trends = client.detect_incident_trends(service=service)
        for t in trends.services:
            if t.service == service:
                lines.append(f"\n## Trend: {t.severity_trend} ({t.incident_count} incidents)")
                break
    except CerebronError:
        pass

    return "\n".join(lines)


if __name__ == "__main__":
    svc = sys.argv[1] if len(sys.argv) > 1 else "my-service"
    print(enrich_with_incident_context(svc))
