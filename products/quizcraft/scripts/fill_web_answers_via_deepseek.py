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


def post_deepseek(api_key: str, payload: dict[str, Any], timeout: int) -> dict[str, Any]:
    request = urllib.request.Request(
        API_URL,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {api_key}"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def build_prompt(question: dict[str, Any]) -> str:
    options = "\n".join(
        f"{chr(65 + idx)}. {option}"
        for idx, option in enumerate(question.get("options") or [])
    )
    return f"""请为下面这道 Web 前端基础题判定正确答案并生成解析。

要求：
1. 只输出 JSON，不要 Markdown。
2. 单选题 answer 输出 0-3 的整数；多选题 answer 输出整数数组，如 [0,2]；判断题 answer 输出 true 或 false。
3. analysis 用中文，50-120 字，说明为什么该答案正确，必要时指出易错点。
4. 如果题干表述有歧义，按常见 HTML/CSS/JavaScript 教材知识判定。

题型：{question["type"]}
题目：{question["content"]}
选项：
{options}
"""


def normalize_answer(question: dict[str, Any], raw_answer: Any) -> Any:
    q_type = question["type"]
    if q_type == "judge":
        if isinstance(raw_answer, bool):
            return raw_answer
        text = str(raw_answer).strip().lower()
        if text in {"true", "对", "正确", "yes"}:
            return True
        if text in {"false", "错", "错误", "no"}:
            return False
        return None
    if q_type == "single":
        if isinstance(raw_answer, int) and 0 <= raw_answer <= 3:
            return raw_answer
        if isinstance(raw_answer, str) and raw_answer.strip().upper()[:1] in "ABCD":
            return ord(raw_answer.strip().upper()[0]) - 65
        return None
    if q_type == "multi":
        values = raw_answer
        if isinstance(values, str):
            values = [ord(ch) - 65 for ch in values.upper() if ch in "ABCD"]
        if isinstance(values, list) and all(isinstance(item, int) and 0 <= item <= 3 for item in values):
            return sorted(set(values))
        return None
    return None


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("bank_json", type=Path)
    parser.add_argument("--output", type=Path, default=Path("generated/web_bank_analyzed.json"))
    parser.add_argument("--api-key", default=os.getenv("DEEPSEEK_API_KEY") or os.getenv("LLM_API_KEY"))
    parser.add_argument("--limit", type=int)
    parser.add_argument("--timeout", type=int, default=120)
    args = parser.parse_args()

    if not args.api_key:
        raise SystemExit("Missing --api-key or DEEPSEEK_API_KEY.")

    data = json.loads(args.bank_json.read_text(encoding="utf-8"))
    questions = data["questions"]
    pending = [q for q in questions if q.get("answer") is None or not q.get("analysis")]
    if args.limit is not None:
        pending = pending[: args.limit]

    started = time.time()
    print(f"pending={len(pending)} output={args.output}")
    for idx, question in enumerate(pending, start=1):
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "system", "content": "你是严谨的 Web 前端课程题库答案校验与解析助手。"},
                {"role": "user", "content": build_prompt(question)},
            ],
            "temperature": 0,
            "response_format": {"type": "json_object"},
        }
        response = post_deepseek(args.api_key, payload, args.timeout)
        content = response["choices"][0]["message"]["content"]
        result = json.loads(content)
        answer = normalize_answer(question, result.get("answer"))
        if answer is None:
            raise SystemExit(f"Invalid answer for {question['id']}: {content}")
        question["answer"] = answer
        question["analysis"] = str(result.get("analysis") or result.get("reason") or "").strip()
        if not question["analysis"]:
            raise SystemExit(f"Missing analysis for {question['id']}: {content}")
        args.output.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
        if idx % 10 == 0 or idx == len(pending):
            elapsed = max(0.1, time.time() - started)
            print(f"filled={idx}/{len(pending)} rate={idx/elapsed:.2f}/s")

    print(json.dumps({"filled": len(pending), "output": str(args.output)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
