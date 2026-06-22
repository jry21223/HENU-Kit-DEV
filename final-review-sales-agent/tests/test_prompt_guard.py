from __future__ import annotations

from final_review_sales.prompts import guard_output


def test_forbidden_terms_are_blocked():
    for text in ["这个包过", "押题必中", "内部资料", "老师给的原题", "百分百保过"]:
        allowed, rewritten, terms = guard_output(text)
        assert allowed is False
        assert terms
        assert "不承诺" in rewritten


def test_normal_package_intro_allowed():
    allowed, rewritten, terms = guard_output("离散数学复习包包含重点讲义和模拟卷。")
    assert allowed is True
    assert rewritten == "离散数学复习包包含重点讲义和模拟卷。"
    assert terms == []
