from fastapi.testclient import TestClient

import server


def test_legacy_ranking_endpoint_fails_closed_until_v2_cutover(monkeypatch):
    source_identifier = "legacy-user-4c6f4d2e@example.test"
    source_name = "不应公开的旧名称"
    monkeypatch.setattr(server, "db_runtime_enabled", lambda: False)
    monkeypatch.setattr(server, "USER_STATS", {
        source_identifier: {
            "name": source_name,
            "correct": 12,
            "total": 15,
            "practice_history": [],
        },
    })

    with TestClient(server.app) as client:
        response = client.get("/api/ranking")

    assert response.status_code == 503
    assert response.json() == {"detail": "旧排行榜正在迁移，暂不可用"}
    assert source_identifier not in response.text
    assert source_name not in response.text
