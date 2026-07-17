import json
import sys
from pathlib import Path

from fastapi.testclient import TestClient

PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

import server
from scripts.bank_metadata import sanitize_source_reference


LOCAL_PATH_MARKERS = ("/Users/", "/home/", "C:\\Users\\", "file://")


def tracked_generated_json_files() -> list[Path]:
    return sorted(PROJECT_ROOT.glob("generated/**/*.json"))


def test_sanitize_source_reference_keeps_traceable_filename():
    assert (
        sanitize_source_reference("/Users/jerry/Downloads/软件工程第一次过程性测试_202606249.md")
        == "软件工程第一次过程性测试_202606249.md"
    )
    assert sanitize_source_reference("file:///home/jerry/banks/java_bank.md") == "java_bank.md"


def test_generated_json_files_do_not_contain_local_absolute_paths():
    offenders = []
    for path in tracked_generated_json_files():
        text = path.read_text(encoding="utf-8")
        if any(marker in text for marker in LOCAL_PATH_MARKERS):
            offenders.append(str(path.relative_to(PROJECT_ROOT)))

    assert offenders == []


def test_software_engineering_bank_is_exposed_from_local_registry(monkeypatch):
    monkeypatch.setattr(server, "db_runtime_enabled", lambda: False)
    monkeypatch.delenv("DISABLED_BANK_KEYS", raising=False)

    try:
        server.load_question_banks()
        response = TestClient(server.app).get("/api/banks")
    finally:
        server.QUESTION_BANKS.clear()
        server.QUESTION_CACHE.clear()
        server.QUESTION_INDEX.clear()

    assert response.status_code == 200
    banks = response.json()["banks"]
    bank = next(
        item for item in banks if item["key"] == "software_engineering_process_tests"
    )
    assert bank["name"] == "软件工程过程性测试"
    assert bank["total"] == 90
    assert {chapter["id"] for chapter in bank["chapters"]} == {"ch01", "ch02"}


def test_software_engineering_bank_metadata_is_traceable_without_local_paths():
    path = PROJECT_ROOT / "generated/software_engineering_process_tests.json"
    data = json.loads(path.read_text(encoding="utf-8"))

    source_files = data["meta"]["source_files"]
    assert source_files == [
        "软件工程第一次过程性测试_202606249.md",
        "软件工程第二次过程性测试_202606249.md",
    ]
