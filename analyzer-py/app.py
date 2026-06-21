#!/usr/bin/env python3
"""OSH Deploy Platform — data diff + AI verdict sidecar (P1 stub)."""
from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = 8766


class Handler(BaseHTTPRequestHandler):
    def do_POST(self) -> None:
        if self.path != "/api/diff-and-judge":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length) or b"{}")
        expected = body.get("expected") or []
        data_diff = {
            "release_id": body.get("release_id"),
            "tables": [
                {"table": "mock_table", "before": 0, "after": 0, "added": 0, "removed": 0, "modified": 0}
            ],
        }
        verdict = "consistent (stub)"
        if expected:
            verdict += f" — expected: {expected[0][:80]}"
        self._json(200, {
            "data_diff": json.dumps(data_diff, ensure_ascii=False),
            "verdict": verdict,
            "passed": True,
        })

    def do_GET(self) -> None:
        if self.path == "/api/health":
            self._json(200, {"status": "ok"})
            return
        self.send_error(404)

    def _json(self, code: int, obj: dict) -> None:
        raw = json.dumps(obj, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, fmt: str, *args) -> None:
        pass


if __name__ == "__main__":
    print(f"analyzer listening :{PORT}")
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
