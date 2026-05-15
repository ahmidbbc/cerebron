"""Unit tests for the Cerebron Python SDK."""

from __future__ import annotations

import json
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from threading import Thread
from typing import Any

from cerebron import (
    CerebronClient,
    CerebronError,
    IncidentAnalysis,
    IncidentTrends,
    RecentDeploymentsResponse,
    SimilarIncidentsResponse,
    IncidentHistoryResponse,
)


def _start_server(handler: type[BaseHTTPRequestHandler]) -> tuple[HTTPServer, str]:
    srv = HTTPServer(("127.0.0.1", 0), handler)
    port = srv.server_address[1]
    Thread(target=srv.serve_forever, daemon=True).start()
    return srv, f"http://127.0.0.1:{port}"


class _AnalyzeHandler(BaseHTTPRequestHandler):
    def log_message(self, *_: Any) -> None:
        pass

    def do_POST(self) -> None:  # noqa: N802
        if self.path == "/api/v1/incidents/analyze":
            body = json.dumps({"service": "api", "confidence": 0.9, "summary": "ok",
                               "time_range": "1h", "model_version": "v1", "groups": []}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
        else:
            self.send_error(404)


class _ErrorHandler(BaseHTTPRequestHandler):
    def log_message(self, *_: Any) -> None:
        pass

    def do_POST(self) -> None:  # noqa: N802
        body = json.dumps({"message": "internal error"}).encode()
        self.send_response(500)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        self.do_POST()


class _SimilarHandler(BaseHTTPRequestHandler):
    def log_message(self, *_: Any) -> None:
        pass

    def do_GET(self) -> None:  # noqa: N802
        body = json.dumps({"related": [{"id": "i1", "fingerprint": "fp", "service": "api",
                                         "analysis": {"service": "api", "time_range": "",
                                                       "model_version": "v1", "groups": [],
                                                       "summary": "", "confidence": 0.0},
                                         "created_at": "", "recurrence_count": 0}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)


class _TrendsHandler(BaseHTTPRequestHandler):
    def log_message(self, *_: Any) -> None:
        pass

    def do_GET(self) -> None:  # noqa: N802
        body = json.dumps({"services": [], "degrading_count": 3,
                           "stable_count": 1, "improving_count": 0,
                           "observation_days": 7.0}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)


class _DeploymentsHandler(BaseHTTPRequestHandler):
    def log_message(self, *_: Any) -> None:
        pass

    def do_GET(self) -> None:  # noqa: N802
        body = json.dumps({"deployments": [{"id": "d1", "source": "github",
                                             "service": "api", "environment": "prod",
                                             "version": "1.0", "commit": "abc",
                                             "author": "dev", "branch": "main",
                                             "status": "success",
                                             "started_at": "", "finished_at": "",
                                             "url": ""}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)


class _HistoryHandler(BaseHTTPRequestHandler):
    def log_message(self, *_: Any) -> None:
        pass

    def do_GET(self) -> None:  # noqa: N802
        body = json.dumps({"incidents": [], "total": 0}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)


class TestCerebronClient(unittest.TestCase):
    def test_analyze_incident(self) -> None:
        srv, url = _start_server(_AnalyzeHandler)
        try:
            c = CerebronClient(url)
            result = c.analyze_incident(service="api")
            self.assertIsInstance(result, IncidentAnalysis)
            self.assertEqual(result.service, "api")
            self.assertAlmostEqual(result.confidence, 0.9)
        finally:
            srv.shutdown()

    def test_api_error_raises(self) -> None:
        srv, url = _start_server(_ErrorHandler)
        try:
            c = CerebronClient(url)
            with self.assertRaises(CerebronError) as ctx:
                c.analyze_incident(service="x")
            self.assertEqual(ctx.exception.status_code, 500)
        finally:
            srv.shutdown()

    def test_find_similar_incidents(self) -> None:
        srv, url = _start_server(_SimilarHandler)
        try:
            c = CerebronClient(url)
            result = c.find_similar_incidents(service="api")
            self.assertIsInstance(result, SimilarIncidentsResponse)
            self.assertEqual(len(result.related), 1)
        finally:
            srv.shutdown()

    def test_detect_incident_trends(self) -> None:
        srv, url = _start_server(_TrendsHandler)
        try:
            c = CerebronClient(url)
            result = c.detect_incident_trends(service="api")
            self.assertIsInstance(result, IncidentTrends)
            self.assertEqual(result.degrading_count, 3)
        finally:
            srv.shutdown()

    def test_get_recent_deployments(self) -> None:
        srv, url = _start_server(_DeploymentsHandler)
        try:
            c = CerebronClient(url)
            result = c.get_recent_deployments(service="api")
            self.assertIsInstance(result, RecentDeploymentsResponse)
            self.assertEqual(len(result.deployments), 1)
            self.assertEqual(result.deployments[0].id, "d1")
        finally:
            srv.shutdown()

    def test_get_incident_history(self) -> None:
        srv, url = _start_server(_HistoryHandler)
        try:
            c = CerebronClient(url)
            result = c.get_incident_history(service="api")
            self.assertIsInstance(result, IncidentHistoryResponse)
            self.assertEqual(result.total, 0)
        finally:
            srv.shutdown()

    def test_mcp_endpoint(self) -> None:
        c = CerebronClient("http://localhost:8080")
        self.assertEqual(c.mcp_endpoint, "http://localhost:8080/mcp")


if __name__ == "__main__":
    unittest.main()
