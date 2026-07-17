#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path

from bank_metadata import LOCAL_PATH_MARKERS, sanitize_source_metadata


PROJECT_ROOT = Path(__file__).resolve().parents[1]


def has_local_reference(value: object) -> bool:
    text = json.dumps(value, ensure_ascii=False)
    return any(marker in text for marker in LOCAL_PATH_MARKERS)


def sanitize_file(path: Path, *, check: bool = False) -> bool:
    data = json.loads(path.read_text(encoding="utf-8"))
    sanitized = sanitize_source_metadata(data)
    changed = sanitized != data

    if check:
        if changed or has_local_reference(sanitized):
            raise SystemExit(f"local source path found: {path}")
        return False

    if changed:
        path.write_text(
            json.dumps(sanitized, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
    return changed


def main() -> int:
    parser = argparse.ArgumentParser(description="Sanitize local paths in generated QuizCraft JSON files.")
    parser.add_argument("paths", nargs="*", type=Path, help="JSON files or directories. Defaults to generated/.")
    parser.add_argument("--check", action="store_true", help="Fail if any file would be rewritten.")
    args = parser.parse_args()

    roots = args.paths or [PROJECT_ROOT / "generated"]
    files: list[Path] = []
    for root in roots:
        path = root if root.is_absolute() else PROJECT_ROOT / root
        if path.is_dir():
            files.extend(sorted(path.rglob("*.json")))
        else:
            files.append(path)

    changed = [path for path in files if sanitize_file(path, check=args.check)]
    if not args.check:
        for path in changed:
            print(path.relative_to(PROJECT_ROOT))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
