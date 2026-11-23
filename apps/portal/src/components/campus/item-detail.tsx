"use client";

import { useState } from "react";
import { useReveal } from "@/components/account/use-reveal";
import { DetailStateBackLink } from "@/components/back-link";
import { ErrorBanner } from "@/components/data-state";
import { Dialog, DialogContent, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import Img from "@/components/ui/img";
import { useDocumentTitle } from "@/components/use-document-title";
import type { CampusItem, CampusMessage } from "@/lib/api/types";
import { categoryOf } from "@/lib/campus/mock";
import { useCampusItemDetail } from "@/lib/campus/use-campus-item-detail";
import { cn } from "@/lib/cn";
import { CampusExchangePanel, CampusMessageForm } from "./item-actions";

const STATUS_LABEL = { open: "待接单", ongoing: "进行中", done: "已完成", hidden: "已隐藏" } as const;

export function normalizeCampusPhotos(images?: string[]): string[] {
  if (!images?.length) return [];
  return images.filter((src): src is string => typeof src === "string" && src.trim().length > 0).slice(0, 6);
}

export default function ItemDetail({ id }: { id: string }) {
  const state = useCampusItemDetail(id);
  useReveal();
  useDocumentTitle(state.loadState === "ready" ? state.item.title : null, "campus");

  if (state.loadState !== "ready") {
    if (state.loadState === "not-found") {
      return (
        <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">404 / NOT FOUND</p>
          <p className="mt-4 font-display text-2xl font-bold">单子不存在或已下架</p>
          <DetailStateBackLink />
        </main>
      );
    }
    // 暂时读不到不等于单子不存在：说明原因并给重试，不显示 404。
    if (state.loadState === "error") {
      return (
        <main className="mx-auto max-w-3xl px-5 py-24 md:px-8">
          <ErrorBanner message={state.error} requestId={state.requestId} onRetry={state.retry} />
          <DetailStateBackLink />
        </main>
      );
    }
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs leading-6 text-ink/70">加载中…</p>
        <DetailStateBackLink />
      </main>
    );
  }

  return <CampusItemContent key={id} initialItem={state.item} initialMessages={state.messages} />;
}

export function CampusItemContent({
  initialItem,
  initialMessages,
}: {
  initialItem: CampusItem;
  initialMessages: CampusMessage[];
}) {
  const [status, setStatus] = useState(initialItem.status);
  const [newMessages, setNewMessages] = useState<CampusMessage[]>([]);
  const [photoIndex, setPhotoIndex] = useState(0);
  const item = { ...initialItem, status };
  const messages = [...initialMessages, ...newMessages];
  const cat = categoryOf(item.category);
  const photos = normalizeCampusPhotos(item.images);
  const activeIndex = photoIndex < photos.length ? photoIndex : 0;
  const activePhoto = photos[activeIndex];

  const showPrevious = () =>
    setPhotoIndex((current) => (current - 1 + photos.length) % photos.length);
  const showNext = () =>
    setPhotoIndex((current) => (current + 1) % photos.length);

  return (
    <main className="mx-auto max-w-site px-5 py-10 md:px-8">
      <div className="gap-10 lg:flex">
        {/* 左侧：单子图片、正文与留言 */}
        <div className="min-w-0 flex-1">
          <div
            data-enter
            className={cn(
              "relative w-full overflow-hidden border border-ink/25",
              photos.length
                ? "aspect-[4/3] max-h-[28rem] bg-paper-raised"
                : "bg-blueprint flex h-44 items-center justify-center"
            )}
          >
            {activePhoto ? (
              <Dialog>
                <DialogTrigger asChild>
                  <button
                    type="button"
                    aria-label={`查看大图：${item.title}，第 ${activeIndex + 1} 张，共 ${photos.length} 张`}
                    className="block h-full w-full cursor-zoom-in focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-focus-ring"
                  >
                    <Img
                      key={activePhoto}
                      src={activePhoto}
                      alt={`${item.title}，图片 ${activeIndex + 1} / ${photos.length}`}
                      label={cat.code}
                      loading="eager"
                      fetchPriority="high"
                      className="h-full w-full border-0 object-contain"
                    />
                  </button>
                </DialogTrigger>
                <DialogContent aria-describedby={undefined} className="w-[min(92vw,64rem)] p-5 sm:p-6">
                  <DialogTitle className="pr-10">
                    {item.title}
                    {photos.length > 1 ? `（${activeIndex + 1} / ${photos.length}）` : ""}
                  </DialogTitle>
                  <div className="mt-4 overflow-hidden border border-line bg-paper-raised">
                    <Img
                      key={`preview-${activePhoto}`}
                      src={activePhoto}
                      alt={`${item.title}，大图 ${activeIndex + 1} / ${photos.length}`}
                      label={cat.code}
                      loading="eager"
                      className="h-[65vh] w-full border-0 object-contain"
                    />
                  </div>
                  {photos.length > 1 && (
                    <div className="mt-4 flex items-center justify-between gap-3">
                      <button
                        type="button"
                        onClick={showPrevious}
                        className="inline-flex min-h-11 items-center border border-ink/30 bg-paper-raised px-4 font-mono text-xs text-ink transition-colors hover:border-ink focus-visible:outline-2 focus-visible:outline-focus-ring"
                      >
                        ← 上一张
                      </button>
                      <span className="font-mono text-xs tabular-nums text-ink/70">
                        {activeIndex + 1} / {photos.length}
                      </span>
                      <button
                        type="button"
                        onClick={showNext}
                        className="inline-flex min-h-11 items-center border border-ink/30 bg-paper-raised px-4 font-mono text-xs text-ink transition-colors hover:border-ink focus-visible:outline-2 focus-visible:outline-focus-ring"
                      >
                        下一张 →
                      </button>
                    </div>
                  )}
                </DialogContent>
              </Dialog>
            ) : (
              <span aria-hidden className="font-display text-6xl font-bold tracking-widest text-ink/20">
                {cat.code}
              </span>
            )}
            <span
              className={cn(
                "pointer-events-none absolute left-0 top-0 px-2 py-0.5 font-mono text-xs",
                item.type === "help" ? "bg-accent text-ink" : "bg-ink text-paper"
              )}
            >
              {item.type === "help" ? "求助单" : "闲置单"}
            </span>
            <span className="pointer-events-none absolute right-2 top-2 border border-ink/25 bg-paper/95 px-2 py-0.5 font-mono text-xs text-ink/75">
              {STATUS_LABEL[item.status]}
            </span>
            {photos.length > 1 && (
              <span className="pointer-events-none absolute bottom-2 right-2 border border-ink/25 bg-paper/95 px-2 py-0.5 font-mono text-xs tabular-nums text-ink">
                {activeIndex + 1} / {photos.length}
              </span>
            )}
          </div>

          {photos.length > 1 && (
            <div data-enter className="mt-3 space-y-2.5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="font-mono text-xs text-ink/65">
                  共 {photos.length} 张图片，点击主图可查看大图
                </p>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={showPrevious}
                    aria-label="查看上一张图片"
                    className="inline-flex min-h-11 items-center border border-ink/25 bg-paper-raised px-3 font-mono text-xs text-ink transition-colors hover:border-ink focus-visible:outline-2 focus-visible:outline-focus-ring"
                  >
                    ← 上一张
                  </button>
                  <button
                    type="button"
                    onClick={showNext}
                    aria-label="查看下一张图片"
                    className="inline-flex min-h-11 items-center border border-ink/25 bg-paper-raised px-3 font-mono text-xs text-ink transition-colors hover:border-ink focus-visible:outline-2 focus-visible:outline-focus-ring"
                  >
                    下一张 →
                  </button>
                </div>
              </div>
              <div
                role="group"
                aria-label="单子图片"
                className="grid grid-cols-3 gap-2 sm:grid-cols-6"
              >
                {photos.map((src, index) => (
                  <button
                    key={`${src}-${index}`}
                    type="button"
                    aria-label={`查看图片 ${index + 1}`}
                    aria-pressed={activeIndex === index}
                    onClick={() => setPhotoIndex(index)}
                    className={cn(
                      "block min-h-11 min-w-0 overflow-hidden border p-0.5 transition-colors focus-visible:outline-2 focus-visible:outline-focus-ring",
                      activeIndex === index
                        ? "border-ink bg-ink"
                        : "border-ink/20 bg-paper-raised hover:border-ink/60"
                    )}
                  >
                    <Img
                      src={src}
                      alt=""
                      label={String(index + 1).padStart(2, "0")}
                      className="aspect-[4/3] w-full border-0"
                    />
                  </button>
                ))}
              </div>
            </div>
          )}

          <h1 data-enter className="mt-6 font-display text-3xl font-bold leading-tight tracking-tight">
            {item.title}
          </h1>

          <p data-enter className="mt-4 whitespace-pre-line text-sm leading-7 text-ink/80">
            {item.desc}
          </p>

          <div data-enter className="mt-6 grid grid-cols-2 gap-3 border-y border-line py-4 font-mono text-xs text-ink/60 md:grid-cols-3">
            <p>分类 · {cat.name}</p>
            <p>位置 · {item.place}</p>
            {item.deadline && <p className="text-accent-text">时限 · {item.deadline}</p>}
            <p>发布于 · {item.time}</p>
          </div>

          {/* 留言区 */}
          <section data-enter className="mt-10">
            <p className="font-mono text-xs text-ink/60">
              <span className="tracking-[0.25em]">MESSAGES</span> / 留言 · {messages.length}
            </p>
            <ul className="mt-5 space-y-5">
              {messages.map((m) => (
                <li key={m.id} className="flex gap-3">
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center border border-ink/40 font-display text-xs font-bold">
                    {m.author.slice(0, 1)}
                  </span>
                  <div className="min-w-0">
                    <p className="font-mono text-xs text-ink/60">
                      {m.author}
                      <span className="mx-2">·</span>
                      {m.time}
                    </p>
                    <p className="mt-1 text-sm leading-6 text-ink/85">{m.text}</p>
                  </div>
                </li>
              ))}
            </ul>
            <div className="mt-6 border-t border-line pt-5">
              <CampusMessageForm id={item.id} onMessage={(message) => setNewMessages((current) => [...current, message])} />
            </div>
          </section>
        </div>

        {/* 右侧栏 */}
        <aside className="mt-10 w-full shrink-0 lg:mt-0 lg:w-80">
          <div className="lg:sticky lg:top-20 lg:space-y-5">
            {/* 赏金卡 */}
            <div data-enter className="border border-ink p-6">
              <p className="font-mono text-xs text-ink/60">
                <span className="tracking-[0.25em]">{item.type === "help" ? "BOUNTY" : "PRICE"}</span> /{" "}
                {item.type === "help" ? "赏金" : "一口价"}
              </p>
              <p className="mt-2 font-display text-5xl font-bold tabular-nums">
                <span className="text-accent-text">¥</span>
                {item.price}
              </p>
              <p
                data-campus-escrow-state="unavailable"
                className="mt-3 border border-dashed border-ink/30 px-2.5 py-1.5 font-mono text-xs text-ink/60"
              >
                金额仅供双方沟通参考，请自行约定结算方式。
              </p>
            </div>
            <CampusExchangePanel item={item} onStatus={setStatus} />

            {/* 卖家信用卡 */}
            <div data-enter className="border border-ink/25 p-5">
              <div className="flex items-center gap-3">
                <span className="bg-blueprint flex h-11 w-11 items-center justify-center border border-ink font-display text-lg font-bold">
                  {item.seller.slice(0, 1)}
                </span>
                <div>
                  <p className="text-sm font-medium">{item.seller}</p>
                  <p className="font-mono text-xs text-ink/60">
                    成交 {item.dealsDone} 单
                  </p>
                </div>
                <div className="ml-auto text-right">
                  <p className="font-display text-2xl font-bold text-accent-text">{item.credit}</p>
                  <p className="font-mono text-xs text-ink/60">信用分</p>
                </div>
              </div>
            </div>
          </div>
        </aside>
      </div>
    </main>
  );
}
