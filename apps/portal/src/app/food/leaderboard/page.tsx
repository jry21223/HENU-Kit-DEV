"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useSyncExternalStore } from "react";
import { CAMPUSES, CAMPUS_KEYS, CampusKey, foodStore } from "@/lib/food/mock";
import { hasGateway } from "@/lib/api/client";
import { getGatewayPosts } from "@/lib/food/gateway";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function FoodLeaderboardPage() {
  const data = useSyncExternalStore(foodStore.subscribe, foodStore.get, foodStore.getServer);
  const [posts, setPosts] = useState(data.posts);
  const [campus, setCampus] = useState<CampusKey | "all">("all");
  useReveal([campus]);

  useEffect(() => {
    if (!hasGateway) return;
    const gw = getGatewayPosts();
    if (gw) setPosts(gw as typeof data.posts);
  }, []);

  const rows = posts
    .filter((p) => !p.hidden && (campus === "all" || p.campus === campus))
    .slice()
    .sort((a, b) => b.likes - a.likes);

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div data-enter className="flex flex-wrap items-end justify-between gap-6">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">RANK</span>
            <span className="mx-2">/</span>
            MUST-EAT
          </p>
          <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">
            必吃美食
          </h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => setCampus("all")}
            className={cn(
              "border px-3 py-1.5 font-mono text-xs transition-colors",
              campus === "all" ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
            )}
          >
            全部
          </button>
          {CAMPUS_KEYS.map((k) => (
            <button
              key={k}
              type="button"
              onClick={() => setCampus(k)}
              className={cn(
                "border px-3 py-1.5 font-mono text-xs transition-colors",
                campus === k ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
              )}
            >
              {CAMPUSES[k].name}
            </button>
          ))}
        </div>
      </div>

      <div className="mt-10 border-t border-ink/40">
        {rows.map((p, i) => (
          <Link
            key={p.id}
            href={`/food/post/${p.id}`}
            data-enter
            className="group flex items-baseline gap-5 border-b border-line py-4 md:gap-8"
          >
            <span
              className={cn(
                "font-display text-3xl font-bold md:text-4xl",
                i < 3 ? "text-accent" : "text-ink/25"
              )}
            >
              {String(i + 1).padStart(2, "0")}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-base font-medium transition-colors group-hover:text-accent md:text-lg">
                {p.title}
              </span>
              <span className="mt-1 block font-mono text-[10px] tracking-wider text-ink/50">
                {CAMPUSES[p.campus].name} · {p.tags.join(" / ")} · {p.shop.name}
              </span>
            </span>
            <span className="shrink-0 font-mono text-sm">
              ▲ <span className={i < 3 ? "text-accent" : ""}>{p.likes}</span>
            </span>
          </Link>
        ))}
      </div>
      <p className="mt-6 font-mono text-[10px] tracking-[0.3em] text-ink/40">
        按点赞数实时排序 / 隐藏文章不计入
      </p>
    </main>
  );
}
