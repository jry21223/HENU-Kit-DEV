"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { fetchLibraryMaterialDetail } from "@/lib/api/client";
import { MATERIAL_TYPES, type Material } from "@/lib/library/mock";
import SlidesViewer from "@/components/library/slides-viewer";

/**
 * 幻灯片页容器:按 id 拉取详情(详情接口才带 slides 数据)。
 * 加载/错误状态与资料库其余页面一致。
 */
export default function SlidesPage({ id }: { id: string }) {
  const [material, setMaterial] = useState<Material | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    fetchLibraryMaterialDetail(id)
      .then((response) => {
        if (cancelled) return;
        if (response?.material) {
          setMaterial(response.material);
        } else {
          setError("内容不存在或已下架");
        }
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : "加载幻灯片失败");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (loading) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">LOADING / 加载中</p>
      </main>
    );
  }

  if (error || !material) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">404 / NOT FOUND</p>
        <p className="mt-3 text-sm text-ink/70">{error ?? "内容不存在"}</p>
        <Link
          href="/library"
          className="mt-6 inline-block font-mono text-sm text-accent hover:underline"
        >
          ← 返回书库
        </Link>
      </main>
    );
  }

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
