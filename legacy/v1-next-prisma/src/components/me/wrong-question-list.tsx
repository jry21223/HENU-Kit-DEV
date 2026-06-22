"use client";

import Link from "next/link";
import { useState } from "react";
import { questionTypeLabels } from "@/constants/enums";
import type { WrongQuestionItem } from "@/types";

type WrongQuestionListProps = {
  initialItems: WrongQuestionItem[];
};

export function WrongQuestionList({ initialItems }: WrongQuestionListProps) {
  const [items, setItems] = useState(initialItems);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [error, setError] = useState("");

  async function removeWrongQuestion(itemId: string) {
    setPendingId(itemId);
    setError("");

    try {
      const response = await fetch(`/api/me/wrong-questions/${itemId}`, {
        method: "DELETE",
      });
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;

      if (!response.ok) {
        setError(payload?.error ?? "移除失败，请稍后重试。");
        return;
      }

      setItems((current) => current.filter((item) => item.id !== itemId));
    } catch {
      setError("网络异常，请稍后重试。");
    } finally {
      setPendingId(null);
    }
  }

  if (items.length === 0) {
    return (
      <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
        当前没有匹配的错题。
      </div>
    );
  }

  return (
    <div className="grid gap-4">
      {error ? (
        <div className="rounded-lg border border-[#e7c5b5] bg-[#fff7f3] p-4 text-sm font-semibold text-[#8b3f24]">
          {error}
        </div>
      ) : null}

      {items.map((item) => (
        <article key={item.id} className="rounded-lg border border-line bg-white p-5 shadow-soft">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <div className="flex flex-wrap gap-2 text-xs">
                <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                  {item.course.name}
                </span>
                <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                  {questionTypeLabels[item.question.type]}
                </span>
                <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                  {item.knowledgePointTitle ?? "未关联知识点"}
                </span>
              </div>
              <p className="mt-3 text-base font-semibold leading-7 text-ink">
                {item.question.stem}
              </p>
              <ul className="mt-3 grid gap-2 text-sm text-muted">
                {item.question.options.map((option) => (
                  <li key={option.id} className="rounded-md border border-line bg-panel px-3 py-2">
                    <span className="font-semibold text-ink">{option.id}.</span> {option.text}
                  </li>
                ))}
              </ul>
              <p className="mt-3 text-xs text-muted">
                记录时间：{new Date(item.createdAt).toLocaleString()}
              </p>
            </div>
            <div className="flex shrink-0 flex-col gap-2 sm:w-32">
              <Link
                href={`/courses/${item.course.id}/practice`}
                className="inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
              >
                重新练习
              </Link>
              <button
                type="button"
                onClick={() => removeWrongQuestion(item.id)}
                disabled={pendingId === item.id}
                className="inline-flex h-10 items-center justify-center rounded-md border border-line bg-white px-4 text-sm font-semibold text-ink hover:bg-panel disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
              >
                {pendingId === item.id ? "移除中" : "移除"}
              </button>
            </div>
          </div>
        </article>
      ))}
    </div>
  );
}
