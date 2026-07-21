"use client";

import { useSyncExternalStore } from "react";
import { accountStore, unreadNotices } from "@/lib/auth/mock";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function NotificationsPage() {
  const data = useSyncExternalStore(accountStore.subscribe, accountStore.get, accountStore.getServer);
  useReveal();
  const unread = unreadNotices(data);

  return (
    <div>
      <div data-enter className="flex items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">A-06</span>
            <span className="mx-2">/</span>
            NOTICES
          </p>
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">系统通知</h1>
        </div>
        <button
          type="button"
          onClick={() => accountStore.markAllNoticesRead()}
          disabled={unread === 0}
          className={cn(
            "border px-5 py-2.5 font-mono text-xs tracking-widest transition-colors",
            unread === 0
              ? "cursor-not-allowed border-line text-ink/30"
              : "border-ink hover:bg-ink hover:text-paper"
          )}
        >
          全部已读{unread > 0 ? `（${unread}）` : ""}
        </button>
      </div>

      <ul className="mt-8 border-t border-ink/40">
        {data.notices.map((n) => (
          <li key={n.id} data-enter>
            <button
              type="button"
              onClick={() => accountStore.markNoticeRead(n.id)}
              className="flex w-full gap-4 border-b border-line py-4 text-left transition-colors hover:bg-ink/[0.03]"
            >
              <span
                aria-hidden
                className={cn(
                  "mt-1.5 h-2 w-2 shrink-0",
                  n.read ? "border border-line" : "bg-accent"
                )}
              />
              <span className="min-w-0 flex-1">
                <span className={cn("block truncate text-sm", n.read ? "text-ink/50" : "font-medium")}>
                  {n.title}
                </span>
                <span className={cn("mt-1 block text-xs leading-6", n.read ? "text-ink/40" : "text-ink/65")}>
                  {n.body}
                </span>
              </span>
              <span className="shrink-0 font-mono text-[10px] text-ink/40">{n.time}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
