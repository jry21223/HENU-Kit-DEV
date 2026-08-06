"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { RankingAvatar } from "@/components/practice/ranking-avatar";
import RankingLoginPrompt from "@/components/practice/ranking-login-prompt";
import {
  fetchSession,
  formatPortalError,
  updateQuizCraftRankingProfile,
} from "@/lib/api/client";
import type {
  PortalSession,
  QuizCraftSystemAvatar,
} from "@/lib/api/types";
import { quizCraftV2ReadsEnabled } from "@/lib/api/env";
import { publicDisplayName } from "@/lib/auth/display-name";
import { cn } from "@/lib/cn";
import {
  DEFAULT_RANKING_NICKNAME,
  MAX_RANKING_NICKNAME_RUNES,
  validateRankingNickname,
} from "./nickname-validation";

type SessionState =
  | { kind: "loading" }
  | { kind: "authenticated"; session: PortalSession }
  | { kind: "anonymous" }
  | { kind: "error"; message: string };

const AVATAR_OPTIONS: Array<{ value: QuizCraftSystemAvatar; label: string }> = [
  { value: "scholar-blue", label: "学者" },
  { value: "coder-green", label: "编码者" },
  { value: "reader-amber", label: "读者" },
  { value: "owl-purple", label: "夜猫子" },
];

export default function RankingProfilePage() {
  const enabled = quizCraftV2ReadsEnabled();
  const [sessionState, setSessionState] = useState<SessionState>({ kind: "loading" });
  const [nickname, setNickname] = useState("");
  const [avatar, setAvatar] = useState<QuizCraftSystemAvatar>("scholar-blue");
  const [optedOut, setOptedOut] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [saved, setSaved] = useState(false);
  const idempotencyKey = useRef<string | null>(null);
  const requestVersion = useRef(0);

  useEffect(() => {
    if (!enabled) return;
    const version = ++requestVersion.current;
    void fetchSession().then(
      (session) => {
        if (version !== requestVersion.current) return;
        setSessionState(
          session ? { kind: "authenticated", session } : { kind: "anonymous" }
        );
      },
      (error: unknown) => {
        if (version !== requestVersion.current) return;
        setSessionState({ kind: "error", message: formatPortalError(error) });
      }
    );
    return () => {
      requestVersion.current += 1;
    };
  }, [enabled]);

  if (!enabled) {
    return (
      <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
        <div className="max-w-4xl">
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">RANK</span>
            <span className="mx-2">/</span>
            PROFILE
          </p>
          <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">排行设置</h1>
          <div className="mt-8">
            <EmptyBlock label="QuizCraft V2 排行设置将在确认切换后启用" />
          </div>
        </div>
      </main>
    );
  }

  const validation = validateRankingNickname(nickname);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!validation.ok || saving) return;
    if (!idempotencyKey.current) {
      const random =
        globalThis.crypto?.randomUUID?.() ??
        `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
      idempotencyKey.current = `ranking-profile:${random}`;
    }
    setSaving(true);
    setSaveError("");
    setSaved(false);
    try {
      await updateQuizCraftRankingProfile(
        {
          // Empty input keeps the neutral label; Core applies its own default.
          nickname: validation.normalized,
          system_avatar: avatar,
          visible: !optedOut,
        },
        idempotencyKey.current
      );
      setSaved(true);
      // A new save is a new logical write: drop the consumed key so the next
      // submit mints a fresh one instead of replaying a 409 on Core.
      idempotencyKey.current = null;
    } catch (error) {
      setSaveError(formatPortalError(error));
    } finally {
      setSaving(false);
    }
  };

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div className="max-w-4xl">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">RANK</span>
          <span className="mx-2">/</span>
          PROFILE
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">排行设置</h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-ink/65">
          设置排行榜上的公开身份。公开榜只展示受控昵称与系统头像，绝不暴露邮箱或账户标识。
        </p>

        {sessionState.kind === "loading" && (
          <div className="mt-8">
            <LoadingBlock label="正在确认登录状态" />
          </div>
        )}

        {sessionState.kind === "error" && (
          <div className="mt-8">
            <ErrorBanner message={sessionState.message} />
          </div>
        )}

        {sessionState.kind === "anonymous" && <RankingLoginPrompt next="/practice/profile" />}

        {sessionState.kind === "authenticated" && (
          <form onSubmit={submit} className="mt-8 max-w-2xl" data-testid="ranking-profile-form">
            <div className="border border-ink/25 p-6">
              <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
                PUBLIC IDENTITY /{" "}
                <span className="text-accent">{publicDisplayName(sessionState.session.display_name)}</span>
              </p>

              <div className="mt-6 flex items-center gap-4">
                <RankingAvatar avatar={avatar} size={44} />
                <div className="min-w-0">
                  <p className="truncate font-medium">
                    {validation.ok ? validation.normalized : nickname.trim() || DEFAULT_RANKING_NICKNAME}
                  </p>
                  <p className="font-mono text-xs text-ink/50">
                    {optedOut ? "未公开（已退出排行）" : "公开"}
                  </p>
                </div>
              </div>

              <div className="mt-8">
                <label
                  htmlFor="ranking-nickname"
                  className="font-mono text-xs tracking-widest text-ink/70"
                >
                  昵称
                </label>
                <input
                  id="ranking-nickname"
                  type="text"
                  value={nickname}
                  onChange={(event) => {
                    setNickname(event.target.value);
                    setSaved(false);
                  }}
                  maxLength={MAX_RANKING_NICKNAME_RUNES}
                  placeholder={DEFAULT_RANKING_NICKNAME}
                  autoComplete="off"
                  spellCheck={false}
                  className="mt-2 w-full border border-ink/25 bg-paper px-3 py-2.5 font-mono text-sm text-ink placeholder:text-ink/35 focus:border-ink focus:outline-none"
                />
                {nickname !== "" && !validation.ok && validation.reason && (
                  <p role="alert" className="mt-2 font-mono text-xs text-accent">
                    {validation.reason}
                  </p>
                )}
                <p className="mt-2 font-mono text-xs leading-5 text-ink/50">
                  留空使用默认身份「{DEFAULT_RANKING_NICKNAME}」；最多{" "}
                  {MAX_RANKING_NICKNAME_RUNES} 个字符，仅限中文、字母、数字、空格与 _-. 符号。
                </p>
              </div>

              <fieldset className="mt-8">
                <legend className="font-mono text-xs tracking-widest text-ink/70">系统头像</legend>
                <div className="mt-3 flex flex-wrap gap-3">
                  {AVATAR_OPTIONS.map((option) => (
                    <button
                      key={option.value}
                      type="button"
                      aria-pressed={avatar === option.value}
                      onClick={() => {
                        setAvatar(option.value);
                        setSaved(false);
                      }}
                      className={cn(
                        "flex flex-col items-center gap-2 border p-3 transition-colors",
                        avatar === option.value
                          ? "border-ink bg-ink/5"
                          : "border-ink/25 hover:border-ink"
                      )}
                    >
                      <RankingAvatar avatar={option.value} size={36} />
                      <span className="font-mono text-xs tracking-widest text-ink/70">
                        {option.label}
                      </span>
                    </button>
                  ))}
                </div>
              </fieldset>

              <label className="mt-8 flex items-start gap-3 border border-ink/25 p-4">
                <input
                  type="checkbox"
                  checked={optedOut}
                  onChange={(event) => {
                    setOptedOut(event.target.checked);
                    setSaved(false);
                  }}
                  className="mt-0.5 h-4 w-4 accent-[#FF4D00]"
                />
                <span>
                  <span className="block font-medium">退出排行榜</span>
                  <span className="mt-1 block text-xs leading-5 text-ink/60">
                    开启后你的作答统计不再出现在任何公开排行中；其他学习功能不受影响。
                  </span>
                </span>
              </label>

              {saveError && (
                <p role="alert" className="mt-6 border border-accent/60 bg-accent/5 px-4 py-3 text-sm leading-6 text-ink">
                  {saveError}
                </p>
              )}
              {saved && (
                <p role="status" className="mt-6 border border-ink/25 bg-ink/5 px-4 py-3 text-sm leading-6 text-ink">
                  排行身份已保存。公开榜将在下次读取时使用这份身份。
                </p>
              )}

              <div className="mt-6 flex flex-wrap items-center gap-4">
                <button
                  type="submit"
                  disabled={!validation.ok || saving}
                  className="border border-ink bg-ink px-6 py-2.5 font-mono text-xs tracking-widest text-paper transition-opacity disabled:cursor-not-allowed disabled:opacity-40"
                >
                  {saving ? "保存中…" : "保存排行设置"}
                </button>
                <Link
                  href="/practice/leaderboard"
                  className="font-mono text-xs tracking-widest text-ink/60 transition-colors hover:text-accent"
                >
                  ← 返回排行榜
                </Link>
              </div>

              <p className="mt-6 border-t border-line pt-4 font-mono text-[10px] leading-5 text-ink/45">
                NOTE / Core 目前未提供读取现有设置的接口，此表单以默认值开始，保存将覆盖已有设置；退出排行只影响公开展示，不会删除作答记录。
              </p>
            </div>
          </form>
        )}
      </div>
    </main>
  );
}
