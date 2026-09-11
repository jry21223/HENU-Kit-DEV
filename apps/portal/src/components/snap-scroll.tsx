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

        /**
         * 读者实际待着的那一屏：视口里可见高度最大的 section；恰好各占一半时取靠下
         * 那一屏。
         *
         * 不能用「scrollY + 35% 视口高」的探针：读者并不总停在整屏边界上——视口变大
         * （旋转、拉窗口）或内容回流会让整屏模块重新长高，而浏览器保持 scrollY 不动，
         * 读者于是可能落到某个边界上方不足一屏处。这类落点里，δ 落在 35%~50% 视口高
         * 那一段时探针会少算一屏：读者主要看着下面那一屏（恰好各占一半时按平局归它），
         * 探针却仍把他算在上一屏（δ ≤ 35% 时两者本来一致，δ > 50% 时他确实该算上
         * 一屏）。起点少算一屏的下场：第一处边界上向上滑被判成「已经在第一屏」而空
         * 转，整段手势被吃掉；更高的边界上则从读者看着的那一屏连退两屏。
         */
        const currentIndex = () => {
          const scrollTop = window.scrollY;
          const viewportBottom = scrollTop + window.innerHeight;
          let idx = 0;
          let mostVisible = -1;
          sections.forEach((s, i) => {
            const top = s.offsetTop;
            const visible = Math.max(
              0,
              Math.min(top + s.offsetHeight, viewportBottom) - Math.max(top, scrollTop)
            );
            // 平局归靠下那屏：各占一半时读者上/下都还有屏可去。末屏上的平局是例外
            // ——那里本就没有下一屏，go(1) 空转且不消费手势，向上滑照常离开。
            if (visible >= mostVisible) {
              mostVisible = visible;
              idx = i;
            }
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
