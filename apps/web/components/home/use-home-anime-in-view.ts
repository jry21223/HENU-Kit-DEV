"use client";

import { animate, stagger } from "animejs";
import { type RefObject, useLayoutEffect } from "react";

type InlineStyles = {
  opacity: string;
  translate: string;
  willChange: string;
};

type AnimationTarget = {
  element: HTMLElement;
  styles: InlineStyles;
};

function getInlineStyles(element: HTMLElement): InlineStyles {
  return {
    opacity: element.style.getPropertyValue("opacity"),
    translate: element.style.getPropertyValue("translate"),
    willChange: element.style.getPropertyValue("will-change"),
  };
}

function restoreProperty(element: HTMLElement, property: keyof InlineStyles, value: string) {
  const cssProperty = property === "willChange" ? "will-change" : property;

  if (value) {
    element.style.setProperty(cssProperty, value);
    return;
  }

  element.style.removeProperty(cssProperty);
}

function restoreTargets(targets: AnimationTarget[]) {
  for (const { element, styles } of targets) {
    restoreProperty(element, "opacity", styles.opacity);
    restoreProperty(element, "translate", styles.translate);
    restoreProperty(element, "willChange", styles.willChange);
  }
}

function prefersReducedMotion() {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

export function useHomeAnimeInView({
  reduceMotion,
  rootRef,
  selector,
}: {
  reduceMotion: boolean;
  rootRef: RefObject<HTMLElement | null>;
  selector: string;
}) {
  useLayoutEffect(() => {
    const root = rootRef.current;

    if (!root || reduceMotion || prefersReducedMotion()) {
      return;
    }

    if (!("IntersectionObserver" in window)) {
      return;
    }

    const targets = Array.from(root.querySelectorAll<HTMLElement>(selector));

    if (targets.length === 0) {
      return;
    }

    const animationTargets = targets.map((element) => ({
      element,
      styles: getInlineStyles(element),
    }));
    let animation: ReturnType<typeof animate> | null = null;

    for (const target of targets) {
      target.style.setProperty("opacity", "0");
      target.style.setProperty("translate", "0 12px");
      target.style.setProperty("will-change", "opacity, translate");
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) {
            continue;
          }

          animation = animate(targets, {
            opacity: [0, 1],
            translate: ["0 12px", "0 0"],
            delay: stagger(55),
            duration: 520,
            ease: "outCubic",
            onComplete: () => {
              restoreTargets(animationTargets);
            },
          });

          observer.disconnect();
        }
      },
      { rootMargin: "0px 0px -18% 0px", threshold: 0.18 },
    );

    observer.observe(root);

    return () => {
      observer.disconnect();
      animation?.cancel();
      restoreTargets(animationTargets);
    };
  }, [reduceMotion, rootRef, selector]);
}
