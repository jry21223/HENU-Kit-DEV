"use client";

import Link from "next/link";
import { MATERIAL_TYPES } from "@/lib/library/mock";
import SlidesViewer from "@/components/library/slides-viewer";
import { LibraryLoading, LibraryNotFound, LibraryUnavailable } from "@/components/library/material-states";
import { useMaterialDetail } from "@/lib/library/use-material-detail";

/**
 * 幻灯片页容器:按 id 拉取详情(详情接口才带 slides 数据)。
 * 加载/错误状态与资料库其余页面一致。
 */
export default function SlidesPage({ id }: { id: string }) {
  const state = useMaterialDetail(id);
  if (state.loadState === "loading") return <LibraryLoading />;
  if (state.loadState === "not-found") return <LibraryNotFound />;
  if (state.loadState === "error") return <LibraryUnavailable message={state.error} onRetry={state.retry} />;
  const { material } = state;

  if (material.type !== "slides") {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
          NOT A SLIDES MATERIAL
        </p>
        <Link
          href={`/library/item/${id}`}
          className="mt-6 inline-block font-mono text-sm text-accent hover:underline"
        >
          ← 返回详情
        </Link>
      </main>
    );
  }

  return (
    <div>
      <p className="sr-only">
        {MATERIAL_TYPES[material.type].name} · {material.subject}
      </p>
      <SlidesViewer material={material} />
    </div>
  );
}
