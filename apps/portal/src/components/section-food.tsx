"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";
import AmbientSvg from "@/components/ui/ambient-svg";
import { cn } from "@/lib/cn";

const RANKS = [
  { rank: "01", name: "老碗面 · 西门", tag: "夯", review: "十年不换配方，汤头是真熬出来的，期末周的续命水。" },
  { rank: "02", name: "鸡公煲 · 南门", tag: "夯", review: "微辣是谎言，点单请自觉降一档。分量够两个人。" },
  { rank: "03", name: "烤盘饭 · 食堂三楼", tag: "拉", review: "排队二十分钟，吃饭五分钟，肉量看阿姨心情。" },
  { rank: "04", name: "麻辣烫 · 东门", tag: "拉", review: "称重玄学发源地，同样的菜每次价格都不一样。" },
  { rank: "05", name: "手打柠檬茶 · 商业街", tag: "夯", review: "冰块给得比茶多是事实，但夏天还是得靠它。" },
];

function RankRow({ item }: { item: (typeof RANKS)[number] }) {
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
        <span className="font-display text-4xl font-bold text-ink/25 md:text-6xl">
          {item.rank}
        </span>
        <span className="flex-1 text-lg font-medium md:text-2xl">{item.name}</span>
        <span
          className={cn(
            "border px-2.5 py-1 font-mono text-xs",
            item.tag === "夯"
              ? "border-accent text-accent"
              : "border-ink/30 text-ink/50"
          )}
        >
          {item.tag}
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

export default function SectionFood() {
  const sectionRef = useRef<HTMLElement>(null);

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
    { scope: sectionRef }
  );

  return (
    <section ref={sectionRef} className="snap-screen border-t border-line bg-paper">
      <div className="mx-auto grid min-h-svh max-w-7xl items-center gap-12 px-5 py-24 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] md:px-10">
        <div>
          <SectionHeading index="03" en="FOOD RANKING" title="美食排行榜" />
          <p className="mt-6 font-display text-xl font-medium text-accent">
            「从夯到拉，只说人话。」
          </p>
          <p className="mt-4 max-w-sm text-sm leading-7 text-ink/70">
            全校学生真实打分，每周更新。不接受充值，不接受公关，
            难吃就是难吃。
          </p>
          <MagneticButton href="/food" className="mt-8">
            进入模块
          </MagneticButton>
          <AmbientSvg variant="flow" className="mt-12 hidden text-ink/30 md:block" />
        </div>

        <ul className="border-t border-line">
          {RANKS.map((item) => (
            <RankRow key={item.rank} item={item} />
          ))}
        </ul>
      </div>
    </section>
  );
}
