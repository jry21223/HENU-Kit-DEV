"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { accountStore, unreadNotices, openTickets } from "@/lib/auth/mock";
import { useReveal } from "@/components/account/use-reveal";

export default function AccountOverviewPage() {
  const { user } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  const data = useSyncExternalStore(accountStore.subscribe, accountStore.get, accountStore.getServer);
  useReveal();

  if (!user) return null;

  const cards = [
    { label: "积分余额", value: String(data.balance), href: "/account/wallet", mono: "C-01" },
    { label: "会员到期", value: `${data.membership.daysLeft} 天`, href: "/account/membership", mono: "C-02" },
    { label: "未读通知", value: String(unreadNotices(data)), href: "/account/notifications", mono: "C-03" },
    { label: "进行中工单", value: String(openTickets(data)), href: "/account/tickets", mono: "C-04" },
  ];

  return (
    <div>
      {/* 账号卡 */}
      <div data-enter className="flex items-center gap-5 border border-ink p-6">
        <span className="bg-blueprint flex h-16 w-16 shrink-0 items-center justify-center border border-ink font-display text-3xl font-bold">
          {user.name.slice(0, 1)}
        </span>
        <div className="min-w-0">
          <p className="truncate font-display text-2xl font-bold">{user.name}</p>
          <p className="mt-1 font-mono text-[10px] tracking-[0.25em] text-ink/50">
            UID {user.uid}
            {user.email ? ` · ${user.email}` : ""}
          </p>
        </div>
        <span className="ml-auto shrink-0 border border-accent px-2 py-1 font-mono text-[10px] tracking-widest text-accent">
          {data.membership.plan}
        </span>
      </div>

      {/* 摘要卡 */}
      <div className="mt-6 grid grid-cols-2 gap-4 md:grid-cols-4">
        {cards.map((c) => (
          <Link
            key={c.mono}
            href={c.href}
            data-enter
            className="group border border-ink/25 p-5 transition-colors hover:border-ink"
          >
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
              {c.mono} / {c.label}
            </p>
            <p className="mt-3 font-display text-3xl font-bold group-hover:text-accent">
              {c.value}
            </p>
            <p className="mt-2 font-mono text-[10px] text-ink/40 transition-colors group-hover:text-accent">
              查看 →
            </p>
          </Link>
        ))}
      </div>

      <p data-enter className="mt-8 font-mono text-[10px] tracking-[0.3em] text-ink/40">
        v1 预览 · 数据为会话内示例，刷新后重置
      </p>
    </div>
  );
}
