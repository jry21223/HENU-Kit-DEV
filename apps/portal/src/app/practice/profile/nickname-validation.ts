/**
 * Client-side mirror of QuizCraft Core's ranking nickname rules
 * (practice_http.go normalizeRankingNickname). The server remains the final
 * authority; this only gives the settings form immediate feedback so a write
 * is not attempted with a nickname Core would reject.
 */

export interface RankingNicknameValidation {
  ok: boolean;
  /** NFKC-normalized, trimmed value; empty input resolves to the neutral label. */
  normalized: string;
  reason?: string;
}

/** Neutral public label Core assigns when the nickname is left empty. */
export const DEFAULT_RANKING_NICKNAME = "匿名学习者";

/** Nickname length limit after NFKC normalization (1..32 runes; empty allowed). */
export const MAX_RANKING_NICKNAME_RUNES = 32;

const FORBIDDEN_SUBSTRINGS = [
  "admin",
  "administrator",
  "henukit",
  "quizcraft",
  "官方",
  "管理员",
  "管理員",
  "官网",
  "官網",
];

const SEPARATORS = /[ _\-.]/;
const IDENTIFIER_SHAPE = /^[0-9a-fA-F]{32}$/;

/** Han ranges matching Go's unicode.Han script set (main + extensions). */
function isHanRune(code: number): boolean {
  return (
    (code >= 0x3400 && code <= 0x4dbf) ||
    (code >= 0x4e00 && code <= 0x9fff) ||
    (code >= 0xf900 && code <= 0xfaff) ||
    (code >= 0x20000 && code <= 0x2ebef) ||
    (code >= 0x2f800 && code <= 0x2fa1f)
  );
}

/** Allowed set: Han, ASCII Latin, digits and the separators space/_/-/. */
function isAllowedNicknameChar(rune: string): boolean {
  const code = rune.codePointAt(0) ?? -1;
  if (isHanRune(code)) return true;
  if (code >= 0x30 && code <= 0x39) return true; // 0-9
  if (code >= 0x41 && code <= 0x5a) return true; // A-Z
  if (code >= 0x61 && code <= 0x7a) return true; // a-z
  if (code === 0x20 || code === 0x5f || code === 0x2d || code === 0x2e) {
    return true; // space _ - .
  }
  return false;
}

export function validateRankingNickname(raw: string): RankingNicknameValidation {
  let normalized: string;
  try {
    normalized = raw.normalize("NFKC").trim();
  } catch {
    // Extremely old runtimes without String.prototype.normalize.
    normalized = raw.trim();
  }

  if (normalized === "") {
    return { ok: true, normalized: DEFAULT_RANKING_NICKNAME };
  }

  // Email-shaped values and 32-char hex runs are rejected to keep account
  // identifiers off the public ranking.
  if (normalized.includes("@")) {
    return {
      ok: false,
      normalized,
      reason: "昵称不能包含邮箱或账户标识。",
    };
  }
  const compact = normalized.replace(SEPARATORS, "");
  if (compact.length === 32 && IDENTIFIER_SHAPE.test(compact)) {
    return {
      ok: false,
      normalized,
      reason: "昵称不能包含账户标识。",
    };
  }

  const runes = Array.from(normalized);
  if (runes.length > MAX_RANKING_NICKNAME_RUNES) {
    return {
      ok: false,
      normalized,
      reason: `昵称最多 ${MAX_RANKING_NICKNAME_RUNES} 个字符。`,
    };
  }
  for (const rune of runes) {
    if (!isAllowedNicknameChar(rune)) {
      return {
        ok: false,
        normalized,
        reason: "昵称只能包含中文、字母、数字、空格与 _-. 符号。",
      };
    }
  }

  const skeleton = runes
    .filter((rune) => !SEPARATORS.test(rune))
    .join("")
    .toLowerCase();
  for (const forbidden of FORBIDDEN_SUBSTRINGS) {
    if (skeleton.includes(forbidden)) {
      return {
        ok: false,
        normalized,
        reason: "昵称包含保留词，请换一个。",
      };
    }
  }

  return { ok: true, normalized };
}
