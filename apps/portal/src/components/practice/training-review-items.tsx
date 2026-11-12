"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { Button } from "@/components/ui/button";
import { PortalUnauthorizedError } from "@/lib/api/client";
import { fetchTrainingReviewItems } from "@/lib/api/training";
import type { TrainingReviewItem } from "@/lib/api/generated/training";
import { usePageEnter } from "./transition/use-page-enter";

export default function TrainingReviewItems() {
  usePageEnter(null);
  const [items, setItems] = useState<TrainingReviewItem[]>([]);
  const [cursor, setCursor] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const busy = useRef(false);
  const alive = useRef(true);
  const load = useCallback(async (next?: string, signal?: AbortSignal) => {
    if (busy.current) return;
    busy.current = true;
    setLoading(true);
    setError(null);
    try {
      const response = await fetchTrainingReviewItems(next, signal);
      if (!alive.current || signal?.aborted) return;
      setItems(previous => next ? Array.from(new Map([...previous, ...response.data.items].map(item => [item.unit_version_id, item])).values()) : response.data.items);
      setCursor(response.data.next_cursor);
    } catch (cause) {
      if (alive.current && !signal?.aborted) setError(cause instanceof PortalUnauthorizedError ? "登录状态已失效，请重新登录后查看复习记录。" : "复习记录暂时无法读取，请重试；也可返回目录开始训练。");
    } finally {
      busy.current = false;
      if (alive.current && !signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(undefined, controller.signal), 0);
    return () => { alive.current = false; window.clearTimeout(timer); controller.abort(); };
  }, [load]);

  return <main className="mx-auto max-w-site px-5 py-8 md:px-8 md:py-12"><div className="max-w-3xl">
    <p className="font-mono text-xs tracking-[0.2em] text-accent-text">ENGLISH / REVIEW</p>
    <h1 className="mt-3 font-display text-3xl font-bold md:text-4xl">待复习训练</h1>
    <p className="mt-4 text-sm leading-7 text-ink/70">回到整组材料，核对音频或上下文依据与练习点。新的独立作答会更新待复习记录，首次答案仍保留。</p>
    {error && <ErrorBanner className="mt-6" message={error} onRetry={() => void load()} />}
    {items.length > 0 && <div className="mt-8 divide-y divide-line border-y border-line">{items.map(item => <article key={item.unit_version_id} className="py-6">
      <p className="font-mono text-xs text-ink/60">{item.kind === "listening" ? "听力" : "完形"} · {item.review_required_count} 题需要独立复习</p>
      <h2 className="mt-2 break-words font-display text-2xl font-bold">{item.title}</h2>
      <Button asChild variant="outline" className="mt-4"><Link href={`/practice/training/${item.session_id}`}>{item.phase === "completed" ? "打开完整复盘" : "继续未完成训练"}</Link></Button>
    </article>)}</div>}
    {loading ? <div className="mt-6"><LoadingBlock label="读取复习记录" /></div> : !error && items.length === 0 ? <div className="mt-8"><EmptyBlock label="目前没有需要独立复习的训练。" action={{ label: "选择训练材料", href: "/practice/training" }} /></div> : null}
    {cursor && !error && <Button type="button" variant="outline" className="mt-5" disabled={loading} onClick={() => void load(cursor)}>继续读取复习记录</Button>}
    <p className="mt-6 text-xs leading-6 text-ink/70">复习记录跟随当前账号，或当前浏览器保留的匿名身份。重新练习请从目录选择当前发布的材料。</p>
    <Button asChild variant="ghost" className="mt-3"><Link href="/practice/training">返回训练目录</Link></Button>
  </div></main>;
}
