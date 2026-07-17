import json

from fastapi.testclient import TestClient

import server
from scripts import feedback_mcp_server
from scripts.java_bank_workflow import (
    build_incremental_bank,
    build_save_payload,
    fingerprint_question,
    parse_java_markdown_bank,
)


def test_parse_java_markdown_bank_filters_from_start_number(tmp_path):
    source = tmp_path / "java.md"
    source.write_text(
        """# Java 题库

## 148. 单选题

旧题（　）

- A. A
- B. B
- C. C
- D. D

---

## 149. 单选题

新题（　）

- A. A
- B. B
- C. C
- D. D
""",
        encoding="utf-8",
    )

    bank = parse_java_markdown_bank(source, start_number=149)

    assert [question["number"] for question in bank["questions"]] == ["149"]
    assert bank["meta"]["total"] == 1


def test_build_incremental_bank_skips_existing_content_and_keeps_real_order():
    existing = {
        "meta": {"name": "Java程序设计题库", "color": "#f57c00"},
        "questions": [
            {
                "id": "java_0155",
                "number": "155",
                "type": "single",
                "chapter": "Java题库",
                "chapter_id": "ch01",
                "content": "Thread类定义于下列（　）包中。",
                "options": ["java.util", "java.lang", "java.io", "java.thread"],
                "answer": 1,
                "analysis": "Thread 属于 java.lang 包。",
            }
        ],
    }
    incoming = {
        "questions": [
            {
                "id": "java_0155",
                "number": "155",
                "type": "single",
                "chapter": "Java题库",
                "chapter_id": "ch01",
                "content": "Thread类定义于下列（　）包中。",
                "options": ["java.util", "java.lang", "java.io", "java.thread"],
                "answer": None,
                "analysis": "",
            },
            {
                "id": "java_0156",
                "number": "156",
                "type": "single",
                "chapter": "Java题库",
                "chapter_id": "ch01",
                "content": "Runnable接口中的抽象方法是（　）",
                "options": ["run()", "start()", "sleep()", "wait()"],
                "answer": None,
                "analysis": "",
            },
        ]
    }

    result = build_incremental_bank(existing, incoming, start_number=149, id_prefix="java")

    assert result.added_count == 1
    assert result.skipped_duplicate_count == 1
    assert [question["number"] for question in result.bank["questions"]] == ["155", "156"]
    assert result.bank["questions"][-1]["id"] == "java_0156"
    assert result.added_questions[0]["answer"] is None


def test_build_save_payload_uses_existing_bank_metadata():
    bank = {
        "meta": {"name": "Java程序设计题库", "color": "#f57c00"},
        "questions": [{"id": "java_0001"}],
    }

    payload = build_save_payload(bank, key="java_programming")

    assert payload == {
        "key": "java_programming",
        "name": "Java程序设计题库",
        "color": "#f57c00",
        "questions": [{"id": "java_0001"}],
        "overwrite": True,
    }


def test_fingerprint_question_ignores_whitespace_and_option_case():
    left = {
        "content": "  Thread 类 定义于 下列 包中。 ",
        "options": [" java.util ", "JAVA.LANG", "java.io", "java.thread"],
    }
    right = {
        "content": "Thread类定义于下列包中。",
        "options": ["java.util", "java.lang", "java.io", "java.thread"],
    }

    assert fingerprint_question(left) == fingerprint_question(right)


def test_java_incremental_upload_api_previews_merge_without_saving(monkeypatch):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    server.QUESTION_BANKS["java_programming"] = {
        "name": "Java程序设计题库",
        "color": "#f57c00",
        "data": {
            "meta": {"name": "Java程序设计题库", "color": "#f57c00"},
            "questions": [
                {
                    "id": "java_0155",
                    "number": "155",
                    "type": "single",
                    "chapter": "Java题库",
                    "chapter_id": "ch01",
                    "content": "Thread类定义于下列（　）包中。",
                    "options": ["java.util", "java.lang", "java.io", "java.thread"],
                    "answer": 1,
                    "analysis": "Thread 属于 java.lang 包。",
                }
            ],
        },
    }
    server.QUESTION_CACHE.pop("java_programming", None)
    server.QUESTION_INDEX.pop("java_programming", None)

    markdown = """# Java 题库

## 155. 单选题

Thread类定义于下列（　）包中。

- A. java.util
- B. java.lang
- C. java.io
- D. java.thread

---

## 156. 单选题

Runnable接口中的抽象方法是（　）

- A. run()
- B. start()
- C. sleep()
- D. wait()
"""

    try:
        client = TestClient(server.app)
        response = client.post(
            "/api/banks/java/append-from-markdown",
            data={"start_number": "149", "analyze": "false", "save": "false"},
            files={"file": ("java.md", markdown.encode("utf-8"), "text/markdown")},
            headers={"X-Admin-Token": "test-token"},
        )
    finally:
        server.QUESTION_BANKS.pop("java_programming", None)
        server.QUESTION_CACHE.pop("java_programming", None)
        server.QUESTION_INDEX.pop("java_programming", None)

    assert response.status_code == 200
    payload = response.json()
    assert payload["added"] == 1
    assert payload["skipped_duplicates"] == 1
    assert payload["saved"] is False
    assert payload["bank"]["total"] == 2


def test_java_incremental_upload_api_analyzes_only_added_questions(monkeypatch):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    server.QUESTION_BANKS["java_programming"] = {
        "name": "Java程序设计题库",
        "color": "#f57c00",
        "data": {
            "meta": {"name": "Java程序设计题库", "color": "#f57c00"},
            "questions": [],
        },
    }

    async def fake_analyzer(questions):
        for question in questions:
            question["answer"] = 0
            question["analysis"] = "Runnable 接口的抽象方法是 run()，线程启动后会执行该方法。"
        return questions

    monkeypatch.setattr(server, "fill_java_answer_analyses", fake_analyzer)
    markdown = """# Java 题库

## 156. 单选题

Runnable接口中的抽象方法是（　）

- A. run()
- B. start()
- C. sleep()
- D. wait()
"""

    try:
        client = TestClient(server.app)
        response = client.post(
            "/api/banks/java/append-from-markdown",
            data={"start_number": "149", "analyze": "true", "save": "false"},
            files={"file": ("java.md", markdown.encode("utf-8"), "text/markdown")},
            headers={"X-Admin-Token": "test-token"},
        )
    finally:
        server.QUESTION_BANKS.pop("java_programming", None)
        server.QUESTION_CACHE.pop("java_programming", None)
        server.QUESTION_INDEX.pop("java_programming", None)

    assert response.status_code == 200
    added = response.json()["added_questions"]
    assert added[0]["answer"] == 0
    assert "run()" in added[0]["analysis"]


def test_mcp_java_append_endpoint_reuses_http_api():
    assert (
        feedback_mcp_server._java_append_endpoint("http://127.0.0.1:10086/api")
        == "http://127.0.0.1:10086/api/banks/java/append-from-markdown"
    )


def test_mcp_java_append_base_url_reuses_admin_security_guard():
    assert (
        feedback_mcp_server._resolve_java_append_base_url("http://127.0.0.1:10086/api")
        == "http://127.0.0.1:10086/api"
    )

    try:
        feedback_mcp_server._resolve_java_append_base_url("http://8.146.200.82/api")
    except SystemExit as exc:
        assert "HTTPS" in str(exc)
    else:
        raise AssertionError("public HTTP admin API URL should be rejected")


def test_java_incremental_upload_api_syncs_saved_bank_before_reload(monkeypatch, tmp_path):
    monkeypatch.setenv("ADMIN_TOKEN", "test-token")
    monkeypatch.setattr(server, "TIKU_DIR", str(tmp_path))
    server.QUESTION_BANKS["java_programming"] = {
        "name": "Java程序设计题库",
        "color": "#f57c00",
        "data": {
            "meta": {"name": "Java程序设计题库", "color": "#f57c00"},
            "questions": [],
        },
    }

    async def fake_analyzer(questions):
        for question in questions:
            question["answer"] = 0
            question["analysis"] = "Runnable 接口的抽象方法是 run()。"
        return questions

    synced_totals = []
    monkeypatch.setattr(server, "fill_java_answer_analyses", fake_analyzer)
    monkeypatch.setattr(
        server,
        "sync_question_bank_to_db",
        lambda key, bank: synced_totals.append(len(bank["data"]["questions"])) or True,
    )
    monkeypatch.setattr(server, "load_question_banks", lambda: None)

    markdown = """# Java 题库

## 156. 单选题

Runnable接口中的抽象方法是（　）

- A. run()
- B. start()
- C. sleep()
- D. wait()
"""

    try:
        client = TestClient(server.app)
        response = client.post(
            "/api/banks/java/append-from-markdown",
            data={"start_number": "149", "analyze": "true", "save": "true"},
            files={"file": ("java.md", markdown.encode("utf-8"), "text/markdown")},
            headers={"X-Admin-Token": "test-token"},
        )
    finally:
        server.QUESTION_BANKS.pop("java_programming", None)
        server.QUESTION_CACHE.pop("java_programming", None)
        server.QUESTION_INDEX.pop("java_programming", None)

    assert response.status_code == 200
    assert synced_totals == [1]
