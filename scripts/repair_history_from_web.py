#!/usr/bin/env python3
import argparse
import html
import json
import re
from collections import defaultdict
from dataclasses import dataclass
from difflib import SequenceMatcher
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

from PyPDF2 import PdfReader

ROOT = Path(__file__).resolve().parents[1]
HISTORY_PATH = ROOT / 'tiku' / 'history.json'
REPORT_PATH = ROOT / 'history_web_repair_report.md'
TMP_SOURCES = ROOT / 'tmp_sources'
AHJZU_PDF = TMP_SOURCES / 'ahjzu.pdf'
JNNU_HTML = TMP_SOURCES / 'jnnu.html'

JUDGE_TRUE_VALUES = {'1', 'true', 't', 'yes', 'y', '对', '正确', '√'}
JUDGE_FALSE_VALUES = {'0', 'false', 'f', 'no', 'n', '错', '错误', '×'}

STOPWORDS = [
    '下列', '以下', '关于', '对于', '我国', '中国', '近代中国', '近代史上', '近代史',
    '中国近代史上', '近代', '主要', '表述', '的是', '是', '有', '和', '及', '其中',
    '一个', '成立', '中国共产党', '辛亥革命后', '后', '请问', '哪些', '什么',
]


@dataclass
class SourceQuestion:
    source: str
    q_type: str
    content: str
    norm: str
    options: List[str]
    answer: Optional[str]


@dataclass
class MatchResult:
    candidate: SourceQuestion
    score: float
    margin: float
    exact: bool
    contains: bool
    fit: float
    analysis_letters: Optional[str]
    analysis_value: Any


@dataclass
class PatchRecord:
    qid: str
    q_type: str
    content: str
    source: str
    score: float
    margin: float
    exact: bool
    contains: bool
    old_options: List[str]
    new_options: List[str]
    old_answer: Any
    new_answer: Any
    answer_method: str
    candidate_content: str


def normalize_text_for_match(text: Any) -> str:
    if not isinstance(text, str):
        return ''
    return re.sub(r'[^0-9A-Za-z一-鿿]+', '', text).lower()


def normalize_question_text(text: str) -> str:
    text = text or ''
    text = re.sub(r'更多资料请加.*$', '', text)
    text = text.replace('（ ）', '').replace('（）', '').replace('( )', '').replace('()', '')
    text = text.replace('“', '').replace('”', '').replace('‘', '').replace('’', '')
    text = text.replace('《', '').replace('》', '')
    text = re.sub(r'\s+', '', text)
    text = re.sub(r'[，。；：、,.!?？!（）()\[\]<>【】…\-—"“”‘’·]', '', text)
    return text.lower()


def simplify_for_similarity(text: str) -> str:
    text = normalize_question_text(text)
    for word in STOPWORDS:
        text = text.replace(word, '')
    return text


def bigrams(text: str) -> set[str]:
    return {text[i:i + 2] for i in range(len(text) - 1)} if len(text) >= 2 else ({text} if text else set())


def keyword_chunks(text: str) -> List[str]:
    text = re.sub(r'更多资料请加.*$', '', text or '')
    chunks = re.findall(r'[0-9A-Za-z一-鿿]{2,}', text)
    chunks = [chunk for chunk in chunks if chunk not in STOPWORDS]
    chunks.sort(key=len, reverse=True)
    return chunks


def similarity_score(query: str, candidate: str) -> float:
    q_norm = normalize_question_text(query)
    c_norm = normalize_question_text(candidate)
    if not q_norm or not c_norm:
        return 0.0

    if q_norm == c_norm:
        return 1.0

    q_simple = simplify_for_similarity(query)
    c_simple = simplify_for_similarity(candidate)
    seq = SequenceMatcher(None, q_norm, c_norm).ratio()
    seq_simple = SequenceMatcher(None, q_simple, c_simple).ratio() if q_simple and c_simple else 0.0

    bg_q = bigrams(q_norm)
    bg_c = bigrams(c_norm)
    bg = len(bg_q & bg_c) / max(1, len(bg_q | bg_c)) if bg_q and bg_c else 0.0

    contains = q_norm in c_norm or c_norm in q_norm
    score = seq * 0.45 + seq_simple * 0.35 + bg * 0.20
    if contains:
        score += 0.08

    q_keywords = keyword_chunks(query)[:3]
    c_norm_simple = normalize_question_text(candidate)
    matched_keywords = sum(1 for item in q_keywords if item and normalize_question_text(item) in c_norm_simple)
    if matched_keywords >= 2:
        score += 0.06
    elif matched_keywords == 1:
        score += 0.02

    return min(score, 1.0)


def parse_option_blob(blob: str) -> Optional[List[str]]:
    text = ' '.join(blob.split())
    text = re.sub(r'^\d+\s+', '', text)
    match = re.search(r'A[\.．、]?\s*(.*?)\s*B[\.．、]?\s*(.*?)\s*C[\.．、]?\s*(.*?)\s*D[\.．、]?\s*(.*)$', text, re.S)
    if not match:
        return None
    options = [item.strip() for item in match.groups()]
    if not all(options):
        return None
    return options


def extract_choice_letters(text: Any) -> Optional[List[str]]:
    if not isinstance(text, str):
        return None
    letters = re.findall(r'[A-F]', text.upper())
    deduped: List[str] = []
    for letter in letters:
        if letter not in deduped:
            deduped.append(letter)
    return deduped or None


def choice_letters_to_answer(letters: List[str], q_type: str) -> Tuple[str, Any]:
    indices = [ord(letter) - 65 for letter in letters if 'A' <= letter <= 'F']
    if not indices:
        return q_type, None
    if q_type == 'multi' or len(indices) > 1:
        return 'multi', indices
    return 'single', indices[0]


def infer_choice_answer_from_analysis(analysis: Any, options: Optional[List[str]], q_type: str, stem: str = '') -> Tuple[str, Any]:
    if not isinstance(analysis, str):
        return q_type, None

    text = analysis.strip()
    if not text or not options:
        return q_type, None

    head_clause = re.split(r'[。；;!！?？]', text, maxsplit=1)[0]
    normalized_head = normalize_text_for_match(head_clause)
    if normalized_head:
        head_hits = []
        for index, option in enumerate(options):
            normalized_option = normalize_text_for_match(option)
            if normalized_option and normalized_option in normalized_head:
                head_hits.append(index)
        if q_type == 'single' and len(head_hits) == 1:
            return choice_letters_to_answer([chr(head_hits[0] + 65)], 'single')
        if q_type == 'multi' and head_hits:
            return choice_letters_to_answer([chr(index + 65) for index in head_hits], 'multi')

    normalized_analysis = normalize_text_for_match(text)
    negative_markers = ['其他选项', '其余选项', '均不', '错误', '不正确', '不准确', '不符合', '不选', '均非', '表述错误']
    negative_positions = [pos for pos in (normalized_analysis.find(normalize_text_for_match(item)) for item in negative_markers) if pos >= 0]
    cutoff = min(negative_positions) if negative_positions else len(normalized_analysis)

    positions: List[Tuple[int, int]] = []
    for index, option in enumerate(options):
        normalized_option = normalize_text_for_match(option)
        if not normalized_option:
            continue
        pos = normalized_analysis.find(normalized_option)
        if pos >= 0:
            positions.append((index, pos))

    early_indices = [index for index, pos in positions if pos < cutoff and pos < 80]
    if early_indices:
        return choice_letters_to_answer([chr(index + 65) for index in early_indices], q_type)

    if len(positions) == 1:
        return choice_letters_to_answer([chr(positions[0][0] + 65)], q_type)

    return q_type, None


def answer_to_letters(answer: Any, q_type: str) -> Optional[str]:
    if isinstance(answer, list):
        letters = ''.join(chr(int(index) + 65) for index in answer)
        return letters or None
    if isinstance(answer, int):
        return chr(answer + 65)
    if isinstance(answer, str):
        letters = extract_choice_letters(answer)
        return ''.join(letters) if letters else None
    return None


def load_history() -> Dict[str, Any]:
    return json.loads(HISTORY_PATH.read_text(encoding='utf-8'))


def strip_page_prefix(line: str) -> str:
    return re.sub(r'^\d+\s+(?=(?:\d+[.．、]|[A-D][.．、]|正确答案))', '', line)


def parse_ahjzu() -> List[SourceQuestion]:
    text = '\n'.join((page.extract_text() or '') for page in PdfReader(str(AHJZU_PDF)).pages)
    lines: List[str] = []
    for raw in text.splitlines():
        line = strip_page_prefix(raw.strip())
        if not line or line.isdigit():
            continue
        lines.append(line)

    records: List[SourceQuestion] = []
    mode: Optional[str] = None
    i = 0
    while i < len(lines):
        line = lines[i]
        if '单项选择题' in line or '单选' in line:
            mode = 'single'
            i += 1
            continue
        if '多项选择题' in line or '多选' in line:
            mode = 'multi'
            i += 1
            continue
        if '判断题' in line:
            mode = 'judge'
            i += 1
            continue

        match = re.match(r'^(\d+)[.．、]\s*(.*)', line)
        if mode in {'single', 'multi'} and match:
            question = match.group(2).strip()
            parts: List[str] = []
            answer: Optional[str] = None
            i += 1
            while i < len(lines):
                current = lines[i]
                if re.match(r'^(单项选择题|多项选择题|判断题)', current) or re.match(r'^\d+[.．、]\s*', current):
                    break
                if current.startswith('正确答案'):
                    answer_match = re.search(r'正确答案\s*[：:]\s*([A-D]+)', current)
                    if answer_match:
                        answer = answer_match.group(1)
                    i += 1
                    break
                parts.append(current)
                i += 1
            options = parse_option_blob(' '.join(parts))
            if options:
                records.append(SourceQuestion('ahjzu', mode, question, normalize_question_text(question), options, answer))
            continue
        i += 1
    return records


def parse_jnnu() -> List[SourceQuestion]:
    text = JNNU_HTML.read_text(encoding='utf-8-sig', errors='ignore')
    text = re.sub(r'(?is)<script.*?</script>', ' ', text)
    text = re.sub(r'(?is)<style.*?</style>', ' ', text)
    text = re.sub(r'(?is)<[^>]+>', '\n', text)
    text = html.unescape(text)
    lines = [re.sub(r'\s+', ' ', line).strip() for line in text.splitlines()]
    lines = [line for line in lines if line]

    records: List[SourceQuestion] = []
    mode: Optional[str] = None
    i = 0
    while i < len(lines):
        line = lines[i]
        if '单项选择题' in line or '单选' in line:
            mode = 'single'
            i += 1
            continue
        if '多项选择题' in line or '多选' in line:
            mode = 'multi'
            i += 1
            continue
        if '判断题' in line or '判断' == line:
            mode = 'judge'
            i += 1
            continue

        match = re.match(r'^(\d+)[.．、]\s*(.*)', line)
        if mode in {'single', 'multi'} and match:
            question = match.group(2).strip()
            answer: Optional[str] = None
            inline_answer = re.search(r'([A-D]{1,4})\s*[)）]$', question)
            if inline_answer:
                answer = inline_answer.group(1)
                question = question[:inline_answer.start()].rstrip()

            i += 1
            parts: List[str] = []
            while i < len(lines):
                current = lines[i]
                if ('单选' in current or '单项选择题' in current or '多选' in current or '多项选择题' in current or '判断' in current) and len(current) < 20:
                    break
                if re.match(r'^\d+[.．、]\s*', current):
                    break
                parts.append(current)
                i += 1

            options = parse_option_blob(' '.join(parts))
            if options:
                records.append(SourceQuestion('jnnu', mode, question, normalize_question_text(question), options, answer))
            continue
        i += 1
    return records


def has_broken_options(question: Dict[str, Any]) -> bool:
    options = question.get('options', []) or []
    if len(options) < 4:
        return True
    return any('更多资料请加' in item or re.search(r'\d+[.．、]\s*近代中国', item) for item in options)


def best_match(question: Dict[str, Any], sources_by_type: Dict[str, List[SourceQuestion]]) -> Optional[MatchResult]:
    candidates = sources_by_type.get(question['type'], [])
    if not candidates:
        return None

    q_norm = normalize_question_text(question['content'])
    existing_letters = answer_to_letters(question.get('answer'), question['type'])
    ranked: List[Tuple[float, float, SourceQuestion, bool, bool, Optional[str], Any]] = []
    for candidate in candidates:
        exact = q_norm == candidate.norm
        contains = bool(q_norm and candidate.norm and (q_norm in candidate.norm or candidate.norm in q_norm))
        score = similarity_score(question['content'], candidate.content)
        inferred_type, inferred_value = infer_choice_answer_from_analysis(
            question.get('analysis'), candidate.options, question['type'], question.get('content', '')
        )
        inferred_letters = answer_to_letters(inferred_value, inferred_type) if inferred_value is not None else None

        fit = score
        if candidate.answer:
            fit += 0.05
        if inferred_letters:
            if inferred_type == question['type']:
                fit += 0.08
            else:
                fit -= 0.12
            if existing_letters and inferred_letters == existing_letters:
                fit += 0.03
        if candidate.answer and inferred_letters and candidate.answer == inferred_letters:
            fit += 0.08
        if question['type'] == 'single' and inferred_letters and len(inferred_letters) > 1 and not candidate.answer:
            fit -= 0.10
        if question['type'] == 'multi' and candidate.answer and len(candidate.answer) == 1 and not inferred_letters:
            fit -= 0.06

        ranked.append((fit, score, candidate, exact, contains, inferred_letters, inferred_value))

    ranked.sort(key=lambda item: (item[0], item[1]), reverse=True)
    best = ranked[0]
    second_fit = ranked[1][0] if len(ranked) > 1 else 0.0
    return MatchResult(best[2], best[1], best[0] - second_fit, best[3], best[4], best[0], best[5], best[6])


def should_accept(question: Dict[str, Any], match: MatchResult) -> bool:
    score = match.score
    fit = match.fit
    margin = match.margin
    source = match.candidate.source
    inferred_letters = match.analysis_letters

    if not match.candidate.answer and not inferred_letters:
        return False
    if question['type'] == 'single' and inferred_letters and len(inferred_letters) > 1 and not match.candidate.answer:
        return False
    if question['type'] == 'multi' and inferred_letters and len(inferred_letters) < 2 and not match.candidate.answer:
        return False

    if match.exact and fit >= 0.82 and (match.candidate.answer or not inferred_letters or len(inferred_letters) <= 1):
        return True
    if match.contains and fit >= 0.84 and margin >= 0.04:
        return True
    if source == 'ahjzu' and fit >= 0.82 and score >= 0.68 and margin >= 0.04:
        return True
    if source == 'jnnu' and fit >= 0.88 and score >= 0.72 and margin >= 0.06:
        return True
    if inferred_letters and fit >= 0.78 and score >= 0.68 and margin >= 0.05:
        return True
    if match.candidate.answer and fit >= 0.78 and score >= 0.68 and margin >= 0.05:
        return True
    if score >= 0.92 and fit >= 0.82:
        return True
    return False


def normalize_answer_value(q_type: str, letters: str) -> Any:
    if q_type == 'multi' or len(letters) > 1:
        return [ord(letter) - 65 for letter in letters]
    return ord(letters[0]) - 65


def load_sources() -> List[SourceQuestion]:
    sources: List[SourceQuestion] = []
    if AHJZU_PDF.exists():
        sources.extend(parse_ahjzu())
    if JNNU_HTML.exists():
        sources.extend(parse_jnnu())
    return sources


def repair_history(apply_changes: bool) -> Tuple[List[PatchRecord], List[str], List[PatchRecord]]:
    history = load_history()
    sources = load_sources()
    sources_by_type: Dict[str, List[SourceQuestion]] = defaultdict(list)
    for item in sources:
        sources_by_type[item.q_type].append(item)

    patched: List[PatchRecord] = []
    answer_fixes: List[PatchRecord] = []
    unresolved: List[str] = []

    for question in history['questions']:
        if question.get('type') not in {'single', 'multi'}:
            continue

        broken = has_broken_options(question)
        match = best_match(question, sources_by_type)

        existing_letters = answer_to_letters(question.get('answer'), question['type'])
        inferred_letters = match.analysis_letters if match else None
        inferred_value = match.analysis_value if match else None

        if broken:
            if not match or not should_accept(question, match):
                unresolved.append(question['id'])
                continue

            old_options = list(question.get('options', []) or [])
            old_answer = question.get('answer')
            question['options'] = match.candidate.options

            answer_method = 'keep-existing'
            new_answer = old_answer
            source_answer = match.candidate.answer
            if source_answer:
                new_answer = source_answer
                answer_method = f'source:{match.candidate.source}'
            elif inferred_letters:
                new_answer = normalize_answer_value(question['type'], inferred_letters)
                answer_method = 'analysis'
            else:
                unresolved.append(question['id'])
                continue

            question['answer'] = new_answer
            patched.append(
                PatchRecord(
                    qid=question['id'],
                    q_type=question['type'],
                    content=question['content'],
                    source=match.candidate.source,
                    score=match.score,
                    margin=match.margin,
                    exact=match.exact,
                    contains=match.contains,
                    old_options=old_options,
                    new_options=match.candidate.options,
                    old_answer=old_answer,
                    new_answer=new_answer,
                    answer_method=answer_method,
                    candidate_content=match.candidate.content,
                )
            )
            continue

        if not match or not match.candidate.answer:
            continue
        if match.score < 0.94:
            continue

        old_letters = answer_to_letters(question.get('answer'), question['type'])
        if not old_letters or old_letters == match.candidate.answer:
            continue

        old_answer = question.get('answer')
        question['answer'] = match.candidate.answer
        answer_fixes.append(
            PatchRecord(
                qid=question['id'],
                q_type=question['type'],
                content=question['content'],
                source=match.candidate.source,
                score=match.score,
                margin=match.margin,
                exact=match.exact,
                contains=match.contains,
                old_options=list(question.get('options', []) or []),
                new_options=list(question.get('options', []) or []),
                old_answer=old_answer,
                new_answer=match.candidate.answer,
                answer_method=f'source:{match.candidate.source}',
                candidate_content=match.candidate.content,
            )
        )

    if apply_changes:
        HISTORY_PATH.write_text(json.dumps(history, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')

    return patched, unresolved, answer_fixes


def render_report(patched: List[PatchRecord], unresolved: List[str], answer_fixes: List[PatchRecord]) -> str:
    lines = [
        '# History Web 修复报告',
        '',
        f'- 通过公开来源补回/替换选项: {len(patched)} 道',
        f'- 通过公开来源修正答案: {len(answer_fixes)} 道',
        f'- 仍未可靠修复: {len(unresolved)} 道',
        '',
        '## 公开来源',
        '',
        '- https://www.ahjzu.edu.cn/_upload/article/files/0b/69/013ffcf6427f8acdea96d385c1ab/2d092d17-b471-40f1-b6bb-b27ec1f78f93.pdf',
        '- https://www.jnnu.edu.cn/my/info/1008/1186.htm',
        '',
        '## 已回填题目',
        '',
    ]

    for item in patched[:200]:
        lines.append(
            f'- `{item.qid}` | `{item.q_type}` | `{item.source}` | score={item.score:.3f} | answer={item.old_answer}->{item.new_answer} | 来源题干: {item.candidate_content}'
        )

    lines.extend(['', '## 已修正答案', ''])
    for item in answer_fixes[:100]:
        lines.append(
            f'- `{item.qid}` | `{item.q_type}` | `{item.source}` | score={item.score:.3f} | answer={item.old_answer}->{item.new_answer} | 来源题干: {item.candidate_content}'
        )

    lines.extend(['', '## 未修复题目', ''])
    for qid in unresolved:
        lines.append(f'- `{qid}`')
    lines.append('')
    return '\n'.join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description='Repair history question bank from public web sources.')
    parser.add_argument('--apply', action='store_true', help='Write changes back to tiku/history.json')
    args = parser.parse_args()

    patched, unresolved, answer_fixes = repair_history(args.apply)
    report = render_report(patched, unresolved, answer_fixes)
    REPORT_PATH.write_text(report, encoding='utf-8')

    print(f'patched={len(patched)}')
    print(f'answer_fixes={len(answer_fixes)}')
    print(f'unresolved={len(unresolved)}')
    for item in patched[:40]:
        print(f'PATCH\t{item.qid}\t{item.source}\t{item.score:.3f}\t{item.answer_method}\t{item.content}\t=>\t{item.candidate_content}')
    for item in answer_fixes[:20]:
        print(f'ANSWER\t{item.qid}\t{item.source}\t{item.score:.3f}\t{item.old_answer}->{item.new_answer}\t{item.content}')
    if unresolved:
        print('UNRESOLVED', ','.join(unresolved[:80]))


if __name__ == '__main__':
    main()
