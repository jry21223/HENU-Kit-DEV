import { describe, expect, it } from "vitest";

import {
  DEFAULT_RANKING_NICKNAME,
  validateRankingNickname,
} from "./nickname-validation";

describe("validateRankingNickname", () => {
  it("resolves empty and whitespace-only input to the neutral label", () => {
    expect(validateRankingNickname("")).toEqual({
      ok: true,
      normalized: DEFAULT_RANKING_NICKNAME,
    });
    expect(validateRankingNickname("   ")).toEqual({
      ok: true,
      normalized: DEFAULT_RANKING_NICKNAME,
    });
    expect(validateRankingNickname("　")).toEqual({
      ok: true,
      normalized: DEFAULT_RANKING_NICKNAME,
    });
  });

  it("accepts plain Chinese, ASCII and mixed nicknames", () => {
    expect(validateRankingNickname("认真刷题").ok).toBe(true);
    expect(validateRankingNickname("Code Master 2026").ok).toBe(true);
    expect(validateRankingNickname("Code_Master.2026").ok).toBe(true);
    expect(validateRankingNickname("小明-a").ok).toBe(true);
  });

  it("normalizes NFKC input and keeps the normalized value", () => {
    const result = validateRankingNickname("　ＡＢＣ１２３　");
    expect(result.ok).toBe(true);
    expect(result.normalized).toBe("ABC123");
  });

  it("enforces the 1..32 rune length", () => {
    expect(validateRankingNickname("好".repeat(32)).ok).toBe(true);
    expect(validateRankingNickname("好".repeat(33)).ok).toBe(false);
  });

  it("rejects email and identifier-shaped values", () => {
    expect(validateRankingNickname("learner@example.test").ok).toBe(false);
    // Fullwidth @ normalizes to @ and must still be rejected.
    expect(validateRankingNickname("learner＠example．test").ok).toBe(false);
    for (const nickname of [
      "123e4567e89b12d3a456426614174000",
      "123e4567 e89b 12d3 a456 426614174000",
      "123e4567_e89b_12d3_a456_426614174000",
      "123e4567.e89b.12d3.a456.426614174000",
      "１２３ｅ４５６７ｅ８９ｂ１２ｄ３ａ４５６４２６６１４１７４０００",
    ]) {
      expect(validateRankingNickname(nickname).ok, nickname).toBe(false);
    }
  });

  it("rejects forbidden reserved substrings case-insensitively", () => {
    for (const nickname of [
      "admin",
      "Administrator",
      "henukit",
      "QUIZCRAFT",
      "官方",
      "管理员",
      "管理員",
      "官网",
      "官網",
      "我是管理员",
      "henukit 学习",
    ]) {
      expect(validateRankingNickname(nickname).ok, nickname).toBe(false);
    }
  });

  it("rejects characters outside the allowed set", () => {
    expect(validateRankingNickname("😀").ok).toBe(false);
    expect(validateRankingNickname("café").ok).toBe(false);
    expect(validateRankingNickname("emoji🎓").ok).toBe(false);
    expect(validateRankingNickname("tab\there").ok).toBe(false);
  });
});
