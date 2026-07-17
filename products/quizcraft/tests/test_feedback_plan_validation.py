import importlib.util
import sys
import types
from pathlib import Path

import pytest


def _ensure_fake_psycopg():
    if "psycopg" in sys.modules:
        return
    psycopg = types.ModuleType("psycopg")
    types_pkg = types.ModuleType("psycopg.types")
    json_pkg = types.ModuleType("psycopg.types.json")

    class Jsonb:
        pass

    json_pkg.Jsonb = Jsonb
    types_pkg.json = json_pkg

    sys.modules["psycopg"] = psycopg
    sys.modules["psycopg.types"] = types_pkg
    sys.modules["psycopg.types.json"] = json_pkg


def _load_module(name: str, rel_path: str):
    path = Path(__file__).resolve().parents[1] / rel_path
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


_ensure_fake_psycopg()
apply_feedback_plan = _load_module("apply_feedback_plan", "scripts/apply_feedback_plan.py")
deepseek_feedback_plan = _load_module("deepseek_feedback_plan", "scripts/deepseek_feedback_plan.py")

_validate_plan = apply_feedback_plan._validate_plan
ApplyPlanError = apply_feedback_plan.PlanError
validate_plan = deepseek_feedback_plan.validate_plan
DeepSeekPlanError = deepseek_feedback_plan.PlanError


def _sample_apply_plan():
    return {
        "_source": {
            "bank_key": "bank",
            "question_id": "q1",
            "feedback_id": 1,
        },
        "verdict": "no_change",
        "confidence": 0.75,
        "changed_fields": [],
        "db_patch": {
            "answer": None,
            "type": None,
            "payload": None,
        },
        "reasoning_summary": "No change.",
    }


def _sample_deepseek_plan():
    return {
        "verdict": "no_change",
        "confidence": 0.75,
        "reasoning_summary": "No change.",
        "changed_fields": [],
        "db_patch": {
            "answer": None,
            "type": None,
            "payload": None,
        },
    }


@pytest.mark.parametrize("confidence", [True, False])
def test_apply_feedback_plan_rejects_bool_confidence(confidence):
    plan = _sample_apply_plan()
    plan["confidence"] = confidence
    with pytest.raises(ApplyPlanError, match="confidence"):
        _validate_plan(plan)


@pytest.mark.parametrize("confidence", [True, False])
def test_deepseek_feedback_plan_rejects_bool_confidence(confidence):
    plan = _sample_deepseek_plan()
    plan["confidence"] = confidence
    with pytest.raises(DeepSeekPlanError, match="confidence"):
        validate_plan(plan)


@pytest.mark.parametrize("confidence", [0, 1, 0.75])
def test_apply_feedback_plan_accepts_numeric_confidence(confidence):
    plan = _sample_apply_plan()
    plan["confidence"] = confidence
    _validate_plan(plan)


@pytest.mark.parametrize("confidence", [0, 1, 0.75])
def test_deepseek_feedback_plan_accepts_numeric_confidence(confidence):
    plan = _sample_deepseek_plan()
    plan["confidence"] = confidence
    validate_plan(plan)
