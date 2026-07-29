#!/usr/bin/env python3
"""Regression contracts for QuizCraft's irreversible cutover preflight."""

from __future__ import annotations

import pathlib


SCRIPT = pathlib.Path(__file__).with_name("verify-cutover.sh")


def test_root_owned_go_environment_is_root_only() -> None:
    source = SCRIPT.read_text(encoding="utf-8")
    assert "test $(( 8#$go_environment_permissions & 8#077 )) -eq 0" in source

    def accepted(mode: int) -> bool:
        return mode & 0o077 == 0

    assert accepted(0o600)
    assert not accepted(0o640)
    assert not accepted(0o644)


def test_protected_ranking_probe_is_a_prewrite_gate() -> None:
    source = SCRIPT.read_text(encoding="utf-8")
    ranking_probe = 'write_portal_read_config "/api/v1/rankings/overall?period=weekly"'
    first_write_enabled_happy_path = 'if [[ "$EXPECTED_WRITES_ENABLED" == "true" ]]; then\n  python3 - "$cutover_tmp/banks.json"'
    assert source.index(ranking_probe) < source.index(first_write_enabled_happy_path)


if __name__ == "__main__":
    test_root_owned_go_environment_is_root_only()
    test_protected_ranking_probe_is_a_prewrite_gate()
    print("Cutover preflight contracts passed")
