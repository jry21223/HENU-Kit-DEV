from fastapi.testclient import TestClient

import server


def test_legacy_observation_mode_rejects_writes_but_stays_live(monkeypatch):
    monkeypatch.setenv("QUIZCRAFT_READ_ONLY", "1")
    monkeypatch.setattr(
        server,
        "save_feedback_fallback",
        lambda **_kwargs: (_ for _ in ()).throw(AssertionError("read-only request reached a writer")),
    )
    monkeypatch.setattr(server, "sync_question_banks_to_db", lambda: (_ for _ in ()).throw(AssertionError("read-only startup synced banks")))
    monkeypatch.setattr(server, "initialize_database_if_configured", lambda: (_ for _ in ()).throw(AssertionError("read-only startup ran schema DDL")))
    monkeypatch.setattr(server, "verify_database_read_only_if_configured", lambda: None)
    monkeypatch.setattr(server, "save_rankings", lambda: (_ for _ in ()).throw(AssertionError("read-only shutdown saved rankings")))
    monkeypatch.setattr(server, "save_question_stats", lambda: (_ for _ in ()).throw(AssertionError("read-only shutdown saved question stats")))

    with TestClient(server.app) as client:
        response = client.post(
            "/api/feedback",
            json={"question_index": 1, "question_bank": "java", "suggestion": "must be blocked"},
        )
        assert response.status_code == 503
        assert response.headers["retry-after"] == "60"
        assert response.json()["detail"] == "旧 QuizCraft 正处于只读观察期"

        health = client.get("/api/healthz")
        assert health.status_code == 200
        assert health.json() == {
            "status": "ok",
            "read_only": True,
        }
