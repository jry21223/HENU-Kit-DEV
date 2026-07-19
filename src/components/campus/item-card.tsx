import Link from "next/link";
import { categoryOf, Item } from "@/lib/campus/mock";
import Img from "@/components/ui/img";
import { cn } from "@/lib/cn";

const STATUS_LABEL: Record<Item["status"], string | null> = {
  open: null,
  ongoing: "进行中",
  done: "已完成",
  hidden: "已隐藏",
};

/** 闲鱼式单子卡：编号图块（分类代号大字 + 类型角标）+ 标题 + 赏金 + 发布者行 */
export default function ItemCard({ item }: { item: Item }) {
  const cat = categoryOf(item.category);
  const statusLabel = STATUS_LABEL[item.status];

  return (
    <Link
      href={`/campus/item/${item.id}`}
      className={cn(
        "group mb-4 block break-inside-avoid border bg-paper transition-colors",
        item.type === "help"
          ? "border-accent/50 hover:border-accent"
          : "border-ink/25 hover:border-ink"
      )}
    >
      {/* 编号图块 / 缩略图 */}
      <div
        className={cn(
          "relative flex h-28 items-center justify-center border-b",
          item.type === "help" ? "border-accent/50" : "border-line"
        )}
      >
        {item.images?.[0] ? (
          <Img src={item.images[0]} alt={item.title} label={cat.code} className="h-full w-full border-0" />
        ) : (
          <div className="bg-blueprint absolute inset-0 flex items-center justify-center">
            <span className="font-display text-4xl font-bold tracking-widest text-ink/25">
              {cat.code}
            </span>
          </div>
        )}
        <span
          className={cn(
            "absolute left-0 top-0 px-1.5 py-0.5 font-mono text-[10px] text-paper",
            item.type === "help" ? "bg-accent" : "bg-ink"
          )}
        >
          {item.type === "help" ? "求助" : "闲置"}
        </span>
        {statusLabel && (
          <span className="absolute right-2 top-2 border border-ink/40 bg-paper px-1.5 py-0.5 font-mono text-[10px] text-ink/60">
            {statusLabel}
          </span>
        )}
        <span aria-hidden className="absolute bottom-2 right-2 font-mono text-[10px] text-ink/40">+</span>
      </div>

      <div className="p-4">
        <h3 className="font-medium leading-snug transition-colors group-hover:text-accent">
          {item.title}
        </h3>
        <p className="mt-1.5 line-clamp-2 text-xs leading-5 text-ink/60">{item.desc}</p>

        <p className="mt-3 font-display text-2xl font-bold tabular-nums">
          <span className="text-accent">¥</span>
          {item.price}
          <span className="ml-1.5 font-mono text-[10px] font-normal text-ink/50">
            {item.type === "help" ? "赏金" : "一口价"}
          </span>
        </p>

        <div className="mt-3 flex items-center gap-2 border-t border-line pt-3 font-mono text-[10px] text-ink/50">
          <span className="flex h-5 w-5 items-center justify-center border border-ink/40 font-display text-[10px] font-bold">
            {item.seller.slice(0, 1)}
          </span>
          <span className="truncate">{item.seller}</span>
          <span className="ml-auto shrink-0">想要 {item.wants}</span>
          <span className="shrink-0 text-ink/30">·</span>
          <span className="shrink-0">{item.place.split(" ·")[0]}</span>
        </div>
      </div>
    </Link>
  );
}
