"use client";

import Link from "next/link";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { useReveal } from "@/components/account/use-reveal";

export default function DealsPage() {
  const router = useRouter();
  const { user, ready } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  useReveal();

  useEffect(() => {
    if (ready && !user) router.replace("/account/login?next=/campus/deals");
  }, [ready, user, router]);

  if (!ready || !user) {
    return (
      <main className="flex min-h-[60vh] items-center justify-center">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
          AUTH CHECK<span className="animate-pulse text-accent">…</span>
        </p>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">M-02</span>
        <span className="mx-2">/</span>
        MY DEALS
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">我的交易</h1>

      <div data-enter className="mt-6 border border-dashed border-ink/30 px-6 py-16 text-center">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/50">
          订单管理尚未接通 · COMING SOON
        </p>
        <p className="mx-auto mt-3 max-w-sm text-sm leading-7 text-ink/70">
          互助平台的发布、接单与结算接口正在接入服务端，上线后这里会展示你发布的单子和参与的交易。
        </p>
        <Link
          href="/campus"
          className="mt-6 inline-block border border-ink px-6 py-2.5 font-mono text-xs tracking-widest transition-colors hover:border-accent hover:text-accent"
        >
          ← 返回市集
        </Link>
      </div>
    </main>
  );
}
