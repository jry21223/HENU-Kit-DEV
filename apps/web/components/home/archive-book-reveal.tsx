"use client";

import Link from "next/link";
import { motion, useScroll, useTransform } from "motion/react";
import { useEffect, useRef, useState } from "react";
import { archiveDirectory, courseBooks } from "./home-data";
import { PdfCourseBook } from "./pdf-course-book";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";
import styles from "./home-visuals.module.css";

export function ArchiveBookReveal() {
  const ref = useRef<HTMLElement>(null);
  const reduceMotion = usePrefersReducedMotion();
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end end"],
  });
  const [contentReady, setContentReady] = useState(() => scrollYProgress.get() >= 0.62);

  const rotate = useTransform(scrollYProgress, [0, 0.32], [-10, 0]);
  const y = useTransform(scrollYProgress, [0, 0.32], [120, 0]);
  const scale = useTransform(scrollYProgress, [0, 0.32], [0.86, 1]);
  const coverRotate = useTransform(scrollYProgress, [0.35, 0.78], [0, -172]);
  const contentOpacity = useTransform(scrollYProgress, [0.62, 0.78], [0, 1]);
  const contentY = useTransform(scrollYProgress, [0.62, 0.78], [24, 0]);

  useEffect(() => {
    if (reduceMotion) {
      setContentReady(true);
      return;
    }

    const updateContentReady = () => {
      const stage = ref.current;

      if (!stage) {
        return;
      }

      const rect = stage.getBoundingClientRect();
      const scrollableDistance = Math.max(stage.offsetHeight - window.innerHeight, 1);
      const progress = Math.min(Math.max(-rect.top / scrollableDistance, 0), 1);
      const nextReady = progress >= 0.62;

      setContentReady((current) => (current === nextReady ? current : nextReady));
    };

    updateContentReady();
    window.addEventListener("scroll", updateContentReady, { passive: true });
    window.addEventListener("resize", updateContentReady);

    return () => {
      window.removeEventListener("scroll", updateContentReady);
      window.removeEventListener("resize", updateContentReady);
    };
  }, [reduceMotion]);

  const contentFocusable = reduceMotion || contentReady;
  const contentTabIndex = contentFocusable ? undefined : -1;
  const contentAriaHidden = contentFocusable ? undefined : true;

  return (
    <section ref={ref} className={styles.bookStage} aria-label="课程资料档案册">
      <div className={styles.bookSticky}>
        <motion.div
          className={styles.archiveBook}
          style={{
            rotate: reduceMotion ? 0 : rotate,
            scale: reduceMotion ? 1 : scale,
            y: reduceMotion ? 0 : y,
          }}
        >
          <div className={styles.bookBase} aria-hidden="true" />
          <div className={styles.bookInside}>
            <motion.div
              className={`${styles.bookPage} p-6 xl:p-7`}
              aria-hidden={contentAriaHidden}
              style={{
                opacity: reduceMotion ? 1 : contentOpacity,
                pointerEvents: contentFocusable ? "auto" : "none",
                y: reduceMotion ? 0 : contentY,
              }}
            >
              <p className="font-mono text-xs font-semibold tracking-[0.18em] text-[#b75c32]">资料档案</p>
              <h2 className="mt-3 text-3xl font-black tracking-tight text-[#2b2117] xl:text-4xl">资料目录</h2>
              <div className="mt-5 grid gap-1">
                {archiveDirectory.slice(0, 6).map((item) => (
                  <Link key={item.label} className={`${styles.directoryLine} group block py-2`} href={item.href} tabIndex={contentTabIndex}>
                    <span className="flex items-baseline justify-between gap-3">
                      <span className="text-base font-bold text-[#2b2117] group-hover:text-[#2f6b58] xl:text-lg">{item.label}</span>
                      <span className="font-mono text-xs text-[#a26b43]">打开</span>
                    </span>
                    <span className="mt-1 block text-sm leading-5 text-[#756653] xl:leading-6">{item.description}</span>
                  </Link>
                ))}
              </div>
            </motion.div>

            <motion.div
              className={`${styles.bookPage} p-5 xl:p-6`}
              aria-hidden={contentAriaHidden}
              style={{
                opacity: reduceMotion ? 1 : contentOpacity,
                pointerEvents: contentFocusable ? "auto" : "none",
                y: reduceMotion ? 0 : contentY,
              }}
            >
              <p className="font-mono text-xs font-semibold tracking-[0.18em] text-[#b75c32]">课程 PDF</p>
              <h2 className="mt-3 text-4xl font-black tracking-tight text-[#2b2117]">课程入口</h2>
              <div className="mt-5 grid grid-cols-2 gap-3">
                {courseBooks.map((course) => (
                  <PdfCourseBook key={course.label} course={course} tabIndex={contentTabIndex} />
                ))}
              </div>
            </motion.div>
          </div>

          <motion.div
            className={styles.bookCover}
            style={{ rotateY: reduceMotion ? -172 : coverRotate }}
            aria-hidden="true"
          >
            <div className="flex h-full flex-col justify-between p-12">
              <div>
                <p className="font-mono text-sm font-semibold tracking-[0.18em] text-[#593a24]/70">软件学院</p>
                <h2 className="mt-5 max-w-md text-6xl font-black leading-[0.94] tracking-tight text-[#2b2117]">
                  软件学院资料库
                </h2>
              </div>
              <p className="max-w-sm text-base leading-7 text-[#593a24]/76">
                课程资料、真题、刷题、共创和资料保障，从这一册开始展开。
              </p>
            </div>
          </motion.div>
        </motion.div>
      </div>
    </section>
  );
}
