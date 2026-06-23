"use client";

import Link from "next/link";
import { useRef } from "react";
import { homeAnimSelector } from "./home-animation-selectors";
import { archiveDirectory, courseBooks } from "./home-data";
import { PdfCourseBook } from "./pdf-course-book";
import { useHomeAnimeInView } from "./use-home-anime-in-view";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";
import styles from "./home-visuals.module.css";

const visibleDirectory = archiveDirectory.slice(0, 6);

export function MobileArchiveIntro() {
  const sectionRef = useRef<HTMLElement>(null);
  const reduceMotion = usePrefersReducedMotion();

  useHomeAnimeInView({
    reduceMotion,
    rootRef: sectionRef,
    selector: homeAnimSelector("courseBook"),
  });

  return (
    <section
      ref={sectionRef}
      aria-labelledby="mobile-archive-title"
      className={`${styles.mobileArchive} mx-auto w-[min(720px,calc(100%-32px))] pb-16`}
    >
      <div className={`${styles.mobileBook} p-5`}>
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.16em] text-[#593a24]/70">移动资料册</p>
        <h2 id="mobile-archive-title" className="mt-3 text-3xl font-black leading-tight text-[#2b2117]">
          资料册已打开
        </h2>
        <p className="mt-3 text-sm leading-7 text-[#593a24]/78">
          课程资料、真题和复习包先按目录收好，课程入口从下方滑出。
        </p>
        <div className="mt-5 grid grid-cols-2 gap-2">
          {visibleDirectory.map((item) => (
            <Link
              key={`${item.label}-${item.href}`}
              className="rounded-2xl bg-white/74 px-3 py-2 text-sm font-semibold text-[#2b2117] transition hover:bg-white"
              href={item.href}
            >
              {item.label}
            </Link>
          ))}
        </div>
      </div>

      <div
        aria-label="课程资料入口"
        className="mt-6 flex snap-x gap-4 overflow-x-auto pb-5"
      >
        {courseBooks.map((course) => (
          <div key={`${course.code}-${course.label}`} className="w-56 shrink-0 snap-start">
            <PdfCourseBook course={course} />
          </div>
        ))}
      </div>
    </section>
  );
}
