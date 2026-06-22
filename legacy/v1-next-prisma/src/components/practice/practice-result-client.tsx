"use client";

import Link from "next/link";
import { useMemo, useSyncExternalStore } from "react";

type PracticeResultClientProps = {
  courseId: string;
  courseName: string;
};

type StoredPracticeResult = {
  courseId: string;
  courseName: string;
  total: number;
  answered: number;
  correct: number;
  wrong: number;
  updatedAt: string;
};

function subscribeToStorage(callback: () => void) {
  if (typeof window === "undefined") {
    return () => {};
  }

  window.addEventListener("storage", callback);
  return () => window.removeEventListener("storage", callback);
}

export function PracticeResultClient({
  courseId,
  courseName,
}: PracticeResultClientProps) {
  const storageKey = `practice-result:${courseId}`;
  const rawResult = useSyncExternalStore(
    subscribeToStorage,
    () => window.localStorage.getItem(storageKey) ?? "",
    () => "",
  );

  const result = useMemo(() => {
    if (!rawResult) {
      return null;
    }

    try {
      return JSON.parse(rawResult) as StoredPracticeResult;
    } catch {
      return null;
    }
  }, [rawResult]);

  if (!result) {
    return (
      <div className="rounded-lg border border-line bg-white p-6 shadow-soft">
        <p className="text-sm text-muted">当前浏览器没有找到 {courseName} 的练习结果。</p>
        <Link
          href={`/courses/${courseId}/practice`}
          className="mt-4 inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
        >
          开始练习
        </Link>
      </div>
    );
  }

  const correctRate =
    result.answered > 0 ? Math.round((result.correct / result.answered) * 100) : 0;

  return (
    <div className="grid gap-5">
      <section className="rounded-lg border border-line bg-white p-6 shadow-soft">
        <p className="text-sm font-semibold text-muted">{result.courseName}</p>
        <h2 className="mt-2 text-2xl font-semibold text-ink">练习结果</h2>
        <div className="mt-5 grid gap-3 sm:grid-cols-4">
          <div className="rounded-lg border border-line bg-panel p-4">
            <p className="text-xs font-semibold text-muted">题目数</p>
            <p className="mt-2 text-2xl font-semibold text-ink">{result.total}</p>
          </div>
          <div className="rounded-lg border border-line bg-panel p-4">
            <p className="text-xs font-semibold text-muted">已答</p>
            <p className="mt-2 text-2xl font-semibold text-ink">{result.answered}</p>
          </div>
          <div className="rounded-lg border border-line bg-panel p-4">
            <p className="text-xs font-semibold text-muted">正确</p>
            <p className="mt-2 text-2xl font-semibold text-ink">{result.correct}</p>
          </div>
          <div className="rounded-lg border border-line bg-panel p-4">
            <p className="text-xs font-semibold text-muted">正确率</p>
            <p className="mt-2 text-2xl font-semibold text-ink">{correctRate}%</p>
          </div>
        </div>
        <p className="mt-4 text-sm text-muted">
          更新时间：{new Date(result.updatedAt).toLocaleString()}
        </p>
      </section>

      <div className="flex flex-col gap-3 sm:flex-row">
        <Link
          href={`/courses/${courseId}/practice`}
          className="inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
        >
          继续练习
        </Link>
        <Link
          href={`/courses/${courseId}`}
          className="inline-flex h-10 items-center justify-center rounded-md border border-line bg-white px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
        >
          返回课程
        </Link>
      </div>
    </div>
  );
}
