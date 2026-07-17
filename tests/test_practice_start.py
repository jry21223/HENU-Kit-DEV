import asyncio

import server


def test_chapter_mode_returns_every_question_in_selected_chapter():
    server.QUESTION_BANKS["chapter_all_test"] = {
        "name": "Chapter All Test",
        "color": "#1976d2",
        "data": {
            "questions": [
                {
                    "id": "ch01_q1",
                    "number": "1",
                    "type": "single",
                    "chapter": "Chapter 1",
                    "chapter_id": "ch01",
                    "content": "Q1",
                    "options": ["A", "B"],
                    "answer": 0,
                    "analysis": "",
                },
                {
                    "id": "ch01_q2",
                    "number": "2",
                    "type": "single",
                    "chapter": "Chapter 1",
                    "chapter_id": "ch01",
                    "content": "Q2",
                    "options": ["A", "B"],
                    "answer": 0,
                    "analysis": "",
                },
                {
                    "id": "ch01_q3",
                    "number": "3",
                    "type": "single",
                    "chapter": "Chapter 1",
                    "chapter_id": "ch01",
                    "content": "Q3",
                    "options": ["A", "B"],
                    "answer": 0,
                    "analysis": "",
                },
                {
                    "id": "ch02_q1",
                    "number": "4",
                    "type": "single",
                    "chapter": "Chapter 2",
                    "chapter_id": "ch02",
                    "content": "Q4",
                    "options": ["A", "B"],
                    "answer": 0,
                    "analysis": "",
                },
            ]
        },
    }
    server.QUESTION_CACHE.pop("chapter_all_test", None)
    server.QUESTION_INDEX.pop("chapter_all_test", None)

    try:
        result = asyncio.run(
            server.start_practice(
                server.StartPracticeRequest(
                    bank="chapter_all_test",
                    mode="chapter",
                    params={"chapter_id": "ch01", "count": 1},
                )
            )
        )
    finally:
        server.QUESTION_BANKS.pop("chapter_all_test", None)
        server.QUESTION_CACHE.pop("chapter_all_test", None)
        server.QUESTION_INDEX.pop("chapter_all_test", None)

    assert result["total"] == 3
    assert {question["id"] for question in result["questions"]} == {
        "ch01_q1",
        "ch01_q2",
        "ch01_q3",
    }
