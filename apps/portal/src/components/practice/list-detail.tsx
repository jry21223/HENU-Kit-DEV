"use client";

import {
  QuizListMeta,
  Question,
  getListQuestions,
} from "@/lib/practice/mock";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
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
          <p data-enter className="mt-2 font-mono text-[10px] tracking-wider text-ink/40">
            当前页面为示例题单，正式数据接入中
          </p>
          <div data-enter className="mt-6">
            <button
              type="button"
              disabled
              className="group relative inline-flex cursor-not-allowed items-center gap-3 overflow-hidden border border-line px-7 py-3.5 font-mono text-sm tracking-widest text-ink/40"
            >
              练习入口接入中
            </button>
            <p className="mt-3 font-mono text-[10px] tracking-wider text-ink/45">
              题库对接完成后即可从题单开始刷题。
            </p>
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
            <QuestionRow key={q.id} q={q} index={i} />
          ))}
        </ul>
      </div>
    </main>
  );
}

function QuestionRow({
  q,
  index,
}: {
  q: Question;
  index: number;
}) {
  const status = 0 as 0 | 1 | 2; // 示例题单：不展示伪随机作答状态，统一为未做

  return (
    <li>
      <div
        className="group grid grid-cols-[3rem_1fr_4rem_4rem_3rem] items-center gap-3 border-b border-line py-3.5 md:grid-cols-[4rem_1fr_6rem_5rem_4rem]"
        title="练习入口接入中"
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
      </div>
    </li>
  );
}
