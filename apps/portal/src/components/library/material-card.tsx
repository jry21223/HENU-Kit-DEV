import Link from "next/link";
import type { Material } from "@/lib/library/mock";
import { MATERIAL_TYPES } from "@/lib/library/material-types";
import { readableMaterialTitle } from "@/lib/library/material-title";

/**
 * 资料卡：封面块（图纸网格 + 类型代号）+ 元信息行。公开目录只收免费资料（契约 price 恒为 0），
 * 所以不逐张标“免费”。
 */
export default function MaterialCard({ material }: { material: Material }) {
  const t = MATERIAL_TYPES[material.type];

  return (
    <Link href={`/library/item/${material.id}`} className="group block min-w-0 border border-ink/25 bg-paper transition-colors hover:border-ink focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-accent">
      {/* 封面 */}
      <div className="bg-blueprint relative flex h-36 flex-col justify-between border-b border-line p-3">
        {/* 类型代号只作装饰：卡片下方写着中文类型名。 */}
        <span aria-hidden className="font-mono text-[10px] tracking-[0.3em] text-ink/40">{t.code}</span>
        <div className="min-w-0">
          <p className="mb-1 break-words font-mono text-xs text-ink/60">{material.subject}</p>
          <h2 className="line-clamp-2 break-words font-display text-lg font-bold leading-snug">
            {readableMaterialTitle(material)}
          </h2>
        </div>
      </div>

      <div className="p-4">
        <p className="font-mono text-xs text-ink/60">
          {t.name}
        </p>
        <div className="mt-3 flex items-center gap-3 border-t border-line pt-3 font-mono text-xs text-ink/60">
          <span className="truncate">{material.author}</span>
          {material.rating !== undefined && (
            <span className="ml-auto shrink-0">★ {material.rating.toFixed(1)}</span>
          )}
          <span className="shrink-0">↓ {material.downloads}</span>
        </div>
      </div>
    </Link>
  );
}
