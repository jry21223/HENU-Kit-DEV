"use client";

import { useState } from "react";
import type { NoticeFeedItem } from "@/lib/api/types";

function formatNoticeTime(value: string | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/**
 * 已发布通知列表。通知服务没有独立详情端点，正文随快照返回，
 * 因此详情在列表内就地展开，内容全部来自服务端真实数据。
 */
export default function NoticeFeed({ items }: { items: NoticeFeedItem[] }) {
  const [openId, setOpenId] = useState<string | null>(null);

  return (
    <ol className="border-t border-ink">
      {items.map((item, index) => {
        const open = openId === item.id;
        const publishedAt = formatNoticeTime(item.source_published_at ?? item.created_at);
        return (
          <li key={item.id} data-enter className="border-b border-line">
            <button
              type="button"
              aria-expanded={open}
              onClick={() => setOpenId(open ? null : item.id)}
              className="group grid w-full grid-cols-[3rem_minmax(0,1fr)_auto] items-center gap-3 py-5 text-left md:grid-cols-[3rem_minmax(0,1fr)_14rem_auto] md:gap-5"
            >
              <span className="font-mono text-sm text-ink/30">
                {String(index + 1).padStart(2, "0")}
              </span>
              <span className="min-w-0">
                <span className="block font-display text-xl font-bold transition-colors group-hover:text-accent">
                  {item.title}
                </span>
                <span className="mt-1 block font-mono text-[10px] tracking-[0.12em] text-ink/40">
                  {item.source.name} · 第 {item.version} 版
                </span>
              </span>
              <span className="hidden font-mono text-[10px] tracking-[0.12em] text-ink/40 md:block">
                {publishedAt}
              </span>
              <span
                aria-hidden
                className="justify-self-end font-mono text-xs text-ink/50 transition-transform group-hover:translate-x-1"
              >
                {open ? "收起 ↑" : "展开 ↓"}
              </span>
            </button>
            {open && (
              <div className="grid gap-4 pb-6 pl-[3rem] pr-4 md:pl-[3rem] md:pr-6">
                <p className="whitespace-pre-wrap leading-7 text-ink/75">{item.body}</p>
                <dl className="border-t border-dashed border-line pt-3 font-mono text-[10px] leading-5 text-ink/45">
                  <div className="flex flex-wrap gap-x-6 gap-y-1">
                    <span>来源 / {item.source.name}</span>
                    <span>发布时间 / {publishedAt}</span>
                    <span>版本 / v{item.version}</span>
                    {item.source_url && (
                      <a
                        href={item.source_url}
                        target="_blank"
                        rel="noreferrer"
                        className="text-accent hover:underline"
                      >
                        原文链接 ↗
                      </a>
                    )}
                  </div>
                </dl>
              </div>
            )}
          </li>
        );
      })}
    </ol>
  );
}
