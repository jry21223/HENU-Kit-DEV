"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { morphStore, MorphPayload } from "./transition-store";

/** 通用过渡的橙色标尺线扫过（无共享元素场景） */
function rulerSweep() {
  const bar = document.createElement("div");
  bar.style.cssText =
    "position:fixed;left:0;right:0;top:0;height:2px;background:#ff4d00;z-index:80;transform:scaleX(0);transform-origin:left center;pointer-events:none";
  document.body.appendChild(bar);
  gsap.to(bar, {
    scaleX: 1,
    duration: 0.45,
    ease: "power2.inOut",
    onComplete: () => {
      gsap.to(bar, {
        autoAlpha: 0,
        duration: 0.2,
        delay: 0.1,
        onComplete: () => bar.remove(),
      });
    },
  });
}

/**
 * 延迟导航链接：先播当前页"形变收起"（内容块塌缩成结构线），
 * 中后段再 router.push；reduced-motion 时瞬时导航。
 * morph 提供时走共享元素主线（源矩形取链接自身）。
 */
export default function TransitionLink({
  href,
  morph,
  className,
  children,
}: {
  href: string;
  morph?: Omit<MorphPayload, "rect">;
  className?: string;
  children: React.ReactNode;
}) {
  const router = useRouter();

  const onClick = (e: React.MouseEvent<HTMLAnchorElement>) => {
    if (
      e.defaultPrevented ||
      e.metaKey ||
      e.ctrlKey ||
      e.shiftKey ||
      e.altKey ||
      e.button !== 0
    )
      return;
    e.preventDefault();

    if (window.matchMedia(REDUCED_MOTION).matches) {
      router.push(href);
      return;
    }

    const page = document.querySelector<HTMLElement>("[data-transition-page]");
    const blocks = page?.querySelectorAll("[data-block]");

    if (morph) {
      const r = e.currentTarget.getBoundingClientRect();
      gsap.set(e.currentTarget, { autoAlpha: 0 });
      morphStore.set({
        ...morph,
        rect: { x: r.x, y: r.y, w: r.width, h: r.height },
      });
    } else {
      rulerSweep();
    }

    const finish = () => router.push(href);
    if (blocks && blocks.length) {
      gsap.to(blocks, {
        scaleY: 0.04,
        transformOrigin: "top center",
        autoAlpha: 0.25,
        duration: 0.3,
        ease: "power2.in",
        stagger: 0.045,
        onComplete: finish,
      });
    } else if (page) {
      gsap.to(page, {
        autoAlpha: 0,
        y: -10,
        duration: 0.28,
        ease: "power2.in",
        onComplete: finish,
      });
    } else {
      finish();
    }
  };

  return (
    <Link href={href} onClick={onClick} className={className}>
      {children}
    </Link>
  );
}
