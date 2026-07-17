#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Minimal backend smoke test without mutating runtime stats."""

import pathlib
import sys

from fastapi.testclient import TestClient

ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from server import app


def main() -> int:
    with TestClient(app) as client:
        banks = client.get("/api/banks")
        banks.raise_for_status()
        payload = banks.json()
        bank_items = payload.get("banks", [])
        if not bank_items:
            raise AssertionError("no banks loaded")
        bank_key = bank_items[0]["key"]

        started = client.post(
            "/api/practice/start",
            json={"bank": bank_key, "mode": "random", "params": {"count": 3}},
        )
        started.raise_for_status()
        questions = started.json().get("questions", [])
        if not questions:
            raise AssertionError("practice/start returned no questions")

        print(f"ok banks={len(bank_items)} sample_bank={bank_key} questions={len(questions)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
