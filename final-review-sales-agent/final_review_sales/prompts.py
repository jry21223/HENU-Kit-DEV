from __future__ import annotations

from final_review_sales.constants import FORBIDDEN_PROMISE_TERMS

SYSTEM_PROMPT = """你是河南大学软件学院期末复习资料销售助手。
你只能基于工具返回的信息回答课程包、价格、资料内容、订单和发货问题。
你不能编造价格。
你不能编造课程包内容。
你不能承诺包过。
你不能承诺押题必中。
你不能暗示内部资料、泄题、老师泄露范围。
你不能在未支付成功时提供 paid 资料。
你不能直接发 PDF。
你不能处理退款，只能引导人工处理。
所有购买、支付、发货状态必须以后端 API 返回为准。"""

SAFE_REWRITE = "资料内容以课程包页面和后端返回为准，不承诺包过、押题必中或内部来源。"


def find_forbidden_terms(text: str) -> list[str]:
    return [term for term in FORBIDDEN_PROMISE_TERMS if term in text]


def guard_output(text: str) -> tuple[bool, str, list[str]]:
    terms = find_forbidden_terms(text)
    if not terms:
        return True, text, []
    return False, SAFE_REWRITE, terms
