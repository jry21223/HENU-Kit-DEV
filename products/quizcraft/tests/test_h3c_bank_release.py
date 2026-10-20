import json
import hashlib

import pytest

from scripts.h3c_bank_release import reconcile


def make_structured_source(root):
    source = root / "gb0330_main"
    source.mkdir()
    question = {
        "id": "gb0330_main_s01_q0001",
        "number": "1",
        "type": "single",
        "content": "IPv6 默认路由的前缀长度是多少？",
        "options": ["/0", "/64"],
        "answer": 1,
        "analysis": "",
        "source_answer": "B",
        "source_file": "GB0-330题目202410.pdf",
        "source_pages": [1],
        "review_reasons": [],
    }
    (source / "source_questions.json").write_text(json.dumps([question]), encoding="utf-8")
    (source / "bank.json").write_text(json.dumps({"questions": [question]}), encoding="utf-8")
    (source / "review.json").write_text("[]", encoding="utf-8")
    (source / "source.json").write_text(json.dumps({
        "file": "GB0-330题目202410.pdf", "sha256": hashlib.sha256(b"reviewed original").hexdigest(),
        "question_count": 1,
    }), encoding="utf-8")


def test_structural_candidate_waits_for_an_independent_explanation(tmp_path):
    structured, output = tmp_path / "structured", tmp_path / "output"
    structured.mkdir()
    make_structured_source(structured)

    pending = reconcile(structured, output)

    assert pending["banks"]["h3c_gb0330_202410"]["questions"] == []
    assert len(pending["source_map"]) == 1
    assert pending["explanation_worklist"][0]["source_question_id"] == "gb0330_main_s01_q0001"
    assert pending["review"][0]["reasons"] == ["needs_independent_explanation"]

    authoring = tmp_path / "authoring"
    authoring.mkdir(exist_ok=True)
    approved = [{
        "source_question_id": "gb0330_main_s01_q0001",
        "analysis": "IPv6 默认路由用全零前缀 ::/0 表示，因此前缀长度是 0；/64 是常见子网前缀，不是默认路由。",
        "reviewed": True,
        "reviewed_by": "Luna",
        "corrected_answer": 0,
        "correction_reason": "核对源答案和规范后发现原选项索引错误。",
        "evidence": ["RFC 4291 section 2.5.2"],
    }]
    (authoring / "approved-0330.json").write_text(json.dumps(approved), encoding="utf-8")
    released = reconcile(structured, output, authoring)

    bank = released["banks"]["h3c_gb0330_202410"]
    assert bank["meta"]["total"] == 1
    assert bank["questions"][0]["id"] == "gb0330_main_s01_q0001"
    assert "/0" in bank["questions"][0]["analysis"]
    assert bank["questions"][0]["answer"] == 0

    corrections = json.loads((output / "answer-corrections.json").read_text())
    assert corrections[0]["source_question_id"] == "gb0330_main_s01_q0001"
    assert corrections[0]["original_answer"] == 1
    assert corrections[0]["corrected_answer"] == 0
    assert corrections[0]["evidence"] == ["RFC 4291 section 2.5.2"]
    template = json.loads((output / "workshop-imports/h3c_gb0330_202410.json").read_text())
    assert template["expected_version"] == 1
    assert template["questions"][0]["source_question_id"] == bank["questions"][0]["id"]
    assert "stats" not in template["questions"][0]

    (authoring / "review-0330.json").write_text(json.dumps([{
        "source_question_id": "gb0330_main_s01_q0001",
        "reasons": ["answer_conflict"],
    }]), encoding="utf-8")
    held = reconcile(structured, output, authoring)
    assert held["banks"]["h3c_gb0330_202410"]["questions"] == []


def test_source_change_after_extraction_cannot_be_relabelled_as_a_new_hash(tmp_path):
    structured, output, originals = (tmp_path / name for name in ("structured", "output", "originals"))
    structured.mkdir()
    originals.mkdir()
    make_structured_source(structured)
    name = "GB0-330题目202410.pdf"
    (structured / "gb0330_main/source.json").write_text(json.dumps({
        "file": name, "sha256": hashlib.sha256(b"reviewed original").hexdigest(),
    }))
    (originals / name).write_bytes(b"different source")
    with pytest.raises(ValueError, match="source hash mismatch"):
        reconcile(structured, output, source_root=originals)


def test_complete_batch_gate_rejects_a_missing_original_document(tmp_path):
    structured = tmp_path / "structured"
    structured.mkdir()
    make_structured_source(structured)
    with pytest.raises(ValueError, match="nine original"):
        reconcile(structured, tmp_path / "output", require_all_sources=True)


@pytest.mark.parametrize("analysis", ["   ", "正确答案见原卷。", "略"])
def test_approval_does_not_make_an_empty_or_generic_explanation_publishable(tmp_path, analysis):
    structured, output = tmp_path / "structured", tmp_path / "output"
    structured.mkdir()
    make_structured_source(structured)
    approved = tmp_path / "approved.json"
    approved.write_text(json.dumps([{
        "source_question_id": "gb0330_main_s01_q0001", "reviewed": True,
        "analysis": analysis, "reviewed_by": "Luna",
    }]))
    result = reconcile(structured, output, approved)
    assert result["banks"]["h3c_gb0330_202410"]["questions"] == []


def test_verified_source_figure_can_leave_the_old_rendering_quarantine(tmp_path):
    from PIL import Image

    structured, output = tmp_path / "structured", tmp_path / "output"
    structured.mkdir()
    make_structured_source(structured)
    folder = structured / "gb0330_main"
    (folder / "bank.json").unlink()
    question = json.loads((folder / "source_questions.json").read_text())[0]
    question["content"] = "根据题图，IPv6 默认路由的前缀长度是多少？"
    box = [10, 20, 100, 80]
    figure = folder / "diagram.png"
    Image.new("RGB", (90, 60), "white").save(figure)
    question["images"] = [{"asset_path": "diagram.png", "page": 1, "bbox": box}]
    question["source_regions"] = [{"page": 1, "bbox": box}]
    question["review_reasons"] = ["image_requires_rendering"]
    (folder / "source_questions.json").write_text(json.dumps([question]))
    approved = tmp_path / "approved.json"
    approved.write_text(json.dumps([{
        "source_question_id": question["id"], "reviewed": True,
        "analysis": "默认路由的前缀长度为零，图中的 ::/0 覆盖全部 IPv6 目的地址，/64 只覆盖一个子网。",
        "reviewed_by": "Luna", "corrected_answer": 0,
        "correction_reason": "默认路由是 ::/0。", "evidence": ["RFC 4291 section 2.5.2"],
    }]))
    result = reconcile(structured, output, approved)
    bank = result["banks"]["h3c_gb0330_202410"]
    assert len(bank["questions"]) == 1
    assert "![题图](/practice-assets/h3c/" in bank["questions"][0]["content"]
    assert bank["questions"][0]["answer"] == 0

    question["images"].append({"asset_path": "missing.png", "page": 1, "bbox": box})
    (folder / "source_questions.json").write_text(json.dumps([question]))
    incomplete = reconcile(structured, output, approved)
    assert incomplete["banks"]["h3c_gb0330_202410"]["questions"] == []
    assert any("figure_asset_unavailable" in row["reasons"] for row in incomplete["review"])


def test_unreviewed_duplicate_correction_cannot_override_an_approved_answer(tmp_path):
    structured, output = tmp_path / "structured", tmp_path / "output"
    structured.mkdir()
    make_structured_source(structured)
    folder = structured / "gb0330_main"
    first = json.loads((folder / "source_questions.json").read_text())[0]
    first["answer"] = 0
    second = dict(first, id="gb0330_main_s01_q0002", number="2")
    for name, data in (("source_questions.json", [first, second]),
                       ("bank.json", {"questions": [first, second]})):
        (folder / name).write_text(json.dumps(data))
    approved = tmp_path / "approved.json"
    approved.write_text(json.dumps([
        {"source_question_id": first["id"], "reviewed": True,
         "analysis": "IPv6 默认路由的前缀为 ::/0，因此长度是 0，而 /64 是普通子网的前缀长度。"},
        {"source_question_id": second["id"], "reviewed": False,
         "corrected_answer": 1, "correction_reason": "unreviewed draft",
         "evidence": ["unreviewed draft"]},
    ]))
    result = reconcile(structured, output, approved)
    questions = result["banks"]["h3c_gb0330_202410"]["questions"]
    assert len(questions) == 1
    assert questions[0]["answer"] == 0


def test_reordered_options_keep_separate_answer_and_explanation_identity(tmp_path):
    structured, output = tmp_path / "structured", tmp_path / "output"
    structured.mkdir()
    make_structured_source(structured)
    folder = structured / "gb0330_main"
    first = json.loads((folder / "source_questions.json").read_text())[0]
    first["answer"] = 0
    second = dict(first, id="gb0330_main_s01_q0002", number="2",
                  options=["/64", "/0"], answer=1)
    for name, data in (("source_questions.json", [first, second]),
                       ("bank.json", {"questions": [first, second]})):
        (folder / name).write_text(json.dumps(data))
    approved = tmp_path / "approved.json"
    approved.write_text(json.dumps([
        {"source_question_id": first["id"], "reviewed": True,
         "analysis": "选项 A 的 ::/0 是 IPv6 默认路由，前缀长度为零；选项 B 的 /64 是子网前缀。"},
        {"source_question_id": second["id"], "reviewed": True,
         "analysis": "选项 B 的 ::/0 是 IPv6 默认路由，前缀长度为零；选项 A 的 /64 是子网前缀。"},
    ]))
    result = reconcile(structured, output, approved)
    questions = result["banks"]["h3c_gb0330_202410"]["questions"]
    assert [(q["id"], q["answer"]) for q in questions] == [(first["id"], 0), (second["id"], 1)]


def test_multiple_nearest_text_regions_of_one_question_are_one_figure_owner(tmp_path):
    from PIL import Image

    structured, output = tmp_path / "structured", tmp_path / "output"
    structured.mkdir()
    make_structured_source(structured)
    folder = structured / "gb0330_main"
    question = json.loads((folder / "source_questions.json").read_text())[0]
    question.update({
        "answer": 0, "raw_text": "[H3C_IMAGE:source-figure]",
        "source": {"images": ["source-figure"], "pages": [1]},
        "source_regions": [{"page": 1, "bbox": [10, 10, 100, 20]},
                           {"page": 1, "bbox": [10, 80, 100, 90]}],
        "review_reasons": ["figure_required"],
    })
    Image.new("RGB", (90, 60), "white").save(folder / "diagram.png")
    (folder / "assets.json").write_text(json.dumps([{
        "id": "source-figure", "page": 1, "bbox": [10, 20, 100, 80], "path": "diagram.png",
    }]))
    (folder / "source_questions.json").write_text(json.dumps([question]))
    (folder / "bank.json").write_text(json.dumps({"questions": [question]}))
    approved = tmp_path / "approved.json"
    approved.write_text(json.dumps([{
        "source_question_id": question["id"], "reviewed": True,
        "analysis": "IPv6 默认路由为 ::/0，前缀长度为零；/64 仅是常用子网的前缀长度，不能作为默认路由。",
    }]))
    result = reconcile(structured, output, approved)
    assert len(result["banks"]["h3c_gb0330_202410"]["questions"]) == 1
    assert result["image_assets"][0]["ownership_status"] == "coordinate_and_block_match"
