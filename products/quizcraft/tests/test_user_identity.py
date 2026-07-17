import asyncio

import server


def test_auto_created_user_id_is_not_bare_numeric(monkeypatch):
    monkeypatch.setattr(server, "NEXT_USER_ID", 7)
    server.USER_STATS.clear()
    server.NAME_TO_ID.clear()

    result = asyncio.run(server.set_user(server.UserRequest(name=None)))

    assert result["user_id"] == "qc_user_7"
    assert result["name"] == "qc_user_7"
    assert "7" not in server.USER_STATS


def test_submit_answer_returns_resolved_user_identity():
    server.QUESTION_BANKS["identity_test"] = {
        "name": "Identity Test",
        "color": "#1976d2",
        "data": {
            "questions": [
                {
                    "id": "q1",
                    "number": "1",
                    "type": "single",
                    "chapter": "1",
                    "chapter_id": "ch01",
                    "content": "Pick A.",
                    "options": ["A", "B"],
                    "answer": 0,
                    "analysis": "",
                }
            ]
        },
    }
    server.QUESTION_CACHE.pop("identity_test", None)
    server.QUESTION_INDEX.pop("identity_test", None)
    server.USER_STATS.clear()
    server.NAME_TO_ID.clear()

    try:
        result = asyncio.run(
            server.submit_answer(
                server.SubmitAnswerRequest(
                    bank="identity_test",
                    question_id="q1",
                    answer=0,
                    user_id="2510250380",
                )
            )
        )
    finally:
        server.QUESTION_BANKS.pop("identity_test", None)
        server.QUESTION_CACHE.pop("identity_test", None)
        server.QUESTION_INDEX.pop("identity_test", None)
        server.USER_STATS.clear()
        server.NAME_TO_ID.clear()

    assert result["user_stats"]["user_id"] == "2510250380"
    assert result["user_stats"]["name"] == "2510250380"
