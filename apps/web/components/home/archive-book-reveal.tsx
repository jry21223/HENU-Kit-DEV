"use client";

import Link from "next/link";
import { ArrowRight, Search } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { archiveDirectory, courseBooks, heroLinks } from "./home-data";
import { homeAnimAttr } from "./home-animation-selectors";
import { PdfCourseBook } from "./pdf-course-book";
import { useHomeAnimeTimeline } from "./use-home-anime-timeline";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";
import styles from "./home-visuals.module.css";

export function ArchiveBookReveal() {
  const ref = useRef<HTMLElement>(null);
  const reduceMotion = usePrefersReducedMotion();
  const [clientAnimationReady, setClientAnimationReady] = useState(false);
  const [contentReady, setContentReady] = useState(false);
  const [introReady, setIntroReady] = useState(true);
  const [pageVisible, setPageVisible] = useState(false);
  const [closingCopyVisible, setClosingCopyVisible] = useState(false);
  const readiness = useMemo(
    () => ({
      setClosingCopyVisible,
      setContentReady,
      setIntroReady,
      setPageVisible,
    }),
    [],
  );

  useEffect(() => {
    setClientAnimationReady(!window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  }, [reduceMotion]);

  useHomeAnimeTimeline({ readiness, reduceMotion: reduceMotion || !clientAnimationReady, stageRef: ref });

  const fallbackVisible = !clientAnimationReady || reduceMotion;
  const contentFocusable = fallbackVisible || contentReady;
  const contentTabIndex = contentFocusable ? undefined : -1;
  const contentAriaHidden = contentFocusable ? undefined : true;
  const pageVisibility = fallbackVisible || pageVisible ? "visible" : "hidden";
  const introFocusable = fallbackVisible || introReady;
  const introTabIndex = introFocusable ? undefined : -1;
  const introAriaHidden = introFocusable ? undefined : true;
  const noScriptArchiveFallbackStyles = `
.${styles.bookStage} {
  min-height: auto;
}

.${styles.bookStage} .${styles.bookSticky} {
  position: relative;
}

.${styles.bookStage} .${styles.bookScene} {
  display: block;
}

.${styles.bookStage} .${styles.bookCopyPanel} {
  position: relative;
}

.${styles.bookStage} .${styles.bookDock} {
  position: relative;
  top: auto;
  left: auto;
  width: min(1040px, 92vw);
  margin: 32px auto 0;
  transform: none;
}

.${styles.bookStage} .${styles.archiveBook} {
  transform: none;
}

.${styles.bookStage} .${styles.bookBase},
.${styles.bookStage} .${styles.bookInside},
.${styles.bookStage} .${styles.bookPage} {
  opacity: 1;
  transform: none;
}

.${styles.bookStage} .${styles.bookCover} {
  opacity: 0;
  transform: rotateY(-176deg);
}

.${styles.bookStage} .${styles.bookCoverShadow},
.${styles.bookStage} .${styles.bookSpineShadow} {
  opacity: 0;
  transform: none;
}

.${styles.bookStage} [data-home-anim="archive-closing-copy"] {
  opacity: 0;
  transform: none;
}
`;

  return (
    <>
      <noscript>
        <style dangerouslySetInnerHTML={{ __html: noScriptArchiveFallbackStyles }} />
      </noscript>
      <section ref={ref} className={styles.bookStage} aria-label="课程资料档案册">
        <div className={styles.bookSticky}>
          <div className={styles.bookScene}>
            <div className={styles.bookCopy}>
              <div
              className={styles.bookCopyPanel}
              data-testid="archive-copy-intro"
              {...homeAnimAttr("archiveIntroCopy")}
              aria-hidden={introAriaHidden}
              style={{
                pointerEvents: introFocusable ? "auto" : "none",
                visibility: introFocusable ? "visible" : "hidden",
              }}
            >
              <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Final Review Platform</p>
              <h1 className="mt-5 text-5xl font-black leading-[0.95] tracking-tight text-[#2b2117] xl:text-7xl">
                打开你的期末复习资料册
              </h1>
              <p className="mt-6 max-w-lg text-base leading-7 text-[#685b4b] sm:text-lg">
                按课程找到讲义、真题、实验资料和复习包，围绕资料继续刷题、讨论和共创。
              </p>
              <form action="/search" className="mt-7 flex max-w-lg items-center rounded-2xl border border-[#2b2117]/14 bg-white/86 p-2 shadow-[0_20px_60px_rgba(71,49,27,0.11)]" method="get">
                <label className="sr-only" htmlFor="archive-search">
                  搜索课程、讲义、真题、实验资料
                </label>
                <Search className="ml-2 size-5 shrink-0 text-[#9a7154]" aria-hidden="true" />
                <input id="archive-search" name="q" className="min-w-0 flex-1 bg-transparent px-3 py-2 text-sm text-[#2b2117] outline-none placeholder:text-[#9b8b78]" placeholder="搜索课程、讲义、真题、实验资料" tabIndex={introTabIndex} type="search" />
                <button className="inline-flex shrink-0 items-center rounded-xl bg-[#2f6b58] px-4 py-2 text-sm font-semibold text-white transition hover:bg-[#285a4b]" tabIndex={introTabIndex} type="submit">
                  搜索
                </button>
              </form>
              <div className="mt-6 flex flex-wrap gap-3">
                <Link className="inline-flex items-center gap-2 rounded-full bg-[#2b2117] px-5 py-3 text-sm font-semibold text-white shadow-[0_16px_40px_rgba(43,33,23,0.2)] transition hover:-translate-y-0.5" href={heroLinks.primary.href} tabIndex={introTabIndex}>
                  {heroLinks.primary.label}
                  <ArrowRight className="size-4" aria-hidden="true" />
                </Link>
                <Link className="inline-flex items-center rounded-full border border-[#2b2117]/18 bg-white/72 px-5 py-3 text-sm font-semibold text-[#2b2117] transition hover:-translate-y-0.5 hover:bg-white" href={heroLinks.secondary.href} tabIndex={introTabIndex}>
                  {heroLinks.secondary.label}
                </Link>
              </div>
            </div>

            <div
              className={styles.bookCopyPanel}
              data-testid="archive-copy-closing"
              {...homeAnimAttr("archiveClosingCopy")}
              style={{
                pointerEvents: "none",
                visibility: clientAnimationReady && !reduceMotion && closingCopyVisible ? "visible" : "hidden",
              }}
              aria-hidden="true"
            >
              <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Community</p>
              <h2 className="mt-4 text-4xl font-black leading-[1] tracking-tight text-[#2b2117] xl:text-6xl">
                资料合上以后，还会继续生长
              </h2>
              <p className="mt-5 max-w-md text-base leading-7 text-[#685b4b]">
                再往下是 Wiki、博客、课程帖子和动态。资料不是一次性下载，而是围绕课程持续补充、勘误和共创。
              </p>
            </div>
          </div>

          <div className={styles.bookDock} style={{ pointerEvents: contentFocusable ? "auto" : "none" }}>
            <div
              className={styles.archiveBook}
              data-testid="archive-book"
              {...homeAnimAttr("archiveBook")}
            >
              <div
                className={styles.bookBase}
                aria-hidden="true"
                {...homeAnimAttr("archiveBase")}
              />
              <span
                className={styles.bookSpineShadow}
                aria-hidden="true"
                {...homeAnimAttr("archiveSpineShadow")}
              />
              <span
                className={styles.bookCoverShadow}
                aria-hidden="true"
                {...homeAnimAttr("archiveCoverShadow")}
              />
              <div className={styles.bookInside} {...homeAnimAttr("archiveInside")}>
                <div
                  className={`${styles.bookPage} ${styles.directoryPage} p-4 xl:p-5`}
                  aria-hidden={contentAriaHidden}
                  {...homeAnimAttr("archivePage")}
                  style={{
                    pointerEvents: contentFocusable ? "auto" : "none",
                    visibility: pageVisibility,
                  }}
                >
                  <span className={styles.pageHighlight} aria-hidden="true" {...homeAnimAttr("archivePageHighlight")} />
                  <span className={styles.directoryScan} aria-hidden="true" {...homeAnimAttr("archiveDirectoryScan")} />
                  <p className="font-mono text-xs font-semibold tracking-[0.18em] text-[#b75c32]">资料档案</p>
                  <h2 className="mt-2 text-2xl font-black tracking-tight text-[#2b2117] xl:text-3xl">资料目录</h2>
                  <div className="mt-3 grid gap-0.5">
                    {archiveDirectory.slice(0, 6).map((item) => (
                      <Link
                        key={item.label}
                        className={`${styles.directoryLine} group block py-1.5`}
                        href={item.href}
                        tabIndex={contentTabIndex}
                        {...homeAnimAttr("archiveDirectoryLine")}
                      >
                        <span className="flex items-baseline justify-between gap-3">
                          <span className="text-sm font-bold text-[#2b2117] group-hover:text-[#2f6b58] xl:text-base">{item.label}</span>
                          <span className="font-mono text-xs text-[#a26b43]">打开</span>
                        </span>
                        <span className="mt-0.5 block text-xs leading-5 text-[#756653]">{item.description}</span>
                      </Link>
                    ))}
                  </div>
                </div>

                <div
                  className={`${styles.bookPage} ${styles.coursePage} p-4`}
                  aria-hidden={contentAriaHidden}
                  {...homeAnimAttr("archivePage")}
                  style={{
                    pointerEvents: contentFocusable ? "auto" : "none",
                    visibility: pageVisibility,
                  }}
                >
                  <span className={styles.pageHighlight} aria-hidden="true" {...homeAnimAttr("archivePageHighlight")} />
                  <p className="font-mono text-xs font-semibold tracking-[0.18em] text-[#b75c32]">课程 PDF</p>
                  <h2 className="mt-2 text-2xl font-black tracking-tight text-[#2b2117] xl:text-3xl">课程入口</h2>
                  <div className="mt-4 grid grid-cols-2 gap-3">
                    {courseBooks.map((course) => (
                      <PdfCourseBook key={course.label} animationMarked compact course={course} tabIndex={contentTabIndex} />
                    ))}
                  </div>
                </div>
              </div>

              <div
                className={styles.bookCover}
                data-testid="archive-cover"
                aria-hidden="true"
                {...homeAnimAttr("archiveCover")}
              >
                <span className={styles.coverBack} aria-hidden="true" />
                <div className={styles.coverFront} {...homeAnimAttr("archiveCoverFront")}>
                  <div className="flex h-full flex-col justify-between p-7 xl:p-8">
                    <div>
                      <p className="font-mono text-xs font-semibold tracking-[0.18em] text-[#593a24]/70 xl:text-sm">软件学院</p>
                      <h2 className="mt-4 max-w-none text-3xl font-black leading-[0.98] tracking-tight text-[#2b2117] xl:text-4xl">
                        软件学院资料库
                      </h2>
                    </div>
                    <p className="max-w-xs text-sm leading-6 text-[#593a24]/76">
                      课程资料、真题、刷题、共创和资料保障，从这一册开始展开。
                    </p>
                  </div>
                  <span className={styles.coverLabel} aria-hidden="true">
                    A4 ARCHIVE
                  </span>
                </div>
              </div>
              <div className={styles.bookSpine} data-testid="archive-seam" aria-hidden="true" {...homeAnimAttr("archiveSpine")} />
              <div className={styles.bookPencil} aria-hidden="true">
                <span className={styles.pencilEraser} />
                <span className={styles.pencilTip} />
              </div>
            </div>
          </div>
        </div>
      </div>
      </section>
    </>
  );
}
