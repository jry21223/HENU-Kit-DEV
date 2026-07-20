import asyncio

import pytest
from fastapi import HTTPException

import server


def test_shadow_compare_is_authenticated_and_has_no_stats_side_effects(monkeypatch):
    monkeypatch.setenv("QUIZCRAFT_SHADOW_COMPARE_SECRET", "shadow-compare-secret-at-least-32-bytes")
    monkeypatch.setenv("QUIZCRAFT_READ_ONLY", "1")
    monkeypatch.setitem(server.QUESTION_BANKS, "shadow_compare_test", {
        "name": "Shadow Compare Test",
        "color": "#1976d2",
        "data": {"questions": [{
            "id": "q1",
            "number": "1",
            "type": "single",
            "chapter": "1",
            "chapter_id": "ch01",
            "content": "1 + 1 = ?",
            "options": ["1", "2"],
            "answer": 1,
            "analysis": "legacy analysis",
        }]},
    })
    server.QUESTION_CACHE.pop("shadow_compare_test", None)
    server.QUESTION_INDEX.pop("shadow_compare_test", None)
    monkeypatch.setattr(server, "update_global_question_stats", lambda *_args: pytest.fail("shadow compare mutated stats"))
    monkeypatch.setattr(server, "save_question_stats", lambda: pytest.fail("shadow compare persisted stats"))

    request = server.SubmitAnswerRequest(bank="shadow_compare_test", question_id="q1", answer=1)
    try:
        result = asyncio.run(server.shadow_compare_answer(
            request,
            x_quizcraft_shadow_secret="shadow-compare-secret-at-least-32-bytes",
        ))
        assert result == {"correct": True, "correct_answer": 1, "analysis": "legacy analysis"}

        with pytest.raises(HTTPException) as exc_info:
            asyncio.run(server.shadow_compare_answer(request, x_quizcraft_shadow_secret="wrong"))
        assert exc_info.value.status_code == 401
    finally:
        server.QUESTION_CACHE.pop("shadow_compare_test", None)
        server.QUESTION_INDEX.pop("shadow_compare_test", None)
