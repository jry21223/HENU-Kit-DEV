"use client";

import { useEffect, useState } from "react";
import TransitionLink from "@/components/practice/transition/transition-link";
import { fetchQuizCraftCatalog } from "@/lib/api/client";
import type { QuizCraftCatalogChapter } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import {
  isValidQuestionCount,
  MAX_QUESTION_COUNT,
  MIN_QUESTION_COUNT,
} from "@/lib/practice/question-count";

export type SessionMode = "random" | "difficult" | "chapter";

const MODES: Array<{ value: SessionMode; label: string; description: string }> = [
  { value: "random", label: "随机", description: "从整个题库随机抽题，适合日常巩固。" },
  { value: "difficult", label: "难题", description: "优先挑选历史答错率高的题，挑战一下。" },
  { value: "chapter", label: "章节", description: "只练所选章节的题，按章节推进。" },
];

/** One user-confirmed selection, ready to compose into a Core command. */
export interface SessionSelection {
  mode: SessionMode;
  chapterID?: string;
  questionCount: number;
}

type ChaptersState =
  | { kind: "loading" }
  | { kind: "ready"; bankName?: string; chapters: QuizCraftCatalogChapter[] }
  | { kind: "missing" }
  | { kind: "error" };

export default function SessionSetup({
  bankID,
  bankVersionID,
  onStart,
}: {
  bankID: string;
  bankVersionID: string;
  onStart: (selection: SessionSelection) => Promise<string | null>;
}) {
  const [mode, setMode] = useState<SessionMode>("random");
  const [chapterID, setChapterID] = useState("");
  const [questionCount, setQuestionCount] = useState("20");
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState("");
  const [chaptersRetry, setChaptersRetry] = useState(0);
  const [chaptersState, setChaptersState] = useState<ChaptersState>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const response = await fetchQuizCraftCatalog();
        if (cancelled) return;
        const bank = response.banks.find(
          (item) => item.bank_id === bankID && item.bank_version_id === bankVersionID
        );
        if (!bank || !bank.available) {
          // Catalog membership is the availability fact for this immutable
          // version. Never let stale URL state become a valid setup command.
          setChaptersState({ kind: "missing" });
          return;
        }
        if (!Array.isArray(bank.chapters)) {
          setChaptersState({ kind: "error" });
          return;
        }
        setChaptersState({ kind: "ready", bankName: bank.name, chapters: bank.chapters });
      } catch {
        if (!cancelled) setChaptersState({ kind: "error" });
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [bankID, bankVersionID, chaptersRetry]);

  const countValue = questionCount.trim() === "" ? Number.NaN : Number(questionCount);
  const countValid = isValidQuestionCount(countValue);
  const chaptersReady = chaptersState.kind === "ready" && chaptersState.chapters.length > 0;
  const canStart = chaptersState.kind === "ready" && countValid && (mode !== "chapter" || (chapterID !== "" && chaptersReady));

  const retryCatalog = () => {
    setChaptersState({ kind: "loading" });
    setChaptersRetry((value) => value + 1);
  };

  if (chaptersState.kind === "missing" || chaptersState.kind === "error") {
    const missing = chaptersState.kind === "missing";
    return (
      <main className="mx-auto max-w-3xl px-5 py-16 md:px-8">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">SETUP</span>
          <span className="mx-2">/</span>
          组卷设置
        </p>
        <div data-enter className="mt-6 border border-accent p-6 md:p-10">
          <h1 className="font-display text-3xl font-bold">
            {missing ? "当前题库版本不可用" : "暂时无法确认题库版本"}
          </h1>
          <p className="mt-3 text-sm leading-7 text-ink/70">
            {missing
              ? "题库版本可能已更新或下架，请重新检查，或返回题库目录选择当前版本。"
              : "暂时无法确认题库版本，请重新检查；若仍无法加载，请返回题库目录重新选择。"}
          </p>
          <div className="mt-6 flex flex-wrap gap-4">
            <button
              type="button"
              onClick={retryCatalog}
              className="min-h-11 border border-ink bg-ink px-5 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
            >
              重新检查题库
            </button>
            <TransitionLink
              href="/practice"
              className="flex min-h-11 items-center border border-ink/30 px-5 py-3 font-mono text-sm tracking-widest transition-colors hover:border-accent hover:text-accent"
            >
              返回题库目录 →
            </TransitionLink>
          </div>
        </div>
      </main>
    );
  }

  const start = async () => {
    if (!canStart || starting) return;
    setStarting(true);
    setStartError("");
    try {
      const error = await onStart({
        mode,
        ...(mode === "chapter" && chapterID ? { chapterID } : {}),
        questionCount: countValue,
      });
      if (error) setStartError(error);
    } catch {
      setStartError("暂时无法创建练习会话，请重试。");
    } finally {
      setStarting(false);
    }
  };

  const countInput = (
    <div>
      <label htmlFor="session-question-count" className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
        题数 / COUNT
      </label>
      <input
        id="session-question-count"
        type="number"
        min={MIN_QUESTION_COUNT}
        max={MAX_QUESTION_COUNT}
        inputMode="numeric"
        value={questionCount}
        onChange={(event) => {
          setQuestionCount(event.target.value);
          setStartError("");
        }}
        className="mt-2 min-h-11 w-32 border border-ink/30 bg-transparent px-3 py-2 font-mono text-sm outline-none transition-colors focus:border-ink"
      />
      {!countValid && (
        <p role="alert" className="mt-2 text-sm text-accent">
          题数需为 {MIN_QUESTION_COUNT} 到 {MAX_QUESTION_COUNT} 之间的整数。
        </p>
      )}
    </div>
  );

  return (
    <main
      data-testid="practice-session-setup"
      aria-busy={chaptersState.kind === "loading"}
      className="mx-auto max-w-3xl px-5 py-16 md:px-8"
    >
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">SETUP</span>
        <span className="mx-2">/</span>
        组卷设置
      </p>
      <h1 data-enter className="mt-3 font-display text-3xl font-bold md:text-4xl">组卷设置</h1>
      {chaptersState.kind === "loading" && (
        <p role="status" aria-live="polite" className="mt-3 text-sm text-ink/60">
          正在确认题库版本…
        </p>
      )}
      {chaptersState.kind === "ready" && chaptersState.bankName && (
        <p data-enter className="mt-2 font-mono text-[10px] tracking-widest text-ink/40">
          {chaptersState.bankName}
        </p>
      )}

      <div data-enter className="mt-6 border border-ink p-6 md:p-10">
        <div className="grid gap-4 md:grid-cols-3">
          {MODES.map((item, index) => (
            <button
              key={item.value}
              type="button"
              onClick={() => {
                setMode(item.value);
                setStartError("");
              }}
              aria-pressed={mode === item.value}
              className={cn(
                "min-h-11 border p-4 text-left transition-colors",
                mode === item.value
                  ? "border-ink bg-ink text-paper"
                  : "border-line hover:border-ink/40"
              )}
            >
              <span className="font-mono text-xs text-accent">M-{String(index + 1).padStart(2, "0")}</span>
              <span className="mt-2 block font-display text-lg font-bold">{item.label}</span>
              <span className={cn("mt-1 block text-xs leading-5", mode === item.value ? "text-paper/70" : "text-ink/60")}>{item.description}</span>
            </button>
          ))}
        </div>

        <div className="mt-6 border-t border-line pt-6">
          {mode === "chapter" ? (
            <div>
              <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">章节 / CHAPTER</p>
              {chaptersState.kind === "loading" && <p className="mt-2 text-sm text-ink/60">正在加载章节列表…</p>}
              {chaptersState.kind === "ready" && chaptersState.chapters.length === 0 && (
                <p className="mt-2 text-sm text-ink/60">该题库暂未划分章节，请选择其他模式。</p>
              )}
              {chaptersState.kind === "ready" && chaptersState.chapters.length > 0 && (
                <div className="flex flex-wrap items-end gap-6">
                  <div>
                    <label htmlFor="session-chapter" className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
                      选择章节
                    </label>
                    <select
                      id="session-chapter"
                      value={chapterID}
                      onChange={(event) => {
                        setChapterID(event.target.value);
                        setStartError("");
                      }}
                      className="mt-2 block min-h-11 w-full max-w-md border border-ink/30 bg-paper px-3 py-2 font-mono text-sm outline-none transition-colors focus:border-ink md:w-auto"
                    >
                      <option value="">请选择章节</option>
                      {chaptersState.chapters.map((chapter) => (
                        <option key={chapter.id} value={chapter.id}>
                          {chapter.name}
                        </option>
                      ))}
                    </select>
                  </div>
                  {countInput}
                </div>
              )}
            </div>
          ) : (
            <div className="flex flex-wrap items-end gap-6">
              {countInput}
              <p className="text-xs leading-5 text-ink/60">
                {mode === "difficult"
                  ? "根据题库历史作答情况，优先挑选答错率高的题。"
                  : "从整个题库随机抽取，题数由本次确认决定。"}
              </p>
            </div>
          )}
        </div>

        {startError && (
          <p role="alert" className="mt-6 border border-accent px-4 py-3 text-sm text-accent">
            {startError}
          </p>
        )}

        <div className="mt-8 flex flex-wrap gap-4">
          <button
            data-testid="practice-session-start"
            type="button"
            onClick={() => void start()}
            disabled={!canStart || starting}
            className={cn(
              "min-h-11 border px-7 py-3.5 font-mono text-sm tracking-widest transition-colors",
              canStart && !starting
                ? "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                : "cursor-not-allowed border-line text-ink/30"
            )}
          >
            {starting ? "正在开始…" : "开始练习 →"}
          </button>
          <TransitionLink
            href="/practice"
            className="flex min-h-11 items-center border border-ink/30 px-7 py-3.5 font-mono text-sm tracking-widest text-ink transition-colors hover:border-accent hover:text-accent"
          >
            返回题库目录 →
          </TransitionLink>
        </div>
      </div>
    </main>
  );
}
