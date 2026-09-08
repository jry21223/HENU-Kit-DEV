import Link from "next/link";
import type { Material } from "@/lib/library/mock";
import { MATERIAL_TYPES } from "@/lib/library/material-types";
import { readableMaterialTitle } from "@/lib/library/material-title";

/** 资料卡：封面块（图纸网格 + 类型代号 + 价格/免费签）+ 元信息行 */
export default function MaterialCard({ material }: { material: Material }) {
  const t = MATERIAL_TYPES[material.type];
  const free = material.price === 0;

  return (
    <Link href={`/library/item/${material.id}`} className="group block min-w-0 border border-ink/25 bg-paper transition-colors hover:border-ink focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-accent">
      {/* 封面 */}
      <div className="bg-blueprint relative flex h-36 flex-col justify-between border-b border-line p-3">
        <div className="flex items-start justify-between">
          <span className="font-mono text-[10px] tracking-[0.3em] text-ink/40">{t.code}</span>
          {free ? (
            <span className="bg-ink px-1.5 py-0.5 font-mono text-[10px] text-paper">免费</span>
          ) : (
            <span className="bg-accent px-1.5 py-0.5 font-mono text-[10px] text-paper">
              {material.price} 积分
            </span>
          )}
        </div>
        <div className="min-w-0">
          <p className="mb-1 break-words font-mono text-[11px] text-ink/60">{material.subject}</p>
          <h2 className="line-clamp-2 break-words font-display text-lg font-bold leading-snug">
            {readableMaterialTitle(material)}
          </h2>
        </div>
      </div>

      <div className="p-4">
        <p className="font-mono text-[11px] text-ink/60">
          {t.name}
        </p>
        <div className="mt-3 flex items-center gap-3 border-t border-line pt-3 font-mono text-[10px] text-ink/50">
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
