"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { Button } from "@/components/ui/button";
import { PortalUnauthorizedError } from "@/lib/api/client";
import { createTrainingSession, fetchTrainingCatalog } from "@/lib/api/training";
import type { TrainingCatalogItem } from "@/lib/api/generated/training";
import { quizCraftCatalogEnabled } from "@/lib/api/env";
import { createIdempotencyKey } from "@/lib/practice/session-handoff";
import TrainingAttributionList from "./training-attribution";
import { usePageEnter } from "./transition/use-page-enter";

export default function TrainingCatalog() {
  usePageEnter(null);
  const router = useRouter();
  const [units, setUnits] = useState<TrainingCatalogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [retryUnit, setRetryUnit] = useState<TrainingCatalogItem | null>(null);
  const [starting, setStarting] = useState<string | null>(null);
  const keys = useRef<Record<string, string>>({});
  const busy = useRef(false);
  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    try {
      if (!quizCraftCatalogEnabled()) { setUnits([]); return; }
      const response = await fetchTrainingCatalog(signal);
      if (!signal?.aborted) setUnits(response.data.units);
    } catch {
      if (!signal?.aborted) setError("训练材料暂时加载不出来，请稍后重试。");
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(controller.signal), 0);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [load]);

  async function start(unit: TrainingCatalogItem) {
    if (busy.current) return;
    busy.current = true;
    setStarting(unit.unit_version_id);
    setRetryUnit(null);
    setError(null);
    const storageKey = `training-start:${unit.unit_version_id}`;
    let remembered: string | null = null;
    try { remembered = sessionStorage.getItem(storageKey); } catch { /* Retry also works in this mounted page. */ }
    const key = keys.current[unit.unit_version_id] ??= remembered && /^[A-Za-z0-9._:-]{16,160}$/.test(remembered) ? remembered : createIdempotencyKey("training-create");
    try { sessionStorage.setItem(storageKey, key); } catch { /* Storage is optional. */ }
    try {
      const response = await createTrainingSession({ unit_id: unit.unit_id, unit_version_id: unit.unit_version_id }, key);
      try { sessionStorage.removeItem(storageKey); } catch { /* No learning facts are cached here. */ }
      router.push(`/practice/training/${response.data.session_id}`);
    } catch (cause) {
      setError(cause instanceof PortalUnauthorizedError ? "登录状态已失效，请重新登录后重试。" : "训练暂时无法开始，请重试。重试会继续这次操作。");
      setRetryUnit(unit);
      setStarting(null);
      busy.current = false;
    }
  }

  return (
    <main className="mx-auto max-w-site px-5 py-8 md:px-8 md:py-12">
      <div className="max-w-3xl">
        <p className="font-mono text-xs tracking-[0.2em] text-accent-text">ENGLISH / TRAINING</p>
        <h1 className="mt-3 font-display text-3xl font-bold md:text-5xl">英语听力与完形训练</h1>
        <p className="mt-4 text-sm leading-7 text-ink/70">通用英语练习：听力核对音频依据，完形结合全文语境作答。完成后查看解析与译文，训练结果单独记录。</p>
        <Button asChild variant="outline" className="mt-5"><Link href="/practice/training/review">待复习训练</Link></Button>
        {error && <ErrorBanner message={error} className="mt-6" onRetry={starting ? undefined : retryUnit ? () => void start(retryUnit) : () => void load()} />}
        {loading ? <div className="mt-8"><LoadingBlock label="加载训练材料" /></div> : units.length === 0 && !error ? <div className="mt-8"><EmptyBlock label="暂时没有可练习的训练材料。" action={{ label: "返回题库", href: "/practice" }} /></div> : (
          <div className="mt-8 divide-y divide-line border-y border-line">
            {units.map(unit => <article key={unit.unit_version_id} className="py-6">
              <p className="font-mono text-xs text-ink/60">{unit.kind === "listening" ? "听力" : "完形"} · {unit.question_count} 题</p>
              <h2 className="mt-2 font-display text-2xl font-bold">{unit.title}</h2>
              <Button className="mt-5" variant="accent" disabled={starting !== null} onClick={() => void start(unit)}>{starting === unit.unit_version_id ? "正在开始…" : unit.kind === "listening" ? "开始听力训练" : "开始完形训练"}</Button>
              <TrainingAttributionList records={unit.attributions} />
            </article>)}
          </div>
        )}
      </div>
    </main>
  );
}
