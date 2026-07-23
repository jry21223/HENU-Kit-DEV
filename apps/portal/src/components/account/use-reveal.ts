"use client";

import { useEffect } from "react";
import { gsap, FINE_MOTION } from "@/lib/gsap";

/** 账号站页面入场 reveal：淡入 + y 位移 stagger（reduced-motion 直显） */
export function useReveal(deps: unknown[] = []) {
  useEffect(() => {
    const mm = gsap.matchMedia();
    mm.add(FINE_MOTION, () => {
      gsap.from("[data-enter]", {
        y: 14,
        autoAlpha: 0,
        duration: 0.45,
        ease: "power2.out",
        stagger: 0.05,
        clearProps: "all",
      });
    });
    return () => mm.revert();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
}
