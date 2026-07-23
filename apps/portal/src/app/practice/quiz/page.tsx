"use client";

import { useEffect, useRef, useState } from "react";
import { QUIZ_SET } from "@/lib/practice/mock";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import TransitionLink from "@/components/practice/transition/transition-link";
import DiffBadge from "@/components/practice/diff-badge";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const SET = QUIZ_SET;
const N = SET.length;
const OPTION_LABEL = ["A", "B", "C", "D"];

function fmtTime(sec: number) {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

export default function QuizPage() {
  // 形变主线 2 的落点：题干卡
  const cardRef = usePageEnter<HTMLDivElement>("question");

  const [idx, setIdx] = useState(0);
  const [selected, setSelected] = useState<number | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [results, setResults] = useState<Array<boolean | null>>(() =>
    Array(N).fill(null)
  );
  const [streak, setStreak] = useState(0);
  const [elapsed, setElapsed] = useState(0);
  const [finished, setFinished] = useState(false);
  const explainRef = useRef<HTMLDivElement>(null);

  const q = SET[idx];
  const correctCount = results.filter((r) => r === true).length;

  // 计时器：挂载后启动（初始 0 与 SSR 一致），结算后停止
  useEffect(() => {
    if (finished) return;
    const id = setInterval(() => setElapsed((v) => v + 1), 1000);
    return () => clearInterval(id);
  }, [finished]);

  const confirm = () => {
    if (selected === null || confirmed) return;
    const correct = selected === q.answer;
    setResults((r) => r.map((v, i) => (i === idx ? correct : v)));
    setConfirmed(true);
    setStreak((s) => (correct ? s + 1 : 0));
    if (!correct && cardRef.current) {
      gsap.fromTo(
        cardRef.current,
        { x: 0 },
        { keyframes: [{ x: -7 }, { x: 6 }, { x: -3 }, { x: 0 }], duration: 0.35, ease: "power1.inOut" }
      );
    }
    // 展开 AI 讲解
    const panel = explainRef.current;
    if (panel) {
      if (window.matchMedia(REDUCED_MOTION).matches) {
        gsap.set(panel, { height: "auto" });
      } else {
        gsap.to(panel, { height: "auto", duration: 0.35, ease: "power2.out" });
      }
    }
  };

  const gotoQuestion = (i: number) => {
    setIdx(i);
    setSelected(null);
    setConfirmed(false);
    if (explainRef.current) gsap.set(explainRef.current, { height: 0 });
    if (cardRef.current && !window.matchMedia(REDUCED_MOTION).matches) {
      gsap.fromTo(
        cardRef.current,
        { x: 24, autoAlpha: 0 },
        { x: 0, autoAlpha: 1, duration: 0.35, ease: "power2.out" }
      );
    }
  };

  const next = () => {
    if (idx === N - 1) {
      setFinished(true);
      return;
    }
    gotoQuestion(idx + 1);
  };

  const reset = () => {
    setIdx(0);
    setSelected(null);
    setConfirmed(false);
    setResults(Array(N).fill(null));
    setStreak(0);
    setElapsed(0);
    setFinished(false);
    if (explainRef.current) gsap.set(explainRef.current, { height: 0 });
  };

  const weakChapters = Array.from(
    new Set(
      SET.filter((_, i) => results[i] === false).map((item) => item.chapter)
    )
  );

  if (finished) {
    const accuracy = Math.round((correctCount / N) * 100);
    return (
      <main className="mx-auto max-w-3xl px-5 py-16 md:px-8">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">RESULT</span>
          <span className="mx-2">/</span>
          本组结算
        </p>
        <div data-enter className="mt-6 border border-ink p-8 md:p-12">
          <p className="font-display text-7xl font-bold md:text-8xl">
            {correctCount * 10}
            <span className="ml-2 font-mono text-base font-normal text-ink/50">分</span>
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
              <p className="mt-1 text-xl">
                {correctCount}/{N}
              </p>
            </div>
          </div>
          <div className="mt-6 border-t border-line pt-6">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
              薄弱章节
            </p>
            {weakChapters.length ? (
              <div className="mt-2 flex flex-wrap gap-2">
                {weakChapters.map((c) => (
                  <span key={c} className="border border-accent px-2 py-1 font-mono text-xs text-accent">
                    {c}
                  </span>
                ))}
              </div>
            ) : (
              <p className="mt-2 font-mono text-xs text-ink/60">全对，没有薄弱点。</p>
            )}
          </div>
        </div>
        <div data-enter className="mt-8 flex flex-wrap gap-4">
          <button
            type="button"
            onClick={reset}
            className="border border-ink bg-ink px-7 py-3.5 font-mono text-sm tracking-widest text-paper transition-colors hover:bg-accent hover:border-accent"
          >
            再来一组
          </button>
          <TransitionLink
            href="/practice/lists/ds-tree"
            className="border border-ink/30 px-7 py-3.5 font-mono text-sm tracking-widest text-ink transition-colors hover:border-accent hover:text-accent"
          >
            推荐：树与图专题 →
          </TransitionLink>
        </div>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-6xl px-5 py-10 md:px-8">
      {/* 状态条 */}
      <div data-block data-enter className="flex flex-wrap items-center gap-x-8 gap-y-3 border-b border-line pb-4">
        <p className="font-mono text-sm">
          第 <span className="text-accent">{idx + 1}</span> / {N} 题
        </p>
        <div className="h-1 min-w-32 flex-1 bg-ink/10">
          <div
            className="h-full bg-accent transition-[width] duration-300"
            style={{ width: `${((idx + (confirmed ? 1 : 0)) / N) * 100}%` }}
          />
        </div>
        <p className="font-mono text-xs tracking-widest text-ink/60">
          TIME {fmtTime(elapsed)}
        </p>
        <p className="font-mono text-xs tracking-widest text-ink/60">
          连对 <span className={streak >= 3 ? "text-accent" : ""}>{streak}</span>
        </p>
      </div>

      <div className="mt-8 grid gap-8 lg:grid-cols-[minmax(0,1fr)_12rem]">
        {/* 题干卡 */}
        <div data-block>
          <div ref={cardRef} className="border border-ink bg-paper p-6 md:p-8">
            <div className="flex flex-wrap items-center gap-3">
              <span className="font-mono text-xs text-accent">
                Q-{String(idx + 1).padStart(2, "0")}
              </span>
              <span className="border border-line px-2 py-0.5 font-mono text-[10px] text-ink/60">
                {q.subject} · {q.chapter}
              </span>
              <DiffBadge value={q.difficulty} />
            </div>
            <h1 className="mt-5 text-xl font-medium leading-relaxed md:text-2xl">
              {q.stem}
            </h1>

            {/* 选项 */}
            <div className="mt-8 space-y-3">
              {q.options.map((opt, i) => {
                const isSel = selected === i;
                const isAns = q.answer === i;
                return (
                  <button
                    key={i}
                    type="button"
                    disabled={confirmed}
                    onClick={() => setSelected(i)}
                    className={cn(
                      "flex w-full items-center gap-4 border px-4 py-3 text-left transition-colors",
                      !confirmed && isSel && "border-ink border-l-4 border-l-accent",
                      !confirmed && !isSel && "border-line hover:border-ink/40",
                      confirmed && isAns && "border-ink bg-ink text-paper",
                      confirmed && isSel && !isAns && "border-accent bg-accent text-paper",
                      confirmed && !isAns && !isSel && "border-line opacity-50"
                    )}
                  >
                    <span className="font-mono text-xs">{OPTION_LABEL[i]}</span>
                    <span className="flex-1 text-sm md:text-base">{opt}</span>
                    {confirmed && isAns && <span className="font-mono text-xs">✓</span>}
                    {confirmed && isSel && !isAns && <span className="font-mono text-xs">✗</span>}
                  </button>
                );
              })}
            </div>

            {/* AI 讲解 */}
            <div ref={explainRef} className="h-0 overflow-hidden">
              <div className="mt-6 border-t border-line pt-5">
                <p className="font-mono text-[10px] tracking-[0.25em] text-accent">
                  AI 讲解 / EXPLAIN
                </p>
                <p className="mt-2 text-sm leading-7 text-ink/80">{q.explanation}</p>
              </div>
            </div>

            {/* 操作 */}
            <div className="mt-8 flex gap-4">
              {!confirmed ? (
                <button
                  type="button"
                  onClick={confirm}
                  disabled={selected === null}
                  className={cn(
                    "border px-7 py-3 font-mono text-sm tracking-widest transition-colors",
                    selected === null
                      ? "cursor-not-allowed border-line text-ink/30"
                      : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                  )}
                >
                  确认
                </button>
              ) : (
                <button
                  type="button"
                  onClick={next}
                  className="border border-ink bg-ink px-7 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
                >
                  {idx === N - 1 ? "查看结算 →" : "下一题 →"}
                </button>
              )}
            </div>
          </div>
        </div>

        {/* 题号导航格（桌面显示） */}
        <aside data-enter className="hidden lg:block">
          <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
            INDEX / 跳题
          </p>
          <div className="mt-3 grid grid-cols-4 gap-1.5">
            {SET.map((_, i) => (
              <button
                key={i}
                type="button"
                onClick={() => gotoQuestion(i)}
                className={cn(
                  "flex h-9 items-center justify-center border font-mono text-[11px] transition-colors",
                  i === idx && "border-accent text-accent",
                  i !== idx && results[i] === null && "border-line text-ink/50 hover:border-ink/40",
                  i !== idx && results[i] === true && "border-ink bg-ink text-paper",
                  i !== idx && results[i] === false && "border-accent bg-accent text-paper"
                )}
              >
                {String(i + 1).padStart(2, "0")}
              </button>
            ))}
          </div>
          <div className="mt-4 space-y-1.5 font-mono text-[10px] text-ink/50">
            <p><span className="mr-2 inline-block h-2 w-2 border border-line align-middle" />未答</p>
            <p><span className="mr-2 inline-block h-2 w-2 bg-ink align-middle" />答对</p>
            <p><span className="mr-2 inline-block h-2 w-2 bg-accent align-middle" />答错</p>
          </div>
        </aside>
      </div>
    </main>
  );
}
