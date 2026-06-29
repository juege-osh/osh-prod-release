#!/usr/bin/env python3
"""OSH Deploy Platform — functional + data diff analyzer with rule-based AI verdict."""
from __future__ import annotations

import json
import re
from http.server import BaseHTTPRequestHandler, HTTPServer
from typing import Any

PORT = 8766


def _lower(s: str) -> str:
    return (s or "").lower()


def _collect_expected_text(items: list[dict[str, Any]]) -> str:
    parts: list[str] = []
    for it in items or []:
        for key in ("label", "ref", "payload", "action", "expected_impact", "data_impact", "test_plan", "title"):
            val = it.get(key)
            if val:
                parts.append(str(val))
    return " ".join(parts)


def _flatten_diff_rows(component_diffs: list[Any]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for raw in component_diffs or []:
        if isinstance(raw, str):
            try:
                diff = json.loads(raw)
            except json.JSONDecodeError:
                continue
        else:
            diff = raw
        if not isinstance(diff, dict):
            continue
        comp = diff.get("component") or diff.get("kind") or "unknown"
        summary = diff.get("summary") or {}
        for name in diff.get("added") or []:
            rows.append({
                "component": comp,
                "change_type": "added",
                "name": str(name),
                "detail": summary,
            })
        for name in diff.get("removed") or []:
            rows.append({
                "component": comp,
                "change_type": "removed",
                "name": str(name),
                "detail": summary,
            })
        for name in diff.get("modified") or []:
            rows.append({
                "component": comp,
                "change_type": "modified",
                "name": str(name),
                "detail": summary,
            })
    return rows


def _functional_summary(functional: list[dict[str, Any]]) -> dict[str, Any]:
    cases = functional or []
    passed = sum(1 for c in cases if c.get("passed"))
    return {
        "total": len(cases),
        "passed": passed,
        "failed": len(cases) - passed,
        "cases": cases,
    }


def _match_expected(name: str, component: str, expected_text: str) -> bool:
    hay = _lower(expected_text)
    if not hay:
        return True
    needle = _lower(name)
    if needle and needle in hay:
        return True
    tokens = re.split(r"[^a-zA-Z0-9_]+", needle)
    tokens = [t for t in tokens if len(t) >= 4]
    if any(t in hay for t in tokens):
        return True
    return _lower(component) in hay


def judge(body: dict[str, Any]) -> tuple[dict[str, Any], str, bool]:
    functional = body.get("functional") or []
    expected_items = body.get("expected_items") or []
    legacy_expected = body.get("expected") or []
    data_impact = body.get("data_impact") or []
    test_plan = body.get("test_plan") or []

    if not expected_items and legacy_expected:
        expected_items = [{"expected_impact": x} for x in legacy_expected]
    for i, txt in enumerate(data_impact):
        if i < len(expected_items):
            expected_items[i]["data_impact"] = txt
        else:
            expected_items.append({"data_impact": txt})
    for i, txt in enumerate(test_plan):
        if i < len(expected_items):
            expected_items[i]["test_plan"] = txt
        else:
            expected_items.append({"test_plan": txt})

    expected_text = _collect_expected_text(expected_items)
    component_diffs = body.get("component_diffs") or []
    rows = _flatten_diff_rows(component_diffs)
    func_summary = _functional_summary(functional)

    mismatches: list[str] = []
    for row in rows:
        if row["change_type"] != "added":
            continue
        if not _match_expected(row["name"], str(row["component"]), expected_text):
            mismatches.append(
                f"{row['component']} 新增 {row['name']} 未在上线说明/用例中找到对应描述"
            )

    func_failed = [c for c in functional if not c.get("passed")]
    if func_failed:
        for c in func_failed:
            mismatches.append(f"功能探测失败: {c.get('name')} — {c.get('detail', '')[:120]}")

    added_count = sum(1 for r in rows if r["change_type"] == "added")
    removed_count = sum(1 for r in rows if r["change_type"] == "removed")
    modified_count = sum(1 for r in rows if r["change_type"] == "modified")

    report = {
        "release_id": body.get("release_id"),
        "functional": func_summary,
        "data_changes": {
            "rows": rows,
            "summary": {
                "added_count": added_count,
                "removed_count": removed_count,
                "modified_count": modified_count,
            },
        },
        "expected_items": expected_items,
        "component_diffs": component_diffs,
        "mismatches": mismatches,
    }

    if mismatches:
        verdict = "不一致 — " + "; ".join(mismatches[:5])
        if len(mismatches) > 5:
            verdict += f" …等 {len(mismatches)} 项"
        return report, verdict, False

    if added_count == 0 and removed_count == 0 and modified_count == 0:
        verdict = "一致 — 未检测到数据变更（或变更已在预期内）"
    else:
        verdict = f"一致 — 功能 {func_summary['passed']}/{func_summary['total']} 通过；数据变更 added={added_count} removed={removed_count} modified={modified_count} 与上线说明匹配"
    return report, verdict, True


class Handler(BaseHTTPRequestHandler):
    def do_POST(self) -> None:
        if self.path != "/api/diff-and-judge":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length) or b"{}")
        report, verdict, passed = judge(body)
        self._json(200, {
            "data_diff": json.dumps(report, ensure_ascii=False),
            "verdict": verdict,
            "passed": passed,
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
