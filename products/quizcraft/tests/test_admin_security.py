import pytest
from fastapi.testclient import TestClient
from starlette.websockets import WebSocketDisconnect

import db_storage
import server
from scripts.admin_api_security import resolve_admin_api_base_url


def test_feedback_dashboard_stays_public(monkeypatch):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    monkeypatch.setattr(server, "db_runtime_enabled", lambda: False)

    client = TestClient(server.app)

    response = client.get("/api/feedback/dashboard")

    assert response.status_code == 200
    assert "pending_items" in response.json()


def test_feedback_status_update_requires_admin_token(monkeypatch):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")

    client = TestClient(server.app)

    response = client.patch(
        "/api/feedback/1/status",
        json={"status": "resolved"},
    )

    assert response.status_code == 403


def test_admin_session_login_sets_httponly_cookie_and_authorizes_requests(
    tmp_path,
    monkeypatch,
):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    monkeypatch.setattr(server, "EXPORT_DIR", str(tmp_path))

    client = TestClient(server.app)

    rejected = client.post(
        "/api/admin/session",
        headers={"X-Admin-Token": "wrong-token"},
    )
    assert rejected.status_code == 403

    login = client.post(
        "/api/admin/session",
        headers={"X-Admin-Token": "test-token"},
    )
    assert login.status_code == 200
    assert login.json()["authenticated"] is True
    set_cookie = login.headers["set-cookie"].lower()
    assert "httponly" in set_cookie
    assert "samesite=strict" in set_cookie

    status = client.get("/api/admin/session")
    assert status.json() == {"authenticated": True}

    protected = client.post(
        "/api/extract/export",
        json={
            "name": "session-test",
            "questions": [
                {
                    "id": "q1",
                    "type": "single",
                    "content": "1 + 1 = ?",
                    "options": ["1", "2"],
                    "answer": "B",
                }
            ],
        },
    )
    assert protected.status_code == 200

    logout = client.delete("/api/admin/session")
    assert logout.status_code == 200
    assert client.get("/api/admin/session").json() == {"authenticated": False}


def test_admin_session_rejects_tampering_and_expiry(monkeypatch):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    monkeypatch.setenv("ADMIN_SESSION_TTL_SECONDS", "300")

    token = server.create_admin_session_token(now=1_000)

    assert server.is_admin_session_valid(token, now=1_001)
    assert not server.is_admin_session_valid(token + "tampered", now=1_001)
    assert not server.is_admin_session_valid(token, now=1_300)


def test_admin_header_remains_available_for_cli_clients(tmp_path, monkeypatch):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    monkeypatch.setattr(server, "EXPORT_DIR", str(tmp_path))

    response = TestClient(server.app).post(
        "/api/extract/export",
        headers={"X-Admin-Token": "test-token"},
        json={
            "name": "cli-test",
            "questions": [
                {
                    "id": "q1",
                    "type": "single",
                    "content": "1 + 1 = ?",
                    "options": ["1", "2"],
                    "answer": "B",
                }
            ],
        },
    )

    assert response.status_code == 200


def test_analysis_websocket_uses_admin_session_cookie(monkeypatch):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    client = TestClient(server.app)

    with pytest.raises(WebSocketDisconnect) as rejected:
        with client.websocket_connect("/ws/analyze/rejected"):
            pass
    assert rejected.value.code == 1008

    client.post(
        "/api/admin/session",
        headers={"X-Admin-Token": "test-token"},
    )
    with client.websocket_connect("/ws/analyze/accepted") as websocket:
        websocket.send_json({"questions": [], "config": {}})
        assert websocket.receive_json() == {
            "type": "error",
            "error": "没有题目需要解析",
        }


def test_feedback_status_normalization_accepts_archived():
    assert server._normalize_feedback_status("archived") == "archived"
    assert db_storage._normalize_feedback_status("archived") == "archived"


def test_feedback_dashboard_fallback_reports_archived_items(tmp_path, monkeypatch):
    feedback_file = tmp_path / "feedbacks.json"
    feedback_file.write_text(
        """
        [
          {
            "feedback_id": 1,
            "question_index": 8,
            "question_bank": "xigai",
            "suggestion": "needs source check",
            "status": "archived",
            "created_at": "2026-06-29T08:00:00+08:00"
          },
          {
            "feedback_id": 2,
            "question_index": 9,
            "question_bank": "java",
            "suggestion": "fixed",
            "status": "resolved",
            "created_at": "2026-06-29T09:00:00+08:00",
            "resolved_at": "2026-06-29T10:00:00+08:00"
          }
        ]
        """,
        encoding="utf-8",
    )
    monkeypatch.setattr(server, "FEEDBACK_FILE", str(feedback_file))

    dashboard = server.load_feedback_dashboard_fallback()

    assert dashboard["summary"]["pending_total"] == 0
    assert dashboard["summary"]["resolved_total"] == 1
    assert dashboard["summary"]["archived_total"] == 1
    assert dashboard["archived_items"][0]["feedback_id"] == 1


def test_admin_api_base_url_requires_explicit_value(monkeypatch):
    monkeypatch.delenv("QUIZCRAFT_API_BASE_URL", raising=False)

    with pytest.raises(SystemExit, match="QUIZCRAFT_API_BASE_URL"):
        resolve_admin_api_base_url(None)


def test_admin_api_base_url_rejects_public_http():
    with pytest.raises(SystemExit, match="HTTPS"):
        resolve_admin_api_base_url("http://8.146.200.82/api")


@pytest.mark.parametrize(
    "url",
    [
        "https://superhuazai.me/api",
        "http://127.0.0.1:10086/api",
        "http://localhost:10086/api",
    ],
)
def test_admin_api_base_url_allows_https_and_local_http(url):
    assert resolve_admin_api_base_url(url) == url.rstrip("/")
