"use client";

import { useEffect } from "react";
import { gsap, ScrollTrigger } from "@/lib/gsap";
import { Observer } from "gsap/Observer";
import {
  DRAG_MINIMUM_PX,
  IDLE_WHEEL_BURST,
  gestureIntent,
  netDisplacementAfter,
  wheelBurstBlocked,
  wheelBurstStep,
  wheelBurstStepped,
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
 * 都会翻一整屏。判定本身是纯函数，在 `@/lib/navigation/gesture-intent` 里。
 *
 * 滚轮端按**突发聚合**判定（#510）：滚轮没有松手事件、也读不到速度，改用时间窗把一次突发
 * 聚合成一次判定，一次突发同样只走一屏。原来每个 tick 直接起跳，只靠 `animating` 挡动画期间
 * 的 tick——1.1s 补间一结束，同一次物理滚动（触控板惯性尾巴）剩下的 tick 就被当成第二次
 * 滚动，实测一次轻扫连跳两屏。判定同样是纯函数（`wheelBurstStep`），窗口与阈值的实测依据
 * 写在那个模块的常量注释里。键盘路径不变。
 *
 * 下面提到的 `Observer.js:NNN` 都指 `node_modules/gsap/src/Observer.js`（gsap 3.15.0 的
 * 可读源码）：`import "gsap/Observer"` 解析到的是它的构建产物，同一个文件里行号不同，
 * 照行号核对时请认准 `src/`。
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
         * 滚轮这次突发攒到哪儿了（#510）。判定是纯函数，状态只是数据：`wheelBurstStep`
         * 每次返回一份新的，这里换掉引用即可。
         *
         * 时钟用 tick 的到达时刻（`performance.now()`）：窗口问的是「这两个 tick 隔了多久」，
         * 与墙上时间无关，单调即可；`Observer` 不提供事件时刻，所以在回调里现取。
         */
        let wheelBurst = IDLE_WHEEL_BURST;

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

        /** 起跳一屏；返回这次到底有没有走成（动画期间或首末屏边界空转时什么都不做）。 */
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
         * 计到 `tolerance` 时才回调一次（`gsap/src/Observer.js:190-193`、`:203-210`），松手
         * 前不足一桶的零头永远拿不到，门槛分辨率也就只有桶粒度（12–16px，占 800 高视口
         * 48px 门槛的四分之一）。实测 8px/帧 × 21 帧（真实 168px）只报出 160px，最后 8px
         * 丢掉；`self.y` 是 Observer 自己在拖动路径上按 `clientY` 维护的位置
         * （同文件 `:253-258`），每次回调取值精确到像素。本票不动 Observer 的 `tolerance`
         * （仍为 12）。
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
            // 浏览器/系统打断这次手势（UA 取消，例如多指手势或系统接管滚动）不是读者松手，
            // 不判定：读者没有表达意图的机会，别替他决定翻一屏。净位移留到下一次按下时再
            // 重置。
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
              // 一次突发一屏（#510）：同一次突发的同方向 tick（触控板惯性尾巴）不消费判定。
              // 方向语义不变——仍然是 `readingDirection("wheel", deltaY)`，正数向下。
              //
              // `self.deltaY` 不是一个原始 tick：Observer 把一帧内的刻度累加进桶，凑够
              // `tolerance` 才回调一次（`Observer.js:190-210`、`:225-229`），所以这里拿到的是
              // 「这一次回调」的桶值；时刻取回调发生的这一刻（事件自己的 `timeStamp` 也在同一
              // 时基上，差不超过一帧）。
              const at = performance.now();
              const judged = wheelBurstStep(wheelBurst, { delta: self.deltaY, at });
              wheelBurst = judged.state;
              if (animating) {
                // 补间还占着，这一下走不成：把窗口从这一刻重新起算（`go()` 自己也会因为
                // `animating` 直接返回）。补间在负载高时会比名义的 1.1s 长得多，不这样做，
                // 窗口会从动画开始那刻起算，读者还在流的那段滚动就会被当成新的一次手势。
                // 判定本身不记账，所以「动画期间滚了一下、落定后应该走一屏」仍然成立。
                wheelBurst = wheelBurstBlocked(wheelBurst, at);
                return;
              }
              if (judged.intent.action !== "step") return;
              // 真的起跳了才记账：没走成（首末屏空转）不该把这次突发花掉——与 #509 触摸端
              // 「抬手前什么都不消费」的语义一致。
              if (go(judged.intent.direction)) {
                wheelBurst = wheelBurstStepped(wheelBurst, judged.intent.direction);
              }
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
