"use client";

import {
  QuizListMeta,
  Question,
  getListQuestions,
  questionStatus,
} from "@/lib/practice/mock";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import TransitionLink from "@/components/practice/transition/transition-link";
import DiffBadge from "@/components/practice/diff-badge";
import { cn } from "@/lib/cn";

const STATUS_LABEL = ["未做", "已做", "做错"] as const;

export default function ListDetail({ list }: { list: QuizListMeta }) {
  const questions = getListQuestions(list);
  // 形变主线 1 的落点：歌单封面
  const coverRef = usePageEnter<HTMLDivElement>("list", list.id);

  return (
    <main className="mx-auto max-w-6xl px-5 py-12 md:px-8 md:py-16">
      {/* 歌单头部 */}
      <div data-block className="flex flex-col gap-8 md:flex-row">
        <div
          ref={coverRef}
          className="bg-blueprint relative h-44 w-44 shrink-0 border border-ink md:h-52 md:w-52"
        >
          <span className="absolute left-3 top-3 font-mono text-[10px] tracking-[0.3em] text-ink/50">
            LIST
          </span>
          <span className="absolute bottom-3 left-3 font-display text-4xl font-bold">
            {list.count}
            <span className="ml-1 font-mono text-xs font-normal text-ink/50">题</span>
          </span>
          <span aria-hidden className="absolute bottom-3 right-3 font-mono text-accent">+</span>
          <span aria-hidden className="absolute right-0 top-0 h-5 w-5 bg-accent" />
        </div>

        <div className="flex flex-1 flex-col justify-end">
          <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">LIST</span>
            <span className="mx-2">/</span>
            {list.tags.join(" · ")}
          </p>
          <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
            {list.name}
          </h1>
          <p data-enter className="mt-3 font-mono text-xs tracking-wider text-ink/50">
            创建者 {list.creator} · {questions.length} 题 · 完成度 {list.completion}%
          </p>
          <div data-enter className="mt-6">
            <TransitionLink
              href="/practice/quiz"
              className="group relative inline-flex items-center gap-3 overflow-hidden border border-ink px-7 py-3.5 font-mono text-sm tracking-widest text-ink"
            >
              <span
                aria-hidden
                className="absolute inset-0 translate-y-full bg-accent transition-transform duration-300 ease-out group-hover:translate-y-0"
              />
              <span className="relative z-10 transition-colors duration-300 group-hover:text-paper">
                开始刷题
              </span>
              <span aria-hidden className="relative z-10 transition-colors duration-300 group-hover:text-paper">
                →
              </span>
            </TransitionLink>
          </div>
        </div>
      </div>

      {/* 全部题目（按难度分升序） */}
      <div data-block className="mt-14">
        <div data-enter className="grid grid-cols-[3rem_1fr_4rem_4rem_3rem] items-center gap-3 border-b border-ink/40 pb-2 font-mono text-[10px] tracking-[0.25em] text-ink/40 md:grid-cols-[4rem_1fr_6rem_5rem_4rem]">
          <span>题号</span>
          <span>题目</span>
          <span>难度分</span>
          <span>正确率</span>
          <span className="text-right">状态</span>
        </div>
        <ul data-enter>
          {questions.map((q, i) => (
            <QuestionRow key={q.id} q={q} index={i} listId={list.id} />
          ))}
        </ul>
      </div>
    </main>
  );
}

function QuestionRow({
  q,
  index,
  listId,
}: {
  q: Question;
  index: number;
  listId: string;
}) {
  const status = questionStatus(listId, index);

  return (
    <li>
      <TransitionLink
        href="/practice/quiz"
        morph={{ kind: "question", title: q.stem, sub: `${q.subject} · ${q.chapter}` }}
        className="group grid grid-cols-[3rem_1fr_4rem_4rem_3rem] items-center gap-3 border-b border-line py-3.5 transition-colors hover:bg-ink/[0.03] md:grid-cols-[4rem_1fr_6rem_5rem_4rem]"
      >
        <span className="font-mono text-xs text-ink/40 group-hover:text-accent">
          Q-{String(index + 1).padStart(2, "0")}
        </span>
        <span className="truncate text-sm">{q.stem}</span>
        <span>
          <DiffBadge value={q.difficulty} />
        </span>
        <span className="font-mono text-xs text-ink/60">{q.accuracy}%</span>
        <span
          className={cn(
            "text-right font-mono text-xs",
            status === 1 && "text-ink/60",
            status === 2 && "text-accent",
            status === 0 && "text-ink/30"
          )}
        >
          {STATUS_LABEL[status]}
        </span>
      </TransitionLink>
    </li>
  );
}
