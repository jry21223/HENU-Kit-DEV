"use client";

import { useEffect } from "react";
import { gsap, ScrollTrigger } from "@/lib/gsap";
import { Observer } from "gsap/Observer";

/**
 * md+ 且非 reduced-motion 时接管滚轮/触摸/键盘：
 * 一次滚动非线性（power2.inOut, ~1.1s）切换到上/下一个 .snap-screen 模块，
 * 动画期间加锁防连滚；切入的模块内容做 autoAlpha 0.35→1 + 轻微 y 位移的淡入。
 * reduced-motion / 小屏：不接管，退回普通滚动。
 */
export default function SnapScroll() {
  useEffect(() => {
    const mm = gsap.matchMedia();

    mm.add(
      "(min-width: 768px) and (prefers-reduced-motion: no-preference)",
      () => {
        const sections = gsap.utils.toArray<HTMLElement>(".snap-screen");
        if (sections.length < 2) return;

        let animating = false;

        const currentIndex = () => {
          const y = window.scrollY + window.innerHeight * 0.35;
          let idx = 0;
          sections.forEach((s, i) => {
            if (s.offsetTop <= y) idx = i;
          });
          return idx;
        };

        /** 真的起跳并返回 true；动画期间或首末屏边界空转时返回 false。 */
        const go = (dir: 1 | -1): boolean => {
          if (animating) return false;
          const from = currentIndex();
          const next = Math.min(sections.length - 1, Math.max(0, from + dir));
          if (next === from) return false;
          animating = true;

          const target = sections[next];
          gsap.to(window, {
            scrollTo: { y: target.offsetTop, autoKill: false },
            duration: 1.1,
            ease: "power2.inOut",
            onComplete: () => {
              animating = false;
              ScrollTrigger.refresh();
            },
            onInterrupt: () => {
              animating = false;
            },
          });

          // 切入模块整体淡入（autoAlpha 0.35→1 + 轻微 y 位移）
          gsap.fromTo(
            target,
            { autoAlpha: 0.35, y: 24 },
            {
              autoAlpha: 1,
              y: 0,
              duration: 0.9,
              delay: 0.25,
              ease: "power2.out",
              overwrite: "auto",
              clearProps: "transform",
            }
          );

          return true;
        };

        // Observer 的 deltaY 是「输入自身的位移」，两种输入的方向相反：滚轮 deltaY
        // 就是页面滚动量（正数 = 向下），而触摸/指针拖动给的是手指位移（负数 = 手指
        // 上滑 = 页面向下）。只判正负会让触摸端整屏切换反向——手指上滑退回上一屏，
        // 手指下滑反而进下一屏；鼠标滚轮和键盘却始终正常。所以先归一化成「读者在往
        // 下走」，再决定切哪一屏。
        const isScrollingDown = (self: Observer) =>
          self.event.type === "wheel" ? self.deltaY > 0 : self.deltaY < 0;

        // 一次触摸手势只切一屏：Observer 每个累积位移都会回调，而整屏动画只有约
        // 1.1s，慢速拖动会在动画结束后继续产生位移，同一次拖动因此连切两屏。手势
        // 起止由按下/抬起圈定（不用 onStop：被机器拉长的位移之间会有空档，那会让
        // 同一次手势重新起跳）。滚轮读者连续滚动仍然一屏一屏走，不受此锁影响。
        let gestureSpent = false;

        const observer = Observer.create({
          type: "wheel,touch",
          preventDefault: true,
          tolerance: 12,
          onPress: () => {
            gestureSpent = false;
          },
          onRelease: () => {
            gestureSpent = false;
          },
          onChangeY: (self) => {
            const fromWheel = self.event.type === "wheel";
            const direction = isScrollingDown(self) ? 1 : -1;
            if (fromWheel) {
              go(direction);
              return;
            }
            if (gestureSpent) return;
            // 只有真的起跳才消费这次手势：动画期间或首末屏边界的空转不能把读者的
            // 一次滑动整段吃掉——同一次手势在动画结束后仍应能补跳。
            if (go(direction)) gestureSpent = true;
          },
        });

        const onKey = (e: KeyboardEvent) => {
          if (e.metaKey || e.ctrlKey || e.altKey) return;
          if (e.key === "ArrowDown" || e.key === "PageDown" || e.key === " ") {
            e.preventDefault();
            go(1);
          } else if (e.key === "ArrowUp" || e.key === "PageUp") {
            e.preventDefault();
            go(-1);
          }
        };
        window.addEventListener("keydown", onKey);

        return () => {
          observer.kill();
          window.removeEventListener("keydown", onKey);
          gsap.killTweensOf(window);
        };
      }
    );

    return () => mm.revert();
  }, []);

  return null;
}
