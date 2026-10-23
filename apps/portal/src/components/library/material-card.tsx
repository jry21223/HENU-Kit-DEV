import Link from "next/link";
import type { Material } from "@/lib/library/mock";
import { MATERIAL_TYPES } from "@/lib/library/material-types";
import { readableMaterialTitle } from "@/lib/library/material-title";

/**
 * 资料卡：封面块（图纸网格 + 类型名）+ 元信息行。公开目录只收免费资料（契约 price 恒为 0），
 * 所以不逐张标“免费”。
 */
export default function MaterialCard({ material }: { material: Material }) {
  const t = MATERIAL_TYPES[material.type];

  return (
    <Link href={`/library/item/${material.id}`} className="group block min-w-0 border border-ink/25 bg-paper transition-colors hover:border-ink focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus-ring">
      {/* 封面 */}
      <div className="bg-blueprint relative flex flex-col gap-2 p-3 md:h-36 md:justify-between md:border-b md:border-line">
        {/* 类型只写中文名，与资料库的类型筛选同一叫法。 */}
        <div className="flex min-w-0 gap-3 font-mono text-xs text-ink/60">
          <span className="shrink-0">{t.name}</span>
          <span className="truncate">{material.subject}</span>
        </div>
        <div className="min-w-0">
          <h2 className="line-clamp-2 break-words font-display text-base font-bold leading-snug md:text-lg">
            {readableMaterialTitle(material)}
          </h2>
        </div>
      </div>

      <div className="px-3 pb-3 md:p-4">
        <div className="flex items-center gap-3 border-t border-line pt-2 font-mono text-xs text-ink/60 md:pt-3">
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
