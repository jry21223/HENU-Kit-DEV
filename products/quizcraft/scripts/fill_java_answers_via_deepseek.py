#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import time
import urllib.request
from pathlib import Path
from typing import Any


API_URL = "https://api.deepseek.com/chat/completions"


def post(api_key: str, payload: dict[str, Any], timeout: int) -> dict[str, Any]:
    request = urllib.request.Request(
        API_URL,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {api_key}"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def prompt(question: dict[str, Any]) -> str:
    options = "\n".join(f"{chr(65+i)}. {option}" for i, option in enumerate(question["options"]))
    return f"""请为下面这道 Java 程序设计单选题判定正确答案并生成解析。

要求：
1. 只输出 JSON，不要 Markdown。
2. answer 输出 0-3 的整数，分别代表 A-D。
3. analysis 用中文，50-120 字，说明为什么该项正确，必要时指出其他选项错误点。
4. 如两个选项文本完全相同且都正确，优先选择靠前的选项。

题目：{question["content"]}
选项：
{options}
"""


def normalize_answer(raw: Any) -> int | None:
    if isinstance(raw, int) and 0 <= raw <= 3:
        return raw
    if isinstance(raw, str):
        s = raw.strip().upper()
        if s[:1] in "ABCD":
            return ord(s[0]) - 65
        if s.isdigit() and 0 <= int(s) <= 3:
            return int(s)
    return None


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("bank_json", type=Path)
    parser.add_argument("--output", type=Path, default=Path("generated/java_bank_analyzed.json"))
    parser.add_argument("--api-key", default=os.getenv("DEEPSEEK_API_KEY") or os.getenv("LLM_API_KEY"))
    parser.add_argument("--timeout", type=int, default=120)
    parser.add_argument("--limit", type=int)
    args = parser.parse_args()
    if not args.api_key:
        raise SystemExit("Missing --api-key or DEEPSEEK_API_KEY.")

    data = json.loads(args.bank_json.read_text(encoding="utf-8"))
    pending = [q for q in data["questions"] if q.get("answer") is None or not q.get("analysis")]
    if args.limit is not None:
        pending = pending[: args.limit]
    print(f"pending={len(pending)} output={args.output}")
    started = time.time()
    for idx, question in enumerate(pending, start=1):
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "system", "content": "你是严谨的 Java 课程题库答案校验与解析助手。"},
                {"role": "user", "content": prompt(question)},
            ],
            "temperature": 0,
            "response_format": {"type": "json_object"},
        }
        response = post(args.api_key, payload, args.timeout)
        content = response["choices"][0]["message"]["content"]
        result = json.loads(content)
        answer = normalize_answer(result.get("answer"))
        if answer is None:
            raise SystemExit(f"Invalid answer for {question['id']}: {content}")
        analysis = str(result.get("analysis") or result.get("reason") or "").strip()
        if not analysis:
            raise SystemExit(f"Missing analysis for {question['id']}: {content}")
        question["answer"] = answer
        question["analysis"] = analysis
        args.output.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
        if idx % 10 == 0 or idx == len(pending):
            elapsed = max(0.1, time.time() - started)
            print(f"filled={idx}/{len(pending)} rate={idx/elapsed:.2f}/s")
    print(json.dumps({"filled": len(pending), "output": str(args.output)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
