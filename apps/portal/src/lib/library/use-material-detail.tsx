"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { fetchLibraryMaterialDetail, formatPortalError } from "@/lib/api/client";
import { getMaterialOrFallback } from "@/lib/library/gateway";
import type { Material } from "@/lib/library/mock";

export type LoadState = "loading" | "ready" | "error";

/**
 * 资料详情统一加载：真实 detail 接口 → gateway 缓存/mock 回退 → error。
 * 回退决策复用 gateway.getMaterialOrFallback，避免各页面内联重写。
 */
export function useMaterialDetail(id: string) {
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [material, setMaterial] = useState<Material | null>(null);
  const [error, setError] = useState<string | null>(null);
  const mounted = useRef(true);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const response = await fetchLibraryMaterialDetail(id);
      if (!mounted.current) return;
      const fallback = getMaterialOrFallback(id);
      const found = response?.material ?? fallback;
      if (found) {
        setMaterial(found);
        setLoadState("ready");
        return;
      }
      setMaterial(null);
      setError("内容不存在或已下架");
      setLoadState("error");
    } catch (loadError) {
      if (!mounted.current) return;
      const fallback = getMaterialOrFallback(id);
      if (fallback) {
        setMaterial(fallback);
        setLoadState("ready");
        return;
      }
      setMaterial(null);
      setError(formatPortalError(loadError));
      setLoadState("error");
    }
  }, [id]);

  useEffect(() => {
    mounted.current = true;
    const timer = window.setTimeout(() => void load(), 0);
    return () => {
      mounted.current = false;
      window.clearTimeout(timer);
    };
  }, [load]);

  return { loadState, material, error };
}

export function LibraryLoading() {
  return (
    <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">LOADING / 加载中</p>
      <Link href="/library" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
        ← 返回书库
      </Link>
    </main>
  );
}

export function LibraryNotFound({ error }: { error?: string | null }) {
  return (
    <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">404 / NOT FOUND</p>
      <p className="mt-4 text-sm text-ink/60">
        内容不存在或已下架{error ? `（${error}）` : ""}。
      </p>
      <Link href="/library" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
        ← 返回书库
      </Link>
    </main>
  );
}
