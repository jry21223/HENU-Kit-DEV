"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

/** 汉字、中文标点和全角符号。含中文的条目不拉开字距，只有拉丁条目拉开（DESIGN_SYSTEM.md 第 4 节）。 */
const CJK = /[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uff00-\uffef]/;

/**
 * 无缝循环 marquee 条带。
 */
export default function Marquee({
  items,
  dark = false,
  className,
}: {
  items: string[];
  dark?: boolean;
  className?: string;
}) {
  const trackRef = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        gsap.to(trackRef.current, {
          xPercent: -50,
          ease: "none",
          duration: 22,
          repeat: -1,
        });
      });
    },
    { scope: trackRef }
  );

  const row = (key: string) => (
    <div key={key} className="flex shrink-0 items-center">
      {items.map((item, i) => (
        <span key={`${key}-${i}`} className="flex items-center whitespace-nowrap">
          <span className={cn("px-6 font-mono text-sm", !CJK.test(item) && "tracking-[0.2em]")}>{item}</span>
          <span aria-hidden className={cn("text-xs", dark ? "text-paper/70" : "text-ink/80")}>
            +
          </span>
        </span>
      ))}
    </div>
  );

  return (
    <div
      className={cn(
        "overflow-hidden border-y py-3",
        // 橙底用墨色字（5.49:1）；纸白字在强调橙上只有 2.92:1。
        dark ? "border-line-dark bg-ink text-paper" : "border-ink/20 bg-accent text-ink",
        className
      )}
    >
      <div ref={trackRef} className="flex w-max will-change-transform">
        {row("a")}
        {row("b")}
      </div>
    </div>
  );
}
