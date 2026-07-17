#!/usr/bin/env python3
# -*- coding: UTF-8 -*
"""测试题目解析功能"""

import sys
sys.path.insert(0, '.')
from server import parse_questions_from_text
import json

# 测试文本
test_text = """
1、在中共长期的革命战争年代形成的优良作风是（）
A. 艰苦奋斗
B. 理论与实际相结合  
C. 密切联系群众
D. 批评与自我批评
答案：BCD
解析：这是党的三大优良作风。

2、以下哪些是马克思主义的基本原理？
A、唯物史观
B、剩余价值理论
C、阶级斗争
D、科学发展观
答案：ABC

3、中国共产党成立的时间是1921年。
答案：对
"""

print("=" * 60)
print("测试题目解析功能")
print("=" * 60)

questions = parse_questions_from_text(test_text)

print(f"\n共解析到 {len(questions)} 道题目：\n")

for q in questions:
    print(f"【题目 {q['number']}】类型: {q['type']}")
    print(f"题干: {q['content'][:50]}...")
    if q.get('options'):
        print(f"选项数: {len(q['options'])}")
        for i, opt in enumerate(q['options']):
            print(f"  {chr(65+i)}. {opt[:30]}...")
    print(f"答案: {q.get('answer', '无')}")
    if q.get('analysis'):
        print(f"解析: {q['analysis'][:50]}...")
    print("-" * 60)

# 验证
print("\n验证结果:")
assert len(questions) == 3, f"应解析出3道题，实际{len(questions)}道"
assert questions[0]['type'] == 'multi', "第1题应为多选题"
assert questions[0]['answer'] == 'BCD', f"第1题答案应为BCD，实际为{questions[0]['answer']}"
assert len(questions[0]['options']) == 4, f"第1题应有4个选项，实际{len(questions[0]['options'])}个"
assert questions[2]['type'] == 'judge', "第3题应为判断题"
print("✓ 所有测试通过！")
