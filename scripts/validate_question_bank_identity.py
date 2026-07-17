#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Validate question-bank IDs before syncing a bank to production."""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from pathlib import Path
from typing import Any


ID_RE = re.compile(r"^(?P<prefix>.+)_(?P<number>\d{4})$")


def load_questions(path: Path) -> list[dict[str, Any]]:
    raw = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(raw, dict):
        questions = raw.get("questions", [])
    else:
        questions = raw
    if not isinstance(questions, list):
        raise ValueError(f"{path}: questions must be a list")
    return [q for q in questions if isinstance(q, dict)]


def validate_file(path: Path) -> list[str]:
    questions = load_questions(path)
    errors: list[str] = []
    ids = [str(q.get("id") or "").strip() for q in questions]
    counts = Counter(ids)

    for index, qid in enumerate(ids, 1):
        if not qid:
            errors.append(f"{path}: question #{index} has empty id")
        elif counts[qid] > 1:
            errors.append(f"{path}: duplicate id {qid!r}")

    matches = [ID_RE.fullmatch(qid) for qid in ids if qid]
    if matches and len(matches) == len([qid for qid in ids if qid]):
        prefixes = {m.group("prefix") for m in matches if m}
        if len(prefixes) == 1:
            prefix = next(iter(prefixes))
            for index, qid in enumerate(ids, 1):
                expected = f"{prefix}_{index:04d}"
                if qid != expected:
                    errors.append(
                        f"{path}: question #{index} id {qid!r} should be {expected!r}"
                    )

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("files", nargs="+", type=Path)
    args = parser.parse_args()

    all_errors: list[str] = []
    for path in args.files:
        all_errors.extend(validate_file(path))

    if all_errors:
        for error in all_errors:
            print(error, file=sys.stderr)
        return 1

    print(f"ok files={len(args.files)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
