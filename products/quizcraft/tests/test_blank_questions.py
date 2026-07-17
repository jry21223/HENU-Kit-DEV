import asyncio

import pytest
from fastapi import HTTPException

import server


def test_build_standard_bank_data_accepts_blank_question_without_options():
    bank = server.build_standard_bank_data(
        "填空题测试",
        [
            {
                "type": "填空题",
                "chapter": "1",
                "content": "我国的根本制度是____。",
                "answer": "社会主义制度",
                "analysis": "宪法规定社会主义制度是根本制度。",
            }
        ],
    )

    question = bank["questions"][0]
    assert question["type"] == "blank"
    assert question["chapter"] == "1"
    assert question["chapter_id"] == "ch01"
    assert question["options"] is None
    assert question["answer"] == "社会主义制度"


def test_parse_question_bank_keeps_standard_blank_questions():
    questions = server.parse_question_bank(
        {
            "questions": [
                {
                    "id": "q1",
                    "type": "blank",
                    "chapter": "2",
                    "content": "国家的一切权力属于____。",
                    "answer": "人民",
                }
            ]
        },
        "blank_test",
    )

    assert len(questions) == 1
    assert questions[0]["type"] == "blank"
    assert questions[0]["options"] is None
    assert questions[0]["answer"] == "人民"


def test_submit_answer_scores_blank_question_with_trimmed_text():
    server.QUESTION_BANKS["blank_test"] = {
        "name": "填空题测试",
        "color": "#1976d2",
        "data": {
            "questions": [
                {
                    "id": "q1",
                    "number": "1",
                    "type": "blank",
                    "chapter": "1",
                    "chapter_id": "ch01",
                    "content": "国家的一切权力属于____。",
                    "options": None,
                    "answer": "人民",
                    "analysis": "",
                }
            ]
        },
    }
    server.QUESTION_CACHE.pop("blank_test", None)
    server.QUESTION_INDEX.pop("blank_test", None)

    try:
        result = asyncio.run(
            server.submit_answer(
                server.SubmitAnswerRequest(
                    bank="blank_test",
                    question_id="q1",
                    answer="  人民  ",
                )
            )
        )
    finally:
        server.QUESTION_BANKS.pop("blank_test", None)
        server.QUESTION_CACHE.pop("blank_test", None)
        server.QUESTION_INDEX.pop("blank_test", None)

    assert result["correct"] is True
    assert result["correct_answer"] == "人民"


def test_blank_question_requires_non_empty_answer():
    with pytest.raises(HTTPException) as exc_info:
        server.build_standard_bank_data(
            "填空题测试",
            [
                {
                    "type": "blank",
                    "content": "国家的一切权力属于____。",
                    "answer": " ",
                }
            ],
        )

    assert "填空题答案不能为空" in exc_info.value.detail


def test_build_standard_bank_data_preserves_blank_answer_candidates():
    bank = server.build_standard_bank_data(
        "填空题候选答案测试",
        [
            {
                "type": "blank",
                "content": "Software is another name for ____.",
                "answer": ["program", "programs"],
            }
        ],
    )

    assert bank["questions"][0]["answer"] == ["program", "programs"]


def test_submit_answer_scores_blank_question_against_candidates():
    server.QUESTION_BANKS["blank_candidates_test"] = {
        "name": "填空题候选答案测试",
        "color": "#1976d2",
        "data": {
            "questions": [
                {
                    "id": "q1",
                    "number": "1",
                    "type": "blank",
                    "chapter": "1",
                    "chapter_id": "ch01",
                    "content": "Software is another name for ____.",
                    "options": None,
                    "answer": ["program", "programs"],
                    "analysis": "",
                }
            ]
        },
    }
    server.QUESTION_CACHE.pop("blank_candidates_test", None)
    server.QUESTION_INDEX.pop("blank_candidates_test", None)

    try:
        result = asyncio.run(
            server.submit_answer(
                server.SubmitAnswerRequest(
                    bank="blank_candidates_test",
                    question_id="q1",
                    answer="Programs",
                )
            )
        )
    finally:
        server.QUESTION_BANKS.pop("blank_candidates_test", None)
        server.QUESTION_CACHE.pop("blank_candidates_test", None)
        server.QUESTION_INDEX.pop("blank_candidates_test", None)

    assert result["correct"] is True
    assert result["correct_answer"] == ["program", "programs"]


def test_build_standard_bank_data_uses_group_as_chapter_alias():
    bank = server.build_standard_bank_data(
        "组别章节测试",
        [
            {
                "type": "blank",
                "group": "3",
                "content": "国家的一切权力属于____。",
                "answer": "人民",
            }
        ],
    )

    question = bank["questions"][0]
    assert question["chapter"] == "3"
    assert question["chapter_id"] == "ch01"


def test_save_bank_syncs_new_bank_to_db_before_runtime_reload(monkeypatch, tmp_path):
    monkeypatch.setattr(server, "TIKU_DIR", str(tmp_path))
    monkeypatch.setattr(server, "db_runtime_enabled", lambda: True)
    synced = []

    def fake_sync(key, bank):
        synced.append((key, len(bank["data"]["questions"])))
        return True

    def fake_load_question_banks():
        if synced:
            server.QUESTION_BANKS["new_bank"] = {
                "name": "新题库",
                "color": "#1976d2",
                "file": "postgresql:new_bank",
                "data": {
                    "meta": {"name": "新题库"},
                    "questions": [
                        {
                            "id": "q0001",
                            "number": "1",
                            "type": "single",
                            "content": "题干",
                            "options": ["A", "B"],
                            "answer": 0,
                            "chapter": "默认章节",
                            "chapter_id": "ch01",
                        }
                    ],
                },
            }

    monkeypatch.setattr(server, "sync_question_bank_to_db", fake_sync)
    monkeypatch.setattr(server, "load_question_banks", fake_load_question_banks)
    server.QUESTION_BANKS.pop("new_bank", None)

    try:
        result = asyncio.run(
            server.save_bank(
                server.SaveBankRequest(
                    key="new_bank",
                    name="新题库",
                    questions=[
                        {
                            "type": "single",
                            "content": "题干",
                            "options": ["A", "B"],
                            "answer": 0,
                        }
                    ],
                    overwrite=True,
                )
            )
        )
    finally:
        server.QUESTION_BANKS.pop("new_bank", None)

    assert synced == [("new_bank", 1)]
    assert result["bank"]["key"] == "new_bank"


@pytest.mark.parametrize("value", ["1", "true", "TRUE", "yes", "on"])
def test_local_bank_db_sync_requires_explicit_truthy_env(monkeypatch, value):
    monkeypatch.setenv("QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB", value)

    assert server.should_sync_local_banks_to_db() is True


@pytest.mark.parametrize("value", ["", "0", "false", "no", "off", "random"])
def test_local_bank_db_sync_defaults_off(monkeypatch, value):
    if value:
        monkeypatch.setenv("QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB", value)
    else:
        monkeypatch.delenv("QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB", raising=False)

    assert server.should_sync_local_banks_to_db() is False


def test_load_question_banks_prefers_db_when_runtime_enabled(monkeypatch, tmp_path):
    monkeypatch.setattr(server, "TIKU_DIR", str(tmp_path))
    monkeypatch.setattr(server, "db_runtime_enabled", lambda: True)
    monkeypatch.setattr(server, "should_sync_local_banks_to_db", lambda: False)
    monkeypatch.setattr(
        server.db_storage,
        "load_question_banks",
        lambda: {
            "db_bank": {
                "name": "DB 题库",
                "color": "#1976d2",
                "metadata": {"name": "DB 题库"},
                "questions": [
                    {
                        "id": "q1",
                        "number": "1",
                        "type": "single",
                        "chapter": "1",
                        "chapter_id": "ch01",
                        "content": "题干",
                        "options": ["A", "B"],
                        "answer": 0,
                        "analysis": "",
                    }
                ],
            }
        },
    )

    def fail_local_load(*args, **kwargs):
        raise AssertionError("local JSON should not load when PostgreSQL has banks")

    monkeypatch.setattr(server, "load_bank_from_file", fail_local_load)

    try:
        server.load_question_banks()
        assert set(server.QUESTION_BANKS) == {"db_bank"}
        assert server.QUESTION_BANKS["db_bank"]["file"] == "postgresql:db_bank"
    finally:
        server.QUESTION_BANKS.clear()
        server.QUESTION_CACHE.clear()
        server.QUESTION_INDEX.clear()
