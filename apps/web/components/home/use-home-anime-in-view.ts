"use client";

import { animate, stagger } from "animejs";
import { type RefObject, useEffect } from "react";

export function useHomeAnimeInView({
  reduceMotion,
  rootRef,
  selector,
}: {
  reduceMotion: boolean;
  rootRef: RefObject<HTMLElement | null>;
  selector: string;
}) {
  useEffect(() => {
    const root = rootRef.current;

    if (!root || reduceMotion) {
      return;
    }

    const targets = root.querySelectorAll(selector);

    if (targets.length === 0) {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) {
            continue;
          }

          animate(targets, {
            opacity: [0, 1],
            translateY: [18, 0],
            rotate: [-2, 0],
            scale: [0.96, 1],
            delay: stagger(70),
            duration: 560,
            ease: "outCubic",
          });

          observer.disconnect();
        }
      },
      { rootMargin: "0px 0px -18% 0px", threshold: 0.18 },
    );

    observer.observe(root);

    return () => {
      observer.disconnect();
    };
  }, [reduceMotion, rootRef, selector]);
}
