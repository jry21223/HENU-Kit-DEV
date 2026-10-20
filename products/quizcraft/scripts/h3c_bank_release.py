#!/usr/bin/env python3
"""Reconcile prepared H3C sources into traceable, review-first bank inputs.

This step does not claim that a structurally parsed answer is verified. It
combines source evidence, exact source figures, deduplication and review state
for the separately reviewed explanation pass that creates publishable bank JSON.
"""
from __future__ import annotations

import argparse
from collections import Counter
import hashlib
import json
import re
import sys
from math import hypot
from collections import defaultdict
from pathlib import Path
from typing import Any


EXAM_CODES = ("0192", "0330", "0372", "0382", "0392")
SOURCE_BANKS = {
    "1_H3CNE-GB0-192题库(附答案详解).pdf": ("h3c_gb0192_main", "0192", "主卷"),
    "GB0-330题目202410.pdf": ("h3c_gb0330_202410", "0330", "2024练习"),
    "H3CSE RS+_372.pdf": ("h3c_gb0372_main", "0372", "主卷"),
    "H3CSE RS+_382.pdf": ("h3c_gb0382_main", "0382", "主卷"),
    "H3CSE RS+_392.pdf": ("h3c_gb0392_main", "0392", "主卷"),
    "新增GB0-192（20240707）（52题）.pdf": ("h3c_gb0192_supp_20240707", "0192", "补充练习"),
    "新增GB0-372（20240719）（41题）.pdf": ("h3c_gb0372_supp_20240719", "0372", "补充练习"),
    "新增GB0-382（20240718）（21题）.pdf": ("h3c_gb0382_supp_20240718", "0382", "补充练习"),
    "新增GB0-392（20230518）.pdf": ("h3c_gb0392_supp_20230518", "0392", "补充练习"),
}
BANK_KEYS = tuple(details[0] for details in SOURCE_BANKS.values())


def _exam_code(source_file: str) -> str | None:
    match = re.search(r"(?:GB0[-_]?|RS\+_)(0?192|0?330|0?372|0?382|0?392)", source_file, re.I)
    return match[1].zfill(4) if match else None


def _source_bank(source_file: str) -> tuple[str, str, str]:
    if source_file in SOURCE_BANKS:
        return SOURCE_BANKS[source_file]
    raise ValueError(f"source file has no bank plan: {source_file}")


def _read(path: Path, default: Any = None) -> Any:
    if not path.exists():
        return default
    return json.loads(path.read_text(encoding="utf-8"))


def _norm(value: str) -> str:
    value = re.sub(r"（(?:单|多)项选择题）|\((?:single|multiple) choice\)", "", value, flags=re.I)
    return re.sub(r"\s+", " ", value).strip().casefold()


def _question_identity(question: dict[str, Any]) -> tuple[str, str, tuple[str, ...]]:
    options = question.get("options") or []
    figures = tuple(sorted(re.findall(r"/practice-assets/h3c/([0-9a-f]{64})\.png", question.get("content", ""))))
    # Option labels in explanations and corrections depend on their order.
    # Keep reordered variants instead of silently remapping a reviewed answer.
    return (str(question.get("type", "")), _norm(question.get("content", "")),
            tuple(_norm(str(o)) for o in options) + figures)


def _answer_semantics(question: dict[str, Any]) -> tuple[str, ...] | str | None:
    options = question.get("options") or []
    answer = question.get("answer")
    if type(answer) is int:
        if answer < 0 or answer >= len(options):
            return None
        return (_norm(str(options[answer])),)
    if isinstance(answer, list):
        if not all(type(i) is int and 0 <= i < len(options) for i in answer):
            return None
        return tuple(sorted(_norm(str(options[i])) for i in answer))
    return _norm(str(answer)) if answer is not None else None


def _source_images(question: dict[str, Any]) -> list[str]:
    if isinstance(question.get("source"), dict):
        return list(question["source"].get("images") or [])
    return [str(image.get("asset_path", "")) for image in question.get("images", [])]


def _rect_gap(first: list[float], second: list[float]) -> float:
    dx = max(first[0] - second[2], second[0] - first[2], 0.0)
    dy = max(first[1] - second[3], second[1] - first[3], 0.0)
    return hypot(dx, dy)


def _figure_owner_evidence(
    item: dict[str, Any], figures: list[dict[str, Any]],
    region_index: dict[tuple[str, int], list[dict[str, Any]]],
    source_file: str, source_id: str,
) -> tuple[bool, list[dict[str, Any]]]:
    raw_text = str(item.get("raw_text") or "")
    source = item.get("source") or {}
    source_pages = set(item.get("source_pages") or source.get("pages") or [])
    local_regions = item.get("source_regions") or source.get("regions") or []
    image_records = item.get("images") or []
    evidence = []
    valid = bool(figures)
    for image_index, figure in enumerate(figures):
        page, image_bbox = figure.get("page"), figure.get("bbox")
        asset_id = figure.get("source_asset_id", "")
        marker_present = bool(asset_id and asset_id in raw_text)
        exact_owner_region = any(
            region.get("page") == page and region.get("bbox")
            and len(region["bbox"]) == len(image_bbox or []) == 4
            and all(abs(float(a) - float(b)) <= 0.02 for a, b in zip(region["bbox"], image_bbox or []))
            for region in local_regions
        )
        item_image = image_records[image_index] if image_index < len(image_records) else {}
        direct_image = bool(item_image.get("asset_path") and item_image.get("page") == page
                            and item_image.get("bbox") and image_bbox
                            and len(item_image["bbox"]) == len(image_bbox) == 4
                            and all(abs(float(a) - float(b)) <= 0.02
                                    for a, b in zip(item_image["bbox"], image_bbox)))
        page_regions = region_index.get((source_file, page), [])
        distances = sorted(
            (_rect_gap(image_bbox, region["bbox"]), region["question_id"])
            for region in page_regions if image_bbox and region.get("bbox") and len(region["bbox"]) == 4
        )
        nearest_gap = distances[0][0] if distances else None
        nearest_ids = (sorted({question_id for gap, question_id in distances if abs(gap - nearest_gap) <= 0.5})
                       if distances else [])
        geometry_owner = nearest_ids == [source_id] and nearest_gap is not None and nearest_gap <= 100.0
        page_associated = page in source_pages
        figure_valid = page_associated and (
            direct_image and exact_owner_region or marker_present and geometry_owner
        )
        valid = valid and figure_valid
        evidence.append({
            "source_asset_id": asset_id,
            "page_in_question_span": page_associated,
            "image_bbox_matches_source_region": exact_owner_region,
            "source_image_marker_within_question_block": marker_present,
            "nearest_question_id_by_bbox": nearest_ids,
            "nearest_bbox_gap_points": round(nearest_gap, 3) if nearest_gap is not None else None,
        })
    return valid, evidence


def _has_explanation(text: str) -> bool:
    compact = re.sub(r"\s+", "", text)
    if len(compact) < 20:
        return False
    vague = ("同上", "上题一样", "上面一样", "不再解释", "自己看看", "答案见原卷", "解析生成失败")
    return not any(token in compact for token in vague)


def _valid_corrected_answer(question: dict[str, Any], answer: Any) -> bool:
    kind = question.get("type")
    options = question.get("options") or []
    if kind == "single":
        return type(answer) is int and 0 <= answer < len(options)
    if kind == "multi":
        return (isinstance(answer, list) and bool(answer)
                and all(type(index) is int and 0 <= index < len(options) for index in answer)
                and len(set(answer)) == len(answer))
    if kind == "blank":
        return isinstance(answer, str) and bool(answer.strip())
    if kind == "judge":
        return type(answer) is bool
    return False


def _copy_figure(
    source_dir: Path, source_item: dict[str, Any], image_assets: dict[str, dict[str, Any]],
    asset_out: Path,
) -> list[dict[str, Any]]:
    copied = []
    record_images = source_item.get("images", []) if isinstance(source_item.get("images"), list) else []
    for image_index, asset_id in enumerate(_source_images(source_item)):
        asset = image_assets.get(asset_id)
        if asset:
            original = source_dir / asset["path"]
        elif image_index < len(record_images) and record_images[image_index].get("asset_path"):
            asset = record_images[image_index]
            original = source_dir / asset["asset_path"]
        else:
            continue
        if not original.is_file():
            continue
        data = original.read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        if asset.get("sha256") and digest != asset["sha256"]:
            raise ValueError(f"asset hash mismatch: {original}")
        try:
            from PIL import Image
            import io

            image = Image.open(io.BytesIO(data))
            image.load()
            png = io.BytesIO()
            image.save(png, format="PNG")
            data = png.getvalue()
        except Exception as exc:
            raise ValueError(f"cannot convert exact source figure {original}: {exc}") from exc
        digest = hashlib.sha256(data).hexdigest()
        target = asset_out / f"{digest}.png"
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        copied.append({
            "source_asset_id": asset_id,
            "sha256": digest,
            "path": str(target),
            "page": asset.get("page"),
            "bbox": asset.get("bbox"),
            "source_path": str(original),
        })
    return copied


def reconcile(
    structured_dir: Path,
    output_dir: Path,
    approved_explanations_path: Path | None = None,
    source_root: Path | None = None,
    *, require_all_sources: bool = False,
) -> dict[str, Any]:
    """Combine all prepared records; source/review manifests are never lossy."""
    groups: dict[str, list[dict[str, Any]]] = defaultdict(list)
    all_sources: list[dict[str, Any]] = []
    image_manifest: list[dict[str, Any]] = []
    source_hashes: dict[str, dict[str, Any]] = {}
    staged: dict[str, Path] = {}
    for folder in sorted(structured_dir.iterdir()):
        if not folder.is_dir():
            continue
        sources = _read(folder / "source_questions.json", [])
        bank_data = _read(folder / "bank.json", {"questions": []})
        banks = bank_data.get("questions", []) if isinstance(bank_data, dict) else []
        source_meta = _read(folder / "source.json", {}) or {}
        if not source_meta:
            source_meta = {"file": (sources[0].get("source_file") or sources[0].get("source", {}).get("file"))
                           if sources else None}
        if source_meta.get("file"):
            source_path = (Path(source_root) / source_meta["file"]) if source_root else None
            if source_path:
                if (not source_path.is_file()
                        or source_path.resolve().parent != Path(source_root).resolve()):
                    raise ValueError(f"source file unavailable or outside source root: {source_meta['file']}")
                extracted_hash = source_meta.get("sha256")
                if not isinstance(extracted_hash, str) or not re.fullmatch(r"[0-9a-f]{64}", extracted_hash):
                    raise ValueError(f"extracted source hash missing: {source_meta['file']}")
                if hashlib.sha256(source_path.read_bytes()).hexdigest() != extracted_hash:
                    raise ValueError(f"source hash mismatch: {source_meta['file']}")
        source_hashes[folder.name] = source_meta
        asset_values = _read(folder / "assets.json", []) or []
        asset_by_id = {asset["id"]: asset for asset in asset_values if asset.get("id")}
        region_index: dict[tuple[str, int], list[dict[str, Any]]] = defaultdict(list)
        for region_item in sources:
            region_source = region_item.get("source") or {}
            region_file = (region_item.get("source_file") or region_source.get("file")
                           or source_meta.get("file"))
            region_id = region_item.get("source_question_id") or region_item.get("id")
            for region in region_item.get("source_regions") or region_source.get("regions") or []:
                if region_file and region_id and region.get("page") is not None and region.get("bbox"):
                    region_index[(region_file, region["page"])].append(
                        {"question_id": region_id, "bbox": region["bbox"]})
        bank_by_id = {question["id"]: question for question in banks if question.get("id")}
        for item in sources:
            raw_source = item.get("source") or {}
            source_file = item.get("source_file") or raw_source.get("file") or source_meta.get("file")
            code = _exam_code(str(source_file))
            if not code:
                continue
            bank_key, bank_code, source_kind = _source_bank(str(source_file))
            source_id = item.get("source_question_id") or item.get("id")
            if not source_id:
                continue
            bank_question = bank_by_id.get(item.get("id"))
            reasons = list(item.get("review_reasons", []))
            if not reasons:
                reasons.extend(next((r.get("reasons", []) for r in _read(folder / "review.json", [])
                                     if r.get("id") == item.get("id") or
                                     r.get("source_question_id") == source_id), []))
            copied_figures = _copy_figure(folder, item, asset_by_id, output_dir / "assets")
            if len(copied_figures) != len(_source_images(item)):
                reasons.append("figure_asset_unavailable")
            reliable_image_owner, owner_evidence = _figure_owner_evidence(
                item, copied_figures, region_index, source_file, source_id
            )
            if copied_figures:
                for figure in copied_figures:
                    figure.update({"question_id": source_id, "source_file": source_file,
                                   "ownership_status": "coordinate_and_block_match" if reliable_image_owner else "ambiguous",
                                   "owner_evidence": owner_evidence})
                image_manifest.extend(copied_figures)
                if reliable_image_owner:
                    reasons = [reason for reason in reasons
                               if reason not in {"figure_required", "image_requires_rendering"}]
                else:
                    reasons.append("image_ownership_ambiguous")
            if bank_question:
                bank_question = dict(bank_question)
            elif not [reason for reason in reasons if reason != "figure_required"]:
                try:
                    project_root = Path(__file__).resolve().parents[1]
                    if str(project_root) not in sys.path:
                        sys.path.insert(0, str(project_root))
                    from server import build_standard_bank_data

                    bank_question = build_standard_bank_data(
                        f"H3C GB0-{int(code)} {source_kind}", [item]
                    )["questions"][0]
                except Exception:
                    bank_question = None
            if bank_question and copied_figures and reliable_image_owner:
                refs = "\n".join(
                    f"![题图](/practice-assets/h3c/{figure['sha256']}.png)"
                    for figure in copied_figures
                )
                bank_question["content"] = bank_question["content"].rstrip() + "\n\n" + refs
            if not bank_question:
                reasons.append("not_structurally_normalized")
            has_analysis = _has_explanation(str(item.get("analysis") or ""))
            if not has_analysis:
                reasons.append("needs_independent_explanation")
            entry = {
                "exam_code": code,
                "bank_key": bank_key,
                "source_kind": source_kind,
                "source_question_id": source_id,
                "source_file": source_file,
                "source_pages": item.get("source_pages") or raw_source.get("pages", []),
                "source_number": item.get("number") or raw_source.get("number"),
                "source_segment": raw_source.get("segment"),
                "content": item.get("content"),
                "options": item.get("options"),
                "source_answer": item.get("source_answer"),
                "normalized_answer": bank_question.get("answer") if bank_question else item.get("answer"),
                "source_analysis": item.get("analysis", ""),
                "source_review_reasons": sorted(set(reasons)),
                "source_images": copied_figures,
                "structural_candidate": bank_question,
                "analysis_ready": has_analysis,
            }
            groups[bank_key].append(entry)
            all_sources.append({k: v for k, v in entry.items() if k not in {"structural_candidate"}})
            staged[source_id] = folder

    if require_all_sources and set(groups) != set(BANK_KEYS):
        raise ValueError("complete batch requires all nine original source documents")

    deduplication: list[dict[str, Any]] = []
    reviews: list[dict[str, Any]] = []
    answer_corrections: list[dict[str, Any]] = []
    explanation_worklist: list[dict[str, Any]] = []
    approved_rows: Any = {}
    if approved_explanations_path:
        if Path(approved_explanations_path).is_dir():
            merged = {}
            for path in sorted(Path(approved_explanations_path).glob("approved-*.json")):
                rows = _read(path, [])
                if isinstance(rows, list):
                    for row in rows:
                        if row.get("source_question_id"):
                            merged[row["source_question_id"]] = row
                elif isinstance(rows, dict):
                    merged.update(rows)
            approved_rows = merged
        else:
            approved_rows = _read(approved_explanations_path, {})
    if isinstance(approved_rows, list):
        approved_rows = {row.get("source_question_id"): row for row in approved_rows}
    approved_rows = approved_rows if isinstance(approved_rows, dict) else {}
    held_ids: set[str] = set()
    if approved_explanations_path and Path(approved_explanations_path).is_dir():
        for review_path in sorted(Path(approved_explanations_path).glob("review-*.json")):
            rows = _read(review_path, [])
            if isinstance(rows, dict):
                rows = rows.get("reviews", [])
            for row in rows if isinstance(rows, list) else []:
                question_id = row.get("source_question_id") or row.get("id")
                if question_id:
                    held_ids.add(question_id)
    cross_file_groups: dict[tuple[str, str, tuple[str, ...]], list[dict[str, Any]]] = defaultdict(list)
    for entries in groups.values():
        for entry in entries:
            if entry["structural_candidate"]:
                cross_file_groups[_question_identity(entry["structural_candidate"])].append(entry)
    cross_file_relations = []
    for related in cross_file_groups.values():
        bank_keys = sorted({entry["bank_key"] for entry in related})
        if len(bank_keys) < 2:
            continue
        answer_semantics = {entry["source_question_id"]: _answer_semantics(entry["structural_candidate"])
                            for entry in related}
        conflict = len(set(answer_semantics.values())) > 1 or None in answer_semantics.values()
        ids = [entry["source_question_id"] for entry in related]
        for entry in related:
            cross_file_relations.append({
                "source_question_id": entry["source_question_id"],
                "bank_key": entry["bank_key"],
                "related_source_question_ids": ids,
                "related_bank_keys": bank_keys,
                "answer_semantics_by_source": answer_semantics,
                "status": "cross_file_answer_conflict" if conflict else "cross_file_duplicate_kept",
            })
            if conflict:
                held_ids.add(entry["source_question_id"])
                reviews.append({"source_question_id": entry["source_question_id"],
                                "exam_code": entry["exam_code"],
                                "bank_key": entry["bank_key"],
                                "reasons": ["cross_file_answer_conflict"]})
    banks_out: dict[str, Any] = {}
    for bank_key in BANK_KEYS:
        _, code, source_kind = next(details for details in SOURCE_BANKS.values() if details[0] == bank_key)
        bucket = groups.get(bank_key, [])
        dedup_groups: dict[tuple[str, str, tuple[str, ...]], list[dict[str, Any]]] = defaultdict(list)
        for entry in bucket:
            candidate = entry["structural_candidate"]
            if candidate:
                dedup_groups[_question_identity(candidate)].append(entry)
            else:
                deduplication.append({"exam_code": code, "bank_key": bank_key,
                                      "source_question_id": entry["source_question_id"],
                                      "canonical_source_question_id": None,
                                      "status": "review_only"})
                reviews.append({"source_question_id": entry["source_question_id"],
                                "exam_code": code, "bank_key": bank_key,
                                "reasons": entry["source_review_reasons"]})
        accepted = []
        for same_question in dedup_groups.values():
            ids = [item["source_question_id"] for item in same_question]
            if held_ids.intersection(ids):
                for item in same_question:
                    deduplication.append({"exam_code": code, "bank_key": bank_key,
                                          "source_question_id": item["source_question_id"],
                                          "canonical_source_question_id": None,
                                          "related_source_question_ids": ids,
                                          "status": "review_only_by_review_file"})
                    reviews.append({"source_question_id": item["source_question_id"],
                                    "exam_code": code, "bank_key": bank_key,
                                    "reasons": ["held_by_review_file"]})
                continue
            answer_sets = {_answer_semantics(item["structural_candidate"]) for item in same_question}
            if len(answer_sets) > 1 or None in answer_sets:
                for item in same_question:
                    reviews.append({"source_question_id": item["source_question_id"],
                                    "exam_code": code, "bank_key": bank_key,
                                    "reasons": sorted(set(item["source_review_reasons"] + ["duplicate_answer_conflict"]))})
                for item in same_question:
                    deduplication.append({"exam_code": code, "bank_key": bank_key,
                                          "source_question_id": item["source_question_id"],
                                          "canonical_source_question_id": None,
                                          "related_source_question_ids": ids,
                                          "status": "conflicting_answers"})
                continue
            canonical = same_question[0]
            for item in same_question:
                deduplication.append({"exam_code": code, "bank_key": bank_key,
                                      "source_question_id": item["source_question_id"],
                                      "canonical_source_question_id": canonical["source_question_id"],
                                      "related_source_question_ids": ids,
                                      "status": "canonical" if item is canonical else "duplicate"})
            blocking = [reason for item in same_question for reason in item["source_review_reasons"]
                        if reason != "needs_independent_explanation"]
            if blocking:
                for item in same_question:
                    if item["source_review_reasons"]:
                        reviews.append({"source_question_id": item["source_question_id"],
                                        "exam_code": code, "bank_key": bank_key,
                                        "reasons": item["source_review_reasons"]})
                continue
            approval = next((approved_rows[item["source_question_id"]]
                                      for item in same_question
                                      if isinstance(approved_rows.get(item["source_question_id"]), dict)
                                      and approved_rows[item["source_question_id"]].get("reviewed") is True
                                      and isinstance(approved_rows[item["source_question_id"]].get("analysis"), str)
                                      and _has_explanation(approved_rows[item["source_question_id"]]["analysis"])), None)
            if not approval:
                for item in same_question:
                    explanation_worklist.append({
                        "exam_code": code,
                        "bank_key": bank_key,
                        "source_question_id": item["source_question_id"],
                        "source_file": item["source_file"],
                        "source_pages": item["source_pages"],
                        "question": item["structural_candidate"],
                        "source_analysis": item["source_analysis"],
                    })
                    reviews.append({"source_question_id": item["source_question_id"],
                                    "exam_code": code, "bank_key": bank_key,
                                    "reasons": ["needs_independent_explanation"]})
                continue
            # Source explanation is retained as an audit draft; a reviewed,
            # standalone explanation is required before final bank release.
            candidate = dict(canonical["structural_candidate"])
            if "corrected_answer" in approval:
                corrected_answer = approval["corrected_answer"]
                evidence = approval.get("evidence")
                correction_reason = approval.get("correction_reason")
                if (not _valid_corrected_answer(candidate, corrected_answer)
                        or not correction_reason
                        or not isinstance(evidence, list) or not evidence):
                    reviews.append({"source_question_id": canonical["source_question_id"],
                                    "exam_code": code, "bank_key": bank_key,
                                    "reasons": ["invalid_or_unsupported_corrected_answer"]})
                    continue
                answer_corrections.append({
                    "source_question_id": canonical["source_question_id"],
                    "approval_source_question_id": approval.get("source_question_id"),
                    "bank_key": bank_key,
                    "source_file": canonical["source_file"],
                    "source_pages": canonical["source_pages"],
                    "source_answer": canonical["source_answer"],
                    "original_answer": candidate["answer"],
                    "corrected_answer": corrected_answer,
                    "correction_reason": correction_reason,
                    "reviewed_by": approval.get("reviewed_by"),
                    "evidence": evidence,
                })
                candidate["answer"] = corrected_answer
            candidate["analysis"] = approval["analysis"].strip()
            if isinstance(candidate.get("chapter"), str):
                candidate["chapter"] = re.sub(r"\bGB0-0(?=\d{3}\b)", "GB0-", candidate["chapter"])
            accepted.append(candidate)
        if accepted:
            project_root = Path(__file__).resolve().parents[1]
            if str(project_root) not in sys.path:
                sys.path.insert(0, str(project_root))
            from server import build_standard_bank_data

            bank = build_standard_bank_data(f"H3C GB0-{int(code)} {source_kind}", accepted)
        else:
            bank = {"meta": {"name": f"H3C GB0-{int(code)} {source_kind}", "total": 0, "chapter_count": 0}, "questions": []}
        banks_out[bank_key] = bank
    metadata_by_file = {value.get("file"): value for value in source_hashes.values() if value.get("file")}
    bank_plan = []
    for source_file, (bank_key, code, source_kind) in SOURCE_BANKS.items():
        entries = groups.get(bank_key, [])
        bank_plan.append({
            "bank_key": bank_key,
            "exam_code": code,
            "source_kind": source_kind,
            "source_file": source_file,
            "source_sha256": metadata_by_file.get(source_file, {}).get("sha256"),
            "source_question_count": len(entries),
            "structural_candidate_count": sum(entry["structural_candidate"] is not None for entry in entries),
            "same_file_duplicate_count": sum(
                row.get("bank_key") == bank_key and row.get("status") == "duplicate"
                for row in deduplication
            ),
            "reviewed_bank_question_count": banks_out[bank_key]["meta"]["total"],
            "reviewed_source_question_ids": [question["id"] for question in banks_out[bank_key]["questions"]],
        })
    output_dir.mkdir(parents=True, exist_ok=True)
    import_dir = output_dir / "workshop-imports"
    import_dir.mkdir(exist_ok=True)
    for entry in bank_plan:
        key = entry["bank_key"]
        destination = import_dir / f"{key}.json"
        questions = banks_out[key]["questions"]
        if not questions:
            destination.unlink(missing_ok=True)
            continue
        source_hash = entry["source_sha256"]
        if not isinstance(source_hash, str) or not re.fullmatch(r"[0-9a-f]{64}", source_hash):
            raise ValueError(f"source hash required for import template: {key}")
        imported = []
        for question in questions:
            row = {field: question[field]
                   for field in ("type", "chapter_id", "chapter", "content", "options", "answer", "analysis")
                   if field in question and question[field] is not None}
            row["source_question_id"] = question["id"]
            imported.append(row)
        # New-bank templates still require a live lifecycle read before sending.
        template = {"expected_version": 1, "source_sha256": source_hash, "questions": imported}
        encoded = json.dumps(template, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        if len(encoded) > 2 << 20:
            raise ValueError(f"import template exceeds the 2 MiB management limit: {key}")
        destination.write_bytes(encoded)
        entry["workshop_import_template"] = f"workshop-imports/{key}.json"
        entry["workshop_import_template_sha256"] = hashlib.sha256(encoded).hexdigest()
        entry["workshop_import_template_bytes"] = len(encoded)
    for key, bank in banks_out.items():
        (output_dir / f"{key}.json").write_text(json.dumps(bank, ensure_ascii=False, indent=2), encoding="utf-8")
    for filename, value in (("source-map.json", all_sources), ("deduplication-map.json", deduplication),
                            ("review.json", reviews), ("image-assets.json", image_manifest),
                            ("source-files.json", source_hashes),
                            ("explanation-worklist.json", explanation_worklist),
                            ("cross-file-relations.json", cross_file_relations),
                            ("bank-plan.json", bank_plan)):
        (output_dir / filename).write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")
    (output_dir / "answer-corrections.json").write_text(
        json.dumps(answer_corrections, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    authoring_dir = output_dir.parent / "authoring"
    authoring_dir.mkdir(parents=True, exist_ok=True)
    for code in EXAM_CODES:
        scoped = [entry for entry in explanation_worklist if entry["exam_code"] == code]
        worklist_path = authoring_dir / f"worklist-{code}.json"
        if not worklist_path.exists():
            worklist_path.write_text(json.dumps(scoped, ensure_ascii=False, indent=2), encoding="utf-8")
    by_bank_dir = authoring_dir / "by-bank"
    by_bank_dir.mkdir(exist_ok=True)
    for bank_key in BANK_KEYS:
        scoped = [entry for entry in explanation_worklist if entry["bank_key"] == bank_key]
        worklist_path = by_bank_dir / f"worklist-{bank_key}.json"
        if not worklist_path.exists():
            worklist_path.write_text(json.dumps(scoped, ensure_ascii=False, indent=2), encoding="utf-8")
    bank_counts = Counter(entry["bank_key"] for entry in all_sources)
    pending_counts = Counter(entry["bank_key"] for entry in explanation_worklist)
    review_counts = Counter(reason for entry in reviews for reason in entry["reasons"])
    image_owners = Counter(asset["ownership_status"] for asset in image_manifest)
    report = [
        "# H3C 题库整理状态",
        "",
        "此目录是可审计的来源整理结果。每份原始 PDF 都保留单独题库入口；跨文件重复题保持独立题目并在关联清单中互相指向。题库只会包含独立解析已复核且未被 review 阻止的题目。",
        "",
        "## 来源数量",
        "",
        "| 独立题库 key | 结构来源条目 | 解析待写/待核对候选 | 最终 bank 条目 |",
        "| --- | ---: | ---: | ---: |",
    ]
    for bank_key in BANK_KEYS:
        _, code, source_kind = next(details for details in SOURCE_BANKS.values() if details[0] == bank_key)
        report.append(f"| {bank_key} (GB0-{code} {source_kind}) | {bank_counts[bank_key]} | {pending_counts[bank_key]} | {banks_out[bank_key]['meta']['total']} |")
    report.extend([
        "",
        f"共 {len(source_hashes)} 份原始 PDF、{len(all_sources)} 个来源题项，{len(deduplication)} 条逐题去重状态映射。",
        f"导出 {len(image_manifest)} 个按 SHA-256 命名的原图 PNG；归属状态：{dict(image_owners)}。图片归属用源marker、PDF页/坐标与同页题目文本区域核验。",
        "",
        "## 复核队列",
        "",
        f"独立解析工作清单包含 {len(explanation_worklist)} 题。其余题项缺答案/选项、答案冲突、题图归属不确定或结构解析失败，见 `review.json`。",
        "",
    ])
    report.extend(f"- `{reason}`：{count} 条复核记录" for reason, count in sorted(review_counts.items()))
    report.extend([
        "",
        "原文解析保留在 `source-map.json` 与工作清单，仅作为来源证据。`cross-file-relations.json` 记录跨卷重题而不合并。批准解析仍需按 `source_question_id` 提供，并未被 `review-*.json` hold。",
        "",
    ])
    (output_dir / "PREPARATION-STATUS.md").write_text("\n".join(report), encoding="utf-8")
    return {"banks": banks_out, "source_map": all_sources, "deduplication": deduplication,
            "review": reviews, "image_assets": image_manifest, "source_files": source_hashes,
            "cross_file_relations": cross_file_relations, "bank_plan": bank_plan,
            "explanation_worklist": explanation_worklist}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--structured", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--approved-explanations", type=Path)
    parser.add_argument("--source-root", type=Path, required=True,
                        help="original PDFs; their hashes must match extraction metadata")
    args = parser.parse_args()
    result = reconcile(args.structured, args.output, args.approved_explanations, args.source_root,
                       require_all_sources=True)
    print(json.dumps({"bank_counts": {key: bank["meta"]["total"] for key, bank in result["banks"].items()},
                      "sources": len(result["source_map"]), "review_items": len(result["review"]),
                      "images": len(result["image_assets"])}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
