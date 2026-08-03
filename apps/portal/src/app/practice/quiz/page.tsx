"use client";

import { useEffect, useRef, useState } from "react";
import {
  createPracticeSession,
  formatPortalError,
  submitPracticeAnswer,
} from "@/lib/api/client";
import type {
  PortalPracticeAnswerResponse,
  PortalPracticeQuestion,
  PortalPracticeSessionInput,
  PortalPracticeSessionResponse,
} from "@/lib/api/types";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import TransitionLink from "@/components/practice/transition/transition-link";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const OPTION_LABEL = ["A", "B", "C", "D", "E", "F", "G", "H"];
// Match the canonical UUID text form accepted by Gateway and QuizCraft Core;
// do not unnecessarily reject a newer UUID version issued by a real bank.
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

type LoadState = "loading" | "ready" | "empty" | "missing-selection" | "error" | "finished";
type AnswerResult = PortalPracticeAnswerResponse["data"];

function fmtTime(sec: number) {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function questionKey(question: PortalPracticeQuestion) {
  return `${question.question_id}:${question.question_version_id}`;
}

function isUUID(value: string | null): value is string {
  return value !== null && UUID.test(value);
}

function practiceInputFromLocation(): { input?: PortalPracticeSessionInput; error?: string; scope?: string } {
  const params = new URLSearchParams(window.location.search);
  const bankID = params.get("bank_id")?.trim() ?? "";
  const bankVersionID = params.get("bank_version_id")?.trim() ?? "";
  if (!bankID && !bankVersionID) {
    return { error: "请先从题库目录选择一组练习后开始。" };
  }
  if (!isUUID(bankID) || !isUUID(bankVersionID)) {
    return { error: "题库选择无效，请返回题库目录重新选择。" };
  }
  const mode = params.get("mode")?.trim() || "random";
  if (mode !== "random" && mode !== "difficult" && mode !== "chapter") {
    return { error: "练习模式无效，请从题库目录重新选择。" };
  }
  const chapterID = params.get("chapter_id")?.trim() || undefined;
  if (mode === "chapter" && !chapterID) {
    return { error: "章节练习需要指定章节。" };
  }
  const countParam = params.get("question_count")?.trim();
  const questionCount = countParam ? Number(countParam) : undefined;
  if (questionCount !== undefined && (!Number.isInteger(questionCount) || questionCount < 1 || questionCount > 500)) {
    return { error: "题目数量必须在 1 到 500 之间。" };
  }
  const input: PortalPracticeSessionInput = {
    bank_id: bankID,
    bank_version_id: bankVersionID,
    mode,
    ...(chapterID ? { chapter_id: chapterID } : {}),
    ...(questionCount ? { question_count: questionCount } : {}),
  };
  return { input, scope: JSON.stringify(input) };
}

function createBrowserKey(prefix: string) {
  const random = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${prefix}:${random}`;
}

type IdempotencyMemory = { current: Record<string, string> };

function idempotencyKeyFor(scope: string, prefix: string, memory: IdempotencyMemory) {
  const remembered = memory.current[scope];
  if (remembered) return remembered;
  const storageKey = `henukit.practice.idempotency.v1:${scope}`;
  try {
    const existing = window.sessionStorage.getItem(storageKey);
    if (existing) {
      memory.current[scope] = existing;
      return existing;
    }
    const created = createBrowserKey(prefix);
    window.sessionStorage.setItem(storageKey, created);
    memory.current[scope] = created;
    return created;
  } catch {
    // Storage can be disabled by the browser. Keep the key in this component
    // so a logical retry still reaches the same Core idempotency record; no
    // user answer or Core identity is persisted.
    const created = createBrowserKey(prefix);
    memory.current[scope] = created;
    return created;
  }
}

function clearIdempotencyKey(scope: string, memory: IdempotencyMemory) {
  delete memory.current[scope];
  try {
    window.sessionStorage.removeItem(`henukit.practice.idempotency.v1:${scope}`);
  } catch {
    // A disabled storage area has nothing to clear.
  }
}

function hasAnswer(question: PortalPracticeQuestion, value: unknown) {
  if (question.type === "multi") return Array.isArray(value) && value.length > 0;
  if (question.type === "blank") return typeof value === "string" && value.trim().length > 0;
  if (question.type === "judge") return typeof value === "boolean";
  return typeof value === "number";
}

function optionIsExpected(expected: unknown, option: string, index: number): boolean {
  if (Array.isArray(expected)) return expected.some((item) => optionIsExpected(item, option, index));
  return expected === index || expected === option;
}

function expectedAnswerText(expected: unknown, question: PortalPracticeQuestion) {
  const textFor = (value: unknown): string => {
    if (typeof value === "number" && Number.isInteger(value) && question.options?.[value]) {
      return `${OPTION_LABEL[value] ?? value + 1}. ${question.options[value]}`;
    }
    if (typeof value === "boolean") return value ? "正确" : "错误";
    if (typeof value === "string") return value;
    try {
      return JSON.stringify(value);
    } catch {
      return "服务端已返回答案";
    }
  };
  return Array.isArray(expected) ? expected.map(textFor).join("、") : textFor(expected);
}

function questionTypeLabel(type: PortalPracticeQuestion["type"]) {
  return { single: "单选题", multi: "多选题", judge: "判断题", blank: "填空题" }[type];
}

export default function QuizPage() {
  const cardRef = usePageEnter<HTMLDivElement>("question");
  const explainRef = useRef<HTMLDivElement>(null);
  const idempotencyKeys = useRef<Record<string, string>>({});
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [session, setSession] = useState<PortalPracticeSessionResponse["data"] | null>(null);
  const [restart, setRestart] = useState(0);
  const [idx, setIdx] = useState(0);
  const [answers, setAnswers] = useState<Record<string, AnswerResult>>({});
  const [drafts, setDrafts] = useState<Record<string, unknown>>({});
  // Once a command begins, this is the immutable browser-side payload for
  // that question. An error means retry the same Core command, never change
  // the answer underneath its idempotency key.
  const [lockedDrafts, setLockedDrafts] = useState<Record<string, unknown>>({});
  const [answerError, setAnswerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [streak, setStreak] = useState(0);
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      const parsed = practiceInputFromLocation();
      if (!parsed.input || !parsed.scope) {
        if (!cancelled) {
          setLoadError(parsed.error ?? "未选择真实题库。");
          setLoadState("missing-selection");
        }
        return;
      }
      const key = idempotencyKeyFor(`create:${parsed.scope}`, "practice-create", idempotencyKeys);
      try {
        const response = await createPracticeSession(parsed.input, key);
        if (cancelled) return;
        setSession(response.data);
        setLoadState(response.data.questions.length === 0 ? "empty" : "ready");
      } catch (error) {
        if (cancelled) return;
        setLoadError(formatPortalError(error));
        setLoadState("error");
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [restart]);

  useEffect(() => {
    if (loadState !== "ready") return;
    const id = window.setInterval(() => setElapsed((value) => value + 1), 1000);
    return () => window.clearInterval(id);
  }, [loadState]);

  const questions = session?.questions ?? [];
  const question = questions[idx];
  const currentKey = question ? questionKey(question) : "";
  const result = currentKey ? answers[currentKey] : undefined;
  const answerLocked = currentKey !== "" && Object.hasOwn(lockedDrafts, currentKey);
  const selected = currentKey ? (answerLocked ? lockedDrafts[currentKey] : drafts[currentKey]) : undefined;
  const correctCount = Object.values(answers).filter((item) => item.correct).length;
  const answeredCount = Object.keys(answers).length;

  const animateExplanation = () => {
    const panel = explainRef.current;
    if (!panel) return;
    if (window.matchMedia(REDUCED_MOTION).matches) {
      gsap.set(panel, { height: "auto" });
    } else {
      gsap.to(panel, { height: "auto", duration: 0.35, ease: "power2.out" });
    }
  };

  const goToQuestion = (nextIndex: number) => {
    setIdx(nextIndex);
    setAnswerError(null);
    if (explainRef.current) gsap.set(explainRef.current, { height: 0 });
    if (cardRef.current && !window.matchMedia(REDUCED_MOTION).matches) {
      gsap.fromTo(
        cardRef.current,
        { x: 24, autoAlpha: 0 },
        { x: 0, autoAlpha: 1, duration: 0.35, ease: "power2.out" }
      );
    }
  };

  const setSelected = (value: unknown) => {
    if (!question || result || answerLocked) return;
    setDrafts((current) => ({ ...current, [currentKey]: value }));
  };

  const submit = () => {
    if (!session || !question || result || submitting || !hasAnswer(question, selected)) return;
    const submittedAnswer = selected;
    if (!answerLocked) {
      setLockedDrafts((current) => ({ ...current, [currentKey]: submittedAnswer }));
    }
    setSubmitting(true);
    setAnswerError(null);
    const idempotencyKey = idempotencyKeyFor(
      `answer:${session.session_id}:${question.question_id}:${question.question_version_id}`,
      "practice-answer",
      idempotencyKeys
    );
    void submitPracticeAnswer(
      session.session_id,
      {
        question_id: question.question_id,
        question_version_id: question.question_version_id,
        answer: submittedAnswer,
      },
      idempotencyKey
    )
      .then((response) => {
        const serverResult = response.data;
        setAnswers((current) => ({ ...current, [currentKey]: serverResult }));
        setStreak((current) => (serverResult.correct ? current + 1 : 0));
        if (!serverResult.correct && cardRef.current) {
          gsap.fromTo(
            cardRef.current,
            { x: 0 },
            { keyframes: [{ x: -7 }, { x: 6 }, { x: -3 }, { x: 0 }], duration: 0.35, ease: "power1.inOut" }
          );
        }
        animateExplanation();
      })
      .catch((error: unknown) => setAnswerError(formatPortalError(error)))
      .finally(() => setSubmitting(false));
  };

  const prepareSessionLoad = () => {
    setLoadState("loading");
    setLoadError(null);
    setSession(null);
    setIdx(0);
    setAnswers({});
    setDrafts({});
    setLockedDrafts({});
    setAnswerError(null);
    setStreak(0);
    setElapsed(0);
    if (explainRef.current) gsap.set(explainRef.current, { height: 0 });
  };

  const retrySessionLoad = () => {
    prepareSessionLoad();
    setRestart((value) => value + 1);
  };

  const startAnotherSession = () => {
    const parsed = practiceInputFromLocation();
    if (parsed.scope) clearIdempotencyKey(`create:${parsed.scope}`, idempotencyKeys);
    retrySessionLoad();
  };

  if (loadState === "loading") {
    return <PracticeState title="正在连接题库" detail="正在创建练习会话…" />;
  }
  if (loadState === "missing-selection") {
    return <PracticeState title="请先选择题库" detail={loadError ?? "请从题库目录选择练习后开始。"} />;
  }
  if (loadState === "error") {
    return (
      <PracticeState
        title="练习暂时不可用"
        detail={loadError ?? "暂时无法创建练习会话，请稍后重试。"}
        actionLabel="重试"
        onAction={retrySessionLoad}
      />
    );
  }
  if (loadState === "empty") {
    return <PracticeState title="当前题库没有可练习题目" detail="请返回题库目录重新选择。" />;
  }
  if (!session || !question) {
    return <PracticeState title="练习会话无效" detail="请返回题库目录重新选择。" />;
  }

  const weakChapters = Array.from(
    new Set(questions.filter((item) => answers[questionKey(item)]?.correct === false).map((item) => item.chapter))
  );

  if (loadState === "finished") {
    const accuracy = questions.length ? Math.round((correctCount / questions.length) * 100) : 0;
    return (
      <main className="mx-auto max-w-3xl px-5 py-16 md:px-8">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">RESULT</span>
          <span className="mx-2">/</span>
          本组结算
        </p>
        <div data-enter className="mt-6 border border-ink p-8 md:p-12">
          <p className="font-display text-7xl font-bold md:text-8xl">
            {accuracy}
            <span className="ml-2 font-mono text-base font-normal text-ink/50">%</span>
          </p>
          <div className="mt-8 grid grid-cols-3 gap-4 border-t border-line pt-6 font-mono text-xs">
            <div>
              <p className="text-ink/40">正确率</p>
              <p className="mt-1 text-xl">{accuracy}%</p>
            </div>
            <div>
              <p className="text-ink/40">用时</p>
              <p className="mt-1 text-xl">{fmtTime(elapsed)}</p>
            </div>
            <div>
              <p className="text-ink/40">答对</p>
              <p className="mt-1 text-xl">{correctCount}/{questions.length}</p>
            </div>
          </div>
          <div className="mt-6 border-t border-line pt-6">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">薄弱章节</p>
            {weakChapters.length ? (
              <div className="mt-2 flex flex-wrap gap-2">
                {weakChapters.map((chapter) => (
                  <span key={chapter} className="border border-accent px-2 py-1 font-mono text-xs text-accent">{chapter}</span>
                ))}
              </div>
            ) : (
              <p className="mt-2 font-mono text-xs text-ink/60">当前没有错误题目。</p>
            )}
          </div>
        </div>
        <div data-enter className="mt-8 flex flex-wrap gap-4">
          <button
            type="button"
            onClick={startAnotherSession}
            className="border border-ink bg-ink px-7 py-3.5 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
          >
            再来一组
          </button>
          <TransitionLink
            href="/practice"
            className="border border-ink/30 px-7 py-3.5 font-mono text-sm tracking-widest text-ink transition-colors hover:border-accent hover:text-accent"
          >
            返回题库目录 →
          </TransitionLink>
        </div>
      </main>
    );
  }

  const confirmed = result !== undefined;
  const optionIndexes = Array.isArray(selected) ? selected.filter((value): value is number => typeof value === "number") : [];
  const options = question.options ?? [];

  return (
    <main className="mx-auto max-w-6xl px-5 py-10 md:px-8">
      <div data-block data-enter className="flex flex-wrap items-center gap-x-8 gap-y-3 border-b border-line pb-4">
        <p className="font-mono text-sm">第 <span className="text-accent">{idx + 1}</span> / {questions.length} 题</p>
        <div className="h-1 min-w-32 flex-1 bg-ink/10">
          <div className="h-full bg-accent transition-[width] duration-300" style={{ width: `${((idx + (confirmed ? 1 : 0)) / questions.length) * 100}%` }} />
        </div>
        <p className="font-mono text-xs tracking-widest text-ink/60">TIME {fmtTime(elapsed)}</p>
        <p className="font-mono text-xs tracking-widest text-ink/60">已答 <span className="text-accent">{answeredCount}</span></p>
        <p className="font-mono text-xs tracking-widest text-ink/60">连对 <span className={streak >= 3 ? "text-accent" : ""}>{streak}</span></p>
      </div>

      <div className="mt-8 grid gap-8 lg:grid-cols-[minmax(0,1fr)_12rem]">
        <div data-block>
          <div ref={cardRef} className="border border-ink bg-paper p-6 md:p-8">
            <div className="flex flex-wrap items-center gap-3">
              <span className="font-mono text-xs text-accent">Q-{String(idx + 1).padStart(2, "0")}</span>
              <span className="border border-line px-2 py-0.5 font-mono text-[10px] text-ink/60">{question.chapter}</span>
              <span className="border border-line px-2 py-0.5 font-mono text-[10px] text-ink/60">{questionTypeLabel(question.type)}</span>
            </div>
            <h1 className="mt-5 text-xl font-medium leading-relaxed md:text-2xl">{question.content}</h1>

            <div className="mt-8 space-y-3">
              {(question.type === "single" || question.type === "multi") && options.map((option, optionIndex) => {
                const selectedHere = question.type === "multi" ? optionIndexes.includes(optionIndex) : selected === optionIndex;
                const expectedHere = confirmed && optionIsExpected(result.expected_answer, option, optionIndex);
                return (
                  <button
                    key={optionIndex}
                    type="button"
                    disabled={confirmed || answerLocked}
                    onClick={() => {
                      if (question.type === "multi") {
                        setSelected(selectedHere ? optionIndexes.filter((value) => value !== optionIndex) : [...optionIndexes, optionIndex]);
                      } else {
                        setSelected(optionIndex);
                      }
                    }}
                    className={cn(
                    "flex w-full items-center gap-4 border px-4 py-3 text-left transition-colors",
                    !confirmed && selectedHere && "border-ink border-l-4 border-l-accent",
                    !confirmed && !selectedHere && !answerLocked && "border-line hover:border-ink/40",
                    !confirmed && !selectedHere && answerLocked && "border-line opacity-50",
                      confirmed && expectedHere && "border-ink bg-ink text-paper",
                      confirmed && selectedHere && !expectedHere && "border-accent bg-accent text-paper",
                      confirmed && !expectedHere && !selectedHere && "border-line opacity-50"
                    )}
                  >
                    <span className="font-mono text-xs">{OPTION_LABEL[optionIndex] ?? optionIndex + 1}</span>
                    <span className="flex-1 text-sm md:text-base">{option}</span>
                    {confirmed && expectedHere && <span className="font-mono text-xs">✓</span>}
                    {confirmed && selectedHere && !expectedHere && <span className="font-mono text-xs">✗</span>}
                  </button>
                );
              })}
              {question.type === "judge" && (
                <div className="grid grid-cols-2 gap-3">
                  {[true, false].map((value) => {
                    const selectedHere = selected === value;
                    const expectedHere = confirmed && result.expected_answer === value;
                    return (
                      <button
                        key={String(value)}
                        type="button"
                        disabled={confirmed || answerLocked}
                        onClick={() => setSelected(value)}
                        className={cn(
                          "border px-4 py-3 font-mono text-sm transition-colors",
                          !confirmed && selectedHere && "border-ink bg-ink text-paper",
                          !confirmed && !selectedHere && !answerLocked && "border-line hover:border-ink/40",
                          !confirmed && !selectedHere && answerLocked && "border-line opacity-50",
                          confirmed && expectedHere && "border-ink bg-ink text-paper",
                          confirmed && selectedHere && !expectedHere && "border-accent bg-accent text-paper",
                          confirmed && !expectedHere && !selectedHere && "border-line opacity-50"
                        )}
                      >
                        {value ? "正确" : "错误"}
                      </button>
                    );
                  })}
                </div>
              )}
              {question.type === "blank" && (
                <input
                  aria-label="填写答案"
                  type="text"
                  disabled={confirmed || answerLocked}
                  value={typeof selected === "string" ? selected : ""}
                  onChange={(event) => setSelected(event.target.value)}
                  className="w-full border border-line bg-transparent px-4 py-3 text-sm outline-none transition-colors focus:border-ink disabled:opacity-60"
                  placeholder="输入答案"
                />
              )}
              {(question.type === "single" || question.type === "multi") && options.length === 0 && (
                <p className="border border-accent px-4 py-3 text-sm text-accent">这道题的选项不可用，不能以本地题目替代。</p>
              )}
            </div>

            <div ref={explainRef} className="h-0 overflow-hidden">
              {confirmed && (
                <div className="mt-6 border-t border-line pt-5">
                  <p className="font-mono text-[10px] tracking-[0.25em] text-accent">服务端解析 / EXPLAIN</p>
                  <p className="mt-2 font-mono text-xs text-ink/60">参考答案：{expectedAnswerText(result.expected_answer, question)}</p>
                  <p className="mt-2 text-sm leading-7 text-ink/80">{result.analysis || "本题暂无补充解析。"}</p>
                  {result.replayed && <p className="mt-2 font-mono text-xs text-ink/50">已恢复同一提交的服务端结果。</p>}
                </div>
              )}
            </div>

            {answerError && (
              <p role="alert" className="mt-5 border border-accent px-4 py-3 text-sm text-accent">{answerError}</p>
            )}
            <div className="mt-8 flex flex-wrap gap-4">
              {!confirmed ? (
                <button
                  type="button"
                  onClick={submit}
                  disabled={submitting || !hasAnswer(question, selected) || ((question.type === "single" || question.type === "multi") && options.length === 0)}
                  className={cn(
                    "border px-7 py-3 font-mono text-sm tracking-widest transition-colors",
                    submitting || !hasAnswer(question, selected) || ((question.type === "single" || question.type === "multi") && options.length === 0)
                      ? "cursor-not-allowed border-line text-ink/30"
                      : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                  )}
                >
                  {submitting ? "正在提交…" : answerError ? "重试提交" : "确认"}
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    if (idx === questions.length - 1) setLoadState("finished");
                    else goToQuestion(idx + 1);
                  }}
                  className="border border-ink bg-ink px-7 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
                >
                  {idx === questions.length - 1 ? "查看结算 →" : "下一题 →"}
                </button>
              )}
            </div>
          </div>
        </div>

        <aside data-enter className="hidden lg:block">
          <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">INDEX / 跳题</p>
          <div className="mt-3 grid grid-cols-4 gap-1.5">
            {questions.map((item, itemIndex) => {
              const itemResult = answers[questionKey(item)];
              return (
                <button
                  key={questionKey(item)}
                  type="button"
                  onClick={() => goToQuestion(itemIndex)}
                  className={cn(
                    "flex h-9 items-center justify-center border font-mono text-[11px] transition-colors",
                    itemIndex === idx && "border-accent text-accent",
                    itemIndex !== idx && !itemResult && "border-line text-ink/50 hover:border-ink/40",
                    itemIndex !== idx && itemResult?.correct && "border-ink bg-ink text-paper",
                    itemIndex !== idx && itemResult && !itemResult.correct && "border-accent bg-accent text-paper"
                  )}
                >
                  {String(itemIndex + 1).padStart(2, "0")}
                </button>
              );
            })}
          </div>
          <div className="mt-4 space-y-1.5 font-mono text-[10px] text-ink/50">
            <p><span className="mr-2 inline-block h-2 w-2 border border-line align-middle" />未答</p>
            <p><span className="mr-2 inline-block h-2 w-2 bg-ink align-middle" />服务端判对</p>
            <p><span className="mr-2 inline-block h-2 w-2 bg-accent align-middle" />服务端判错</p>
          </div>
        </aside>
      </div>
    </main>
  );
}

function PracticeState({
  title,
  detail,
  actionLabel,
  onAction,
}: {
  title: string;
  detail: string;
  actionLabel?: string;
  onAction?: () => void;
}) {
  return (
    <main className="mx-auto max-w-3xl px-5 py-16 md:px-8">
      <div data-enter className="border border-ink p-8 md:p-12">
        <p className="font-mono text-xs tracking-[0.3em] text-accent">PRACTICE / REAL DATA</p>
        <h1 className="mt-5 text-2xl font-medium md:text-3xl">{title}</h1>
        <p className="mt-4 max-w-xl text-sm leading-7 text-ink/70">{detail}</p>
        <div className="mt-8 flex flex-wrap gap-4">
          {actionLabel && onAction && (
            <button type="button" onClick={onAction} className="border border-ink bg-ink px-6 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent">
              {actionLabel}
            </button>
          )}
          <TransitionLink href="/practice" className="border border-ink/30 px-6 py-3 font-mono text-sm tracking-widest text-ink transition-colors hover:border-accent hover:text-accent">
            返回题库目录 →
          </TransitionLink>
        </div>
      </div>
    </main>
  );
}
