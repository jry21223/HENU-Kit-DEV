from fastapi.testclient import TestClient

import server


def test_postgres_feedback_failure_is_not_acknowledged_by_json_fallback(monkeypatch):
    monkeypatch.setattr(server, "db_runtime_enabled", lambda: True)
    monkeypatch.setattr(
        server.db_storage,
        "create_feedback",
        lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("database unavailable")),
    )
    monkeypatch.setattr(
        server,
        "save_feedback_fallback",
        lambda **_kwargs: (_ for _ in ()).throw(AssertionError("fallback must not accept a migration-window write")),
    )

    response = TestClient(server.app).post(
        "/api/feedback",
        json={
            "question_index": 1,
            "question_bank": "java",
            "question_id": "q1",
            "suggestion": "please fix",
        },
    )

    assert response.status_code == 503
    assert response.json()["detail"] == "反馈暂时无法保存，请稍后重试"
