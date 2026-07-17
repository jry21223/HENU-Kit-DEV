#!/usr/bin/env python3

import argparse
import json
from pathlib import Path
import sys

PROJECT_ROOT = Path(__file__).resolve().parent.parent
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

import server


def main() -> int:
    parser = argparse.ArgumentParser(description="Convert a docx question bank into tiku JSON.")
    parser.add_argument("--input", required=True, help="Path to the source docx file")
    parser.add_argument("--name", required=True, help="Question bank display name")
    parser.add_argument("--key", required=True, help="Question bank key / output filename stem")
    parser.add_argument(
        "--output",
        help="Output JSON path. Defaults to tiku/<key>.json under the project root.",
    )
    args = parser.parse_args()

    input_path = Path(args.input).expanduser().resolve()
    if not input_path.exists():
        raise SystemExit(f"Input docx not found: {input_path}")

    output_path = (
        Path(args.output).expanduser().resolve()
        if args.output
        else PROJECT_ROOT / "tiku" / f"{args.key}.json"
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)

    questions = server.parse_questions_from_docx(str(input_path))
    bank = server.build_standard_bank_data(args.name, questions)
    output_path.write_text(json.dumps(bank, ensure_ascii=False, indent=2), encoding="utf-8")

    print(f"input={input_path}")
    print(f"output={output_path}")
    print(f"total={bank['meta']['total']}")
    print(f"chapter_count={bank['meta'].get('chapter_count', 0)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
