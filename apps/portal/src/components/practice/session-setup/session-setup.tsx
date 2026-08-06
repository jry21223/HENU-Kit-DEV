"use client";

import { useEffect, useState } from "react";
import { fetchQuizCraftCatalog } from "@/lib/api/client";
import type { QuizCraftCatalogChapter } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import TransitionLink from "@/components/practice/transition/transition-link";
import {
  isValidQuestionCount,
  MAX_QUESTION_COUNT,
  MIN_QUESTION_COUNT,
} from "@/lib/practice/question-count";

export type SessionMode = "random" | "difficult" | "chapter";

/** Every supported session mode, in display order. */
export const SESSION_MODES: SessionMode[] = ["random", "difficult", "chapter"];

/** Narrowing guard for an untrusted URL-mode value. */
export function isSessionMode(value: string): value is SessionMode {
  return SESSION_MODES.includes(value as SessionMode);
}

/** One composed session selection, ready for createPracticeSession. */
export interface SessionSelection {
  mode: SessionMode;
  chapterID?: string;
  questionCount: number;
}

const MODES: Array<{ value: SessionMode; label: string; description: string }> = [
  { value: "random", label: "随机", description: "从整个题库随机抽题，适合日常巩固。" },
  { value: "difficult", label: "难题", description: "优先挑选历史答错率高的题，挑战一下。" },
  { value: "chapter", label: "章节", description: "只练所选章节的题，按章节推进。" },
];

type ChaptersState =
  | { kind: "loading" }
  | { kind: "ready"; bankName?: string; chapters: QuizCraftCatalogChapter[] }
  | { kind: "error" };

export default function SessionSetup({
  bankID,
  bankVersionID,
  onStart,
}: {
  bankID: string;
  bankVersionID: string;
  onStart: (selection: SessionSelection) => void;
}) {
  const [mode, setMode] = useState<SessionMode>("random");
  const [chapterID, setChapterID] = useState("");
  const [questionCount, setQuestionCount] = useState("20");
  const [starting, setStarting] = useState(false);
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
        // A Gateway that predates #268 omits chapters; treat that the same as
        // an empty list rather than inventing chapter facts locally.
        setChaptersState({
          kind: "ready",
          bankName: bank?.name,
          chapters: bank?.chapters ?? [],
        });
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
  const countTouched = questionCount.trim() !== "";

  const chaptersReady = chaptersState.kind === "ready" && chaptersState.chapters.length > 0;
  const canStart = countValid && (mode !== "chapter" || (chapterID !== "" && chaptersReady));

  const start = () => {
    if (!canStart || starting) return;
    setStarting(true);
    onStart({
      mode,
      ...(mode === "chapter" && chapterID ? { chapterID } : {}),
      questionCount: countValue,
    });
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
        onChange={(event) => setQuestionCount(event.target.value)}
        className="mt-2 w-32 border border-ink/30 bg-transparent px-3 py-2 font-mono text-sm outline-none transition-colors focus:border-ink"
      />
      {countTouched && !countValid && (
        <p role="alert" className="mt-2 text-sm text-accent">
          题数需为 {MIN_QUESTION_COUNT} 到 {MAX_QUESTION_COUNT} 之间的整数。
        </p>
      )}
    </div>
  );

  return (
    <main className="mx-auto max-w-3xl px-5 py-16 md:px-8">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">SETUP</span>
        <span className="mx-2">/</span>
        组卷设置
      </p>
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
              onClick={() => setMode(item.value)}
              aria-pressed={mode === item.value}
              className={cn(
                "border p-4 text-left transition-colors",
                mode === item.value
                  ? "border-ink bg-ink text-paper"
                  : "border-line hover:border-ink/40"
              )}
            >
              <span className="font-mono text-xs text-accent">
                M-{String(index + 1).padStart(2, "0")}
              </span>
              <p className="mt-2 font-display text-lg font-bold">{item.label}</p>
              <p className={cn("mt-1 text-xs leading-5", mode === item.value ? "text-paper/70" : "text-ink/60")}>
                {item.description}
              </p>
            </button>
          ))}
        </div>

        <div className="mt-6 border-t border-line pt-6">
          {mode === "chapter" ? (
            <div>
              <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">章节 / CHAPTER</p>
              {chaptersState.kind === "loading" && (
                <p className="mt-2 text-sm text-ink/60">正在加载章节列表…</p>
              )}
              {chaptersState.kind === "error" && (
                <div className="mt-2 border border-accent px-4 py-3">
                  <p className="text-sm text-accent">章节列表暂时加载不出来，无法发起章节练习。</p>
                  <button
                    type="button"
                    onClick={() => {
                      setChaptersState({ kind: "loading" });
                      setChaptersRetry((value) => value + 1);
                    }}
                    className="mt-2 border border-accent px-3 py-1 font-mono text-xs text-accent transition-colors hover:bg-accent hover:text-paper"
                  >
                    重试
                  </button>
                </div>
              )}
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
                      onChange={(event) => setChapterID(event.target.value)}
                      className="mt-2 block w-full max-w-md border border-ink/30 bg-paper px-3 py-2 font-mono text-sm outline-none transition-colors focus:border-ink md:w-auto"
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
                  ? "服务端按题库历史作答统计挑选答错率高的题，未作答的题会优先出现。"
                  : "服务端从整个题库随机挑选，不设默认上限。"}
              </p>
            </div>
          )}
        </div>

        <div className="mt-8 flex flex-wrap gap-4">
          <button
            type="button"
            onClick={start}
            disabled={!canStart || starting}
            className={cn(
              "border px-7 py-3.5 font-mono text-sm tracking-widest transition-colors",
              canStart && !starting
                ? "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                : "cursor-not-allowed border-line text-ink/30"
            )}
          >
            {starting ? "正在开始…" : "开始练习 →"}
          </button>
          <TransitionLink
            href="/practice"
            className="border border-ink/30 px-7 py-3.5 font-mono text-sm tracking-widest text-ink transition-colors hover:border-accent hover:text-accent"
          >
            返回题库目录 →
          </TransitionLink>
        </div>
      </div>
    </main>
  );
}
