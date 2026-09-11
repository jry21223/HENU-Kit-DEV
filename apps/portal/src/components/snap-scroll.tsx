"use client";

import { useEffect } from "react";
import { gsap, ScrollTrigger } from "@/lib/gsap";
import { Observer } from "gsap/Observer";
import {
  DRAG_MINIMUM_PX,
  gestureIntent,
  netDisplacementAfter,
  readingDirection,
} from "@/lib/navigation/gesture-intent";

/**
 * md+ 且非 reduced-motion 时接管滚轮/触摸/键盘：
 * 一次滚动非线性（power2.inOut, ~1.1s）切换到上/下一个 .snap-screen 模块，
 * 动画期间加锁防连滚；切入的模块内容做 autoAlpha 0.35→1 + 轻微 y 位移的淡入。
 * reduced-motion / 小屏：不接管，退回普通滚动。
 *
 * 触摸/指针端按**手势意图**判定（#509）：松手时看这次手势的净位移（同向累加、反向重置）
 * 与峰值速度，一次手势只判一次。整屏吸附原来的判据只有 Observer 的桶累计阈值
 * `tolerance: 12`——位移累加到 12px 就回调一次，于是十几像素的手抖、误触、点击前的位移
 * 都会翻一整屏。判定本身是纯函数，在 `@/lib/navigation/gesture-intent` 里。滚轮与键盘
 * 路径不变；滚轮侧的 burst 聚合留给 #510。
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

        /** 起跳一屏；动画期间或首末屏边界空转时什么都不做。 */
        const go = (dir: 1 | -1): void => {
          if (animating) return;
          const from = currentIndex();
          const next = Math.min(sections.length - 1, Math.max(0, from + dir));
          if (next === from) return;
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
        };

        // 这次手势的净位移（输入自身的符号）与上一次取样的指针位置。
        let netDisplacement = 0;
        let lastSampledY = 0;
        // 这次手势有没有按下来过：Observer 把触摸/指针的抬起挂在 document 上，窗口外按下
        // 再抬进页面、或者飘来的 pointerup 都会回调 onRelease，那时指针位置还是上一次手势
        // 留下的，不能当成这次的手势。
        let pressed = false;

        /**
         * 把手势里的位移并进净位移：**同向累加，反向就以新方向重新起算**。
         *
         * 取样用 Observer 报的指针位置（`self.y`），不用它的 `deltaY`：`deltaY` 只在桶累
         * 计到 `tolerance` 时才回调一次（`Observer.js:190-193`、`203-210`），松手前不足一桶
         * 的零头永远拿不到，门槛分辨率也就只有桶粒度（12–16px，占 800 高视口 48px 门槛的
         * 四分之一）。实测 8px/帧 × 21 帧（真实 168px）只报出 160px，最后 8px 丢掉；
         * `self.y` 是 Observer 自己在拖动路径上按 `clientY` 维护的位置
         * （`Observer.js:253-258`），每次回调取值精确到像素。本票不动 Observer 的
         * `tolerance`（仍为 12）。
         */
        const sampleTouchDisplacement = (self: Observer) => {
          const y = self.y;
          // 指针位置缺失时不动净位移（类型里 `y?: number` 是给不带指针位置的输入留的口
          // 子；触摸/指针的每次拖动都带着 clientY）。
          if (y === undefined) return;
          const delta = y - lastSampledY;
          lastSampledY = y;
          netDisplacement = netDisplacementAfter(netDisplacement, delta);
        };

        const observer = Observer.create({
          type: "wheel,touch",
          preventDefault: true,
          tolerance: 12,
          // `dragMinimum` 依 #507 的决定取 8：小于它的位移不构成拖动，轻触与点击前的抖动
          // 连拖动状态都进不去（第一道手段；第二道是下面按视口比例取的位移门槛）。
          dragMinimum: DRAG_MINIMUM_PX,
          onPress: (self) => {
            pressed = true;
            netDisplacement = 0;
            // Observer 按下时就把 startY 设成指针位置（`Observer.js:273` 的 `_onPress`）；
            // `?? 0` 只用来满足类型里的 `startY?: number`。
            lastSampledY = self.startY ?? 0;
          },
          onRelease: (self) => {
            if (!pressed) return;
            pressed = false;
            // 系统取消的手势（第二根手指、浏览器接管）不是读者松手，不判定：读者没有表达
            // 意图的机会，别替他决定翻一屏。净位移留到下一次按下时再重置。
            if (self.event.type === "touchcancel" || self.event.type === "pointercancel") return;
            // 松手补上最后一段：不足一桶的零头也属于这次手势。
            sampleTouchDisplacement(self);
            const intent = gestureIntent({
              source: "touch",
              netDisplacement,
              // 速度只能在松手回调里读：Observer 进 onStop 之前先把速度清零
              // （`Observer.js:183-188`），在 onStop 里读出来恒为 0。
              peakVelocity: self.velocityY,
              viewportHeight: window.innerHeight,
            });
            netDisplacement = 0;
            // 一次手势一屏：判定只在松手做这一次，中途不消费任何东西——#506 的两条契约因此
            // 都还在：动画期间开始的手势不被整段吞掉（位移一直攒着，落定后松手即可补跳），
            // 首末屏边界空转也不会把这次手势提前花掉。
            if (intent.action === "step") go(intent.direction);
          },
          onChangeY: (self) => {
            if (self.event.type === "wheel") {
              // 滚轮侧本票不动（#510）：每个 tick 仍然直接起跳，方向语义与改造前一致。
              go(readingDirection("wheel", self.deltaY));
              return;
            }
            sampleTouchDisplacement(self);
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
