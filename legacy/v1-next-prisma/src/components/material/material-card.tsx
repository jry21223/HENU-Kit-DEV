import Link from "next/link";
import { materialTypeLabels } from "@/constants/enums";
import type { Material } from "@/types";
import { AccessBadge } from "@/components/material/access-badge";

type MaterialCardProps = {
  material: Material;
};

export function MaterialCard({ material }: MaterialCardProps) {
  return (
    <article className="rounded-lg border border-line bg-white p-5 shadow-soft">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="rounded-md bg-panel px-2.5 py-1 text-xs font-semibold text-muted">
              {materialTypeLabels[material.type]}
            </span>
            <AccessBadge accessLevel={material.accessLevel} />
          </div>
          <h3 className="text-base font-semibold text-ink">{material.title}</h3>
          <p className="mt-2 text-sm leading-6 text-muted">{material.description}</p>
        </div>
        <Link
          href={`/materials/${material.id}`}
          className="inline-flex h-10 shrink-0 items-center justify-center rounded-md border border-line px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
        >
          查看详情
        </Link>
      </div>
      <div className="mt-4 grid gap-2 border-t border-line pt-4 text-xs text-muted sm:grid-cols-3">
        <span>文件：{material.fileName}</span>
        <span>大小：{material.fileSize}</span>
        <span>更新：{material.updatedAt}</span>
      </div>
    </article>
  );
}

