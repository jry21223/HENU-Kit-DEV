"use client";

import Link from "next/link";
import { useMemo, useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";
import AmbientSvg from "@/components/ui/ambient-svg";
import { cn } from "@/lib/cn";
import type { FoodPost } from "@/lib/api/types";
import { HANG_TIER_KEY, groupFoodPostsByTier, type FoodTier } from "@/lib/food/ranking";
import { useFoodPosts } from "@/lib/food/use-food-posts";

interface RankRowItem {
  rank: string;
  name: string;
  tier: FoodTier;
  review: string;
  href: string;
}

function RankRow({ item }: { item: RankRowItem }) {
  const reviewRef = useRef<HTMLDivElement>(null);

  // 卸载时清理未完成的补间
  useGSAP(
    () => () => {
      if (reviewRef.current) gsap.killTweensOf(reviewRef.current);
    },
    { scope: reviewRef }
  );

  const open = () => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    gsap.to(reviewRef.current, { height: "auto", duration: 0.35, ease: "power2.out" });
  };
  const close = () => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    gsap.to(reviewRef.current, { height: 0, duration: 0.3, ease: "power2.in" });
  };

  return (
    <li
      data-rank-row
      onMouseEnter={open}
      onMouseLeave={close}
      className="border-b border-line"
    >
      <div className="flex items-baseline gap-5 py-5 md:gap-10">
        {/* 编号列定宽、等宽数字，01–05 下店名左缘对齐；宽度与下方锐评的缩进一致。 */}
        <span className="w-[3.25rem] shrink-0 font-display text-4xl font-bold tabular-nums text-ink/25 md:w-20 md:text-6xl">
          {item.rank}
        </span>
        {/* 店名一行只有 28px 高：上下各借 8px 撑满 44px 点击区，行距不变。 */}
        <Link
          href={item.href}
          className="-my-2 flex-1 py-2 text-lg font-medium transition-colors hover:text-accent md:text-2xl"
        >
          {item.name}
        </Link>
        <span
          className={cn(
            "border px-2.5 py-1 font-mono text-xs",
            item.tier.key === HANG_TIER_KEY
              ? "border-accent text-accent"
              : "border-ink/30 text-ink/50"
          )}
        >
          {item.tier.label}
        </span>
      </div>
      <div ref={reviewRef} className="h-0 overflow-hidden">
        <p className="pb-5 pl-[4.5rem] text-sm text-ink/60 md:pl-[7.5rem]">
          学长锐评：{item.review}
        </p>
      </div>
    </li>
  );
}

function toRankRows(posts: FoodPost[]): RankRowItem[] {
  // groupFoodPostsByTier 保证档位按 FOOD_TIERS 顺序，并保留 owner 的新到旧顺序。
  // 注意：这是档位优先的局部 TOP 5，序号是全局序——首档条数 ≥5 时
  // 会出现 01-05 全为同一档的情况，属预期行为。
  return groupFoodPostsByTier(posts)
    .flatMap(({ tier, posts: tierPosts }) =>
      tierPosts.map((post) => ({ tier, post }))
    )
    .slice(0, 5)
    .map(({ tier, post }, index) => ({
      rank: String(index + 1).padStart(2, "0"),
      name: post.shop.name,
      tier,
      review: post.excerpt || post.title,
      href: `/food/post/${post.id}`,
    }));
}

export default function SectionFood() {
  const sectionRef = useRef<HTMLElement>(null);
  const { posts, loadState, error, load } = useFoodPosts();

  const rows = useMemo(() => toRankRows(posts), [posts]);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        gsap.from(gsap.utils.toArray("[data-rank-row]"), {
          x: -60,
          opacity: 0,
          duration: 0.7,
          ease: "power3.out",
          stagger: 0.12,
          scrollTrigger: {
            trigger: sectionRef.current,
            start: "top 60%",
            toggleActions: "play none none reverse",
          },
        });
      });
      return () => mm.revert();
    },
    { scope: sectionRef, dependencies: [loadState] }
  );

  return (
    <section ref={sectionRef} className="snap-screen border-t border-line bg-paper">
      <div className="mx-auto grid max-w-site items-center gap-12 px-5 py-24 md:min-h-svh md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] md:px-8">
        <div>
          <SectionHeading index="03" en="FOOD RANKING" title="美食排行榜" />
          <p className="mt-6 font-display text-xl font-medium text-accent">
            「从夯到拉，只说人话。」
          </p>
          <p className="mt-4 max-w-sm text-sm leading-7 text-ink/70">
            学生视角分档，档内按最新投稿展示；不接受充值，不接受公关，
            难吃就是难吃。
          </p>
          <MagneticButton href="/food" className="mt-8">
            进入模块
          </MagneticButton>
          <AmbientSvg variant="flow" className="mt-12 hidden text-ink/30 md:block" />
        </div>

        <ul className="border-t border-line">
          {loadState === "loading" && (
            <li className="border-b border-line py-5 font-mono text-xs tracking-[0.18em] text-ink/45">
              榜单加载中…
            </li>
          )}
          {loadState === "error" && (
            <li className="border-b border-line py-5">
              <p className="font-mono text-xs tracking-[0.18em] text-ink/60">
                榜单暂时加载不出来，请稍后刷新试试。
              </p>
              {error ? (
                <button
                  type="button"
                  onClick={() => void load()}
                  className="inline-flex min-h-11 items-center font-mono text-xs text-accent underline underline-offset-4"
                >
                  重新加载
                </button>
              ) : null}
            </li>
          )}
          {loadState === "ready" && rows.length === 0 && (
            <li className="border-b border-line py-5 font-mono text-xs tracking-[0.18em] text-ink/45">
              还没有上榜条目。
            </li>
          )}
          {loadState === "ready" &&
            rows.map((item) => <RankRow key={item.rank} item={item} />)}
        </ul>
      </div>
    </section>
  );
}
