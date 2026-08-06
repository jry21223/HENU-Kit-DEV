import Link from "next/link";
import { MATERIAL_TYPES, Material } from "@/lib/library/mock";

/** Human-readable file size; the catalogue records bytes. */
function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
}

/** Uppercase extension, used as the format badge. */
function formatOf(filePath: string): string {
  const dot = filePath.lastIndexOf(".");
  return dot === -1 ? "" : filePath.slice(dot + 1).toUpperCase();
}

/**
 * 资料卡：封面块（图纸网格 + 类型代号 + 价格/免费签）+ 元信息行
 *
 * Mirrored materials record no rating, download count or favourites, so the
 * meta row shows the file facts that do exist — format and size — rather than
 * a row of zeroes. The contributor is credited when the catalogue knows one.
 */
export default function MaterialCard({ material }: { material: Material }) {
  const t = MATERIAL_TYPES[material.type];
  const free = material.price === 0;
  const hasFile = Boolean(material.filePath);

  return (
    <Link
      href={`/library/item/${material.id}`}
      className="group block border border-ink/25 bg-paper transition-colors hover:border-ink"
    >
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
        <p className="font-display text-lg font-bold leading-snug line-clamp-2">
          {material.title}
        </p>
      </div>

      <div className="p-4">
        <p className="font-mono text-[11px] text-ink/60">
          {t.name} · {material.subject}
        </p>
        <div className="mt-3 flex items-center gap-3 border-t border-line pt-3 font-mono text-[10px] text-ink/50">
          {material.author && <span className="truncate">{material.author}</span>}
          {hasFile ? (
            <>
              <span className="ml-auto shrink-0">{formatOf(material.filePath!)}</span>
              {material.fileSize ? (
                <span className="shrink-0">{formatSize(material.fileSize)}</span>
              ) : null}
            </>
          ) : (
            <>
              {material.rating > 0 && (
                <span className="ml-auto shrink-0">★ {material.rating.toFixed(1)}</span>
              )}
              {material.downloads > 0 && <span className="shrink-0">↓ {material.downloads}</span>}
            </>
          )}
        </div>
        {!free && material.previewPages > 0 && (
          <span className="mt-3 inline-block border border-accent/60 px-1.5 py-0.5 font-mono text-[10px] text-accent">
            可试读 {material.previewPages} 页
          </span>
        )}
      </div>
    </Link>
  );
}
