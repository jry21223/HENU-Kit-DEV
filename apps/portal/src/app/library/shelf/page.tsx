"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { STATIC_MATERIALS, libraryStore } from "@/lib/library/mock";
import { hasGateway } from "@/lib/api/client";
import { getMaterials } from "@/lib/library/gateway";
import MaterialCard from "@/components/library/material-card";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function ShelfPage() {
  const router = useRouter();
  const { user, ready } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  const lib = useSyncExternalStore(libraryStore.subscribe, libraryStore.get, libraryStore.getServer);
  const [materials, setMaterials] = useState(STATIC_MATERIALS);
  const [tab, setTab] = useState<"owned" | "fav">("owned");
  useReveal();

  useEffect(() => {
    if (!hasGateway) return;
    setMaterials(getMaterials() as typeof STATIC_MATERIALS);
  }, []);

  useEffect(() => {
    if (ready && !user) router.replace("/account/login?next=/library/shelf");
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

  const owned = materials.filter((m) => lib.owned.includes(m.id));
  const favs = materials.filter((m) => lib.favs.includes(m.id));
  const list = tab === "owned" ? owned : favs;

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">L-02</span>
        <span className="mx-2">/</span>
        MY SHELF
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">我的书架</h1>

      <div data-enter className="mt-6 flex gap-2">
        <button
          type="button"
          onClick={() => setTab("owned")}
          className={cn(
            "border px-4 py-2 font-mono text-xs tracking-widest transition-colors",
            tab === "owned" ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
          )}
        >
          已购（{owned.length}）
        </button>
        <button
          type="button"
          onClick={() => setTab("fav")}
          className={cn(
            "border px-4 py-2 font-mono text-xs tracking-widest transition-colors",
            tab === "fav" ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
          )}
        >
          收藏（{favs.length}）
        </button>
      </div>

      <div data-enter className="mt-8">
        {list.length === 0 ? (
          <p className="border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
            {tab === "owned" ? "还没有购买过资料 / 去书库逛逛" : "还没有收藏 / EMPTY"}
          </p>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
            {list.map((m) => (
              <MaterialCard key={m.id} material={m} />
            ))}
          </div>
        )}
      </div>
    </main>
  );
}
