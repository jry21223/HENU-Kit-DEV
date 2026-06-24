"use client";

import { createTimeline } from "animejs";
import { useEffect, type Dispatch, type RefObject, type SetStateAction } from "react";
import { homeAnimSelector } from "./home-animation-selectors";

export const archiveProgress = {
  introEnd: 0.42,
  straightStart: 0.44,
  openStart: 0.52,
  openEnd: 0.93,
  closingStart: 0.93,
} as const;

type ReadinessSetter = Dispatch<SetStateAction<boolean>>;

type ArchiveReadiness = {
  setClosingCopyVisible: ReadinessSetter;
  setContentReady: ReadinessSetter;
  setIntroReady: ReadinessSetter;
  setPageVisible: ReadinessSetter;
};

type UseHomeAnimeTimelineOptions = {
  readiness: ArchiveReadiness;
  reduceMotion: boolean;
  stageRef: RefObject<HTMLElement | null>;
};

const timelineDuration = 1000;
const insideRevealStart = 0.58;
const insideRevealEnd = 0.76;
const pageInteractionStart = 0.78;
const pageVisibilityStart = archiveProgress.openStart;
const pageVisualEnd = 0.96;
const closingVisualStart = 0.97;

function at(progress: number) {
  return progress * timelineDuration;
}

function durationBetween(start: number, end: number) {
  return at(end - start);
}

function clampProgress(progress: number) {
  return Math.min(Math.max(progress, 0), 1);
}

function setReady(setter: ReadinessSetter, nextReady: boolean) {
  setter((current) => (current === nextReady ? current : nextReady));
}

function syncReadiness(progress: number, readiness: ArchiveReadiness, reduceMotion: boolean) {
  if (reduceMotion) {
    setReady(readiness.setContentReady, true);
    setReady(readiness.setIntroReady, true);
    setReady(readiness.setPageVisible, true);
    setReady(readiness.setClosingCopyVisible, false);
    return;
  }

  setReady(readiness.setContentReady, progress >= pageInteractionStart && progress <= archiveProgress.openEnd);
  setReady(readiness.setIntroReady, progress < archiveProgress.introEnd);
  setReady(readiness.setPageVisible, progress >= pageVisibilityStart && progress <= pageVisualEnd);
  setReady(readiness.setClosingCopyVisible, progress >= closingVisualStart);
}

function getStageScrollProgress(stage: HTMLElement) {
  const stageTop = window.scrollY + stage.getBoundingClientRect().top;
  const scrollableDistance = Math.max(stage.offsetHeight - window.innerHeight, 1);

  return clampProgress((window.scrollY - stageTop) / scrollableDistance);
}

function createArchiveTimeline(stage: HTMLElement) {
  const book = stage.querySelector<HTMLElement>(homeAnimSelector("archiveBook"));
  const base = stage.querySelector<HTMLElement>(homeAnimSelector("archiveBase"));
  const inside = stage.querySelector<HTMLElement>(homeAnimSelector("archiveInside"));
  const cover = stage.querySelector<HTMLElement>(homeAnimSelector("archiveCover"));
  const coverFront = stage.querySelector<HTMLElement>(homeAnimSelector("archiveCoverFront"));
  const coverShadow = stage.querySelector<HTMLElement>(homeAnimSelector("archiveCoverShadow"));
  const leftPanel = stage.querySelector<HTMLElement>(homeAnimSelector("archiveLeftPanel"));
  const rightPanel = stage.querySelector<HTMLElement>(homeAnimSelector("archiveRightPanel"));
  const spineShadow = stage.querySelector<HTMLElement>(homeAnimSelector("archiveSpineShadow"));
  const introCopy = stage.querySelector<HTMLElement>(homeAnimSelector("archiveIntroCopy"));
  const closingCopy = stage.querySelector<HTMLElement>(homeAnimSelector("archiveClosingCopy"));
  const timeline = createTimeline({ autoplay: false, defaults: { ease: "linear" } });

  if (book) {
    timeline
      .add(
        book,
        {
          duration: at(archiveProgress.straightStart),
          ease: "outCubic",
          rotate: [-12, 0],
          x: [300, 0],
          y: [220, 0],
        },
        0,
      )
      .add(
        book,
        {
          duration: durationBetween(archiveProgress.openEnd, 1),
          ease: "inOutSine",
          rotate: [0, 2],
          y: [0, 18],
        },
        at(archiveProgress.openEnd),
      );
  }

  if (cover) {
    timeline
      .add(
        cover,
        {
          duration: durationBetween(archiveProgress.openStart, 0.62),
          ease: "inCubic",
          rotateY: [0, -8],
          scaleX: [1, 0.96],
          x: [0, -2],
          z: [0, 28],
        },
        at(archiveProgress.openStart),
      )
      .add(
        cover,
        {
          duration: durationBetween(0.62, 0.78),
          ease: "inOutSine",
          opacity: [1, 0.18],
          rotateY: [-8, -96],
          scaleX: [0.96, 0.24],
          x: [-2, -10],
          z: [28, 24],
        },
        at(0.62),
      )
      .add(
        cover,
        {
          duration: durationBetween(0.78, 0.9),
          ease: "outCubic",
          opacity: [0.18, 0.015],
          rotateY: [-96, -164],
          scaleX: [0.24, 0.1],
          x: [-10, -18],
          z: [24, 10],
        },
        at(0.78),
      )
      .add(
        cover,
        {
          duration: durationBetween(0.9, archiveProgress.openEnd),
          ease: "outCubic",
          opacity: [0.015, 0],
          rotateY: [-164, -178],
          scaleX: [0.1, 0.08],
          x: [-18, -18],
          z: [10, 0],
        },
        at(0.9),
      )
      .add(
        cover,
        {
          duration: durationBetween(0.94, 1),
          ease: "inOutSine",
          opacity: [0, 1],
          rotateY: [-178, 0],
          x: [-18, 0],
          z: [0, 0],
        },
        at(0.94),
      );
  }

  if (coverFront) {
    timeline
      .add(
        coverFront,
        {
          duration: durationBetween(0.58, 0.68),
          ease: "inCubic",
          opacity: [1, 0],
        },
        at(0.58),
      )
      .add(
        coverFront,
        {
          duration: durationBetween(0.94, 1),
          ease: "inOutSine",
          opacity: [0, 1],
        },
        at(0.94),
      );
  }

  if (coverShadow) {
    timeline
      .add(
        coverShadow,
        {
          duration: durationBetween(archiveProgress.openStart, 0.66),
          ease: "outCubic",
          opacity: [0, 0.28],
          scaleX: [0.58, 0.84],
          x: [0, -18],
        },
        at(archiveProgress.openStart),
      )
      .add(
        coverShadow,
        {
          duration: durationBetween(0.66, archiveProgress.openEnd),
          ease: "inCubic",
          opacity: [0.28, 0],
          scaleX: [0.84, 0.28],
          x: [-18, -96],
        },
        at(0.66),
      );
  }

  if (spineShadow) {
    timeline
      .add(
        spineShadow,
        {
          duration: durationBetween(archiveProgress.openStart, 0.74),
          ease: "outCubic",
          opacity: [0, 0.38],
          scaleX: [0.6, 1.08],
        },
        at(archiveProgress.openStart),
      )
      .add(
        spineShadow,
        {
          duration: durationBetween(0.74, pageVisualEnd),
          ease: "inCubic",
          opacity: [0.38, 0.14],
          scaleX: [1.08, 0.82],
        },
        at(0.74),
      )
      .add(
        spineShadow,
        {
          duration: durationBetween(0.94, 1),
          ease: "inOutSine",
          opacity: [0.18, 0],
          scaleX: [0.82, 0.58],
        },
        at(0.94),
      );
  }

  if (base) {
    timeline
      .add(
        base,
        {
          duration: durationBetween(insideRevealStart, insideRevealEnd),
          ease: "outCubic",
          opacity: [0, 0.22],
          scaleX: [0.985, 1],
        },
        at(insideRevealStart),
      )
      .add(
        base,
        {
          duration: durationBetween(archiveProgress.openEnd, pageVisualEnd),
          ease: "inCubic",
          opacity: [1, 0],
        },
        at(archiveProgress.openEnd),
      );
  }

  if (inside) {
    timeline
      .add(
        inside,
        {
          duration: durationBetween(insideRevealStart, insideRevealEnd),
          ease: "outCubic",
          opacity: [0, 1],
          scaleX: [0.985, 1],
        },
        at(insideRevealStart),
      )
      .add(
        inside,
        {
          duration: durationBetween(archiveProgress.openEnd, pageVisualEnd),
          ease: "inCubic",
          opacity: [1, 0],
        },
        at(archiveProgress.openEnd),
      );
  }

  if (leftPanel) {
    timeline
      .add(
        leftPanel,
        {
          duration: durationBetween(insideRevealStart, 0.9),
          ease: "outSine",
          opacity: [0, 1],
          rotateY: [64, 0],
          scaleX: [0.9, 1],
          x: [34, 0],
          z: [-28, 0],
        },
        at(insideRevealStart),
      )
      .add(
        leftPanel,
        {
          duration: durationBetween(archiveProgress.openEnd, pageVisualEnd),
          ease: "inCubic",
          opacity: [1, 0],
          rotateY: [0, 8],
          y: [0, 12],
        },
        at(archiveProgress.openEnd),
      );
  }

  if (rightPanel) {
    timeline
      .add(
        rightPanel,
        {
          duration: durationBetween(insideRevealStart, 0.9),
          ease: "outSine",
          opacity: [0, 1],
          rotateY: [-22, 0],
          scaleX: [0.96, 1],
          x: [-12, 0],
          z: [-18, 0],
        },
        at(insideRevealStart),
      )
      .add(
        rightPanel,
        {
          duration: durationBetween(archiveProgress.openEnd, pageVisualEnd),
          ease: "inCubic",
          opacity: [1, 0],
          rotateY: [0, -5],
          y: [0, 12],
        },
        at(archiveProgress.openEnd),
      );
  }

  if (introCopy) {
    timeline.add(
      introCopy,
      {
        duration: durationBetween(0.2, archiveProgress.introEnd),
        ease: "inCubic",
        opacity: [1, 0],
      },
      at(0.2),
    );
  }

  if (closingCopy) {
    timeline.add(
      closingCopy,
      {
        duration: durationBetween(closingVisualStart, 1),
        ease: "outCubic",
        opacity: [0, 1],
        y: [24, 0],
      },
      at(closingVisualStart),
    );
  }

  timeline.seek(0, true);

  return timeline;
}

export function useHomeAnimeTimeline({ reduceMotion, readiness, stageRef }: UseHomeAnimeTimelineOptions) {
  useEffect(() => {
    syncReadiness(0, readiness, reduceMotion);

    if (reduceMotion) {
      return;
    }

    const stage = stageRef.current;

    if (!stage) {
      return;
    }

    const timeline = createArchiveTimeline(stage);
    let animationFrame: number | null = null;

    const syncTimeline = () => {
      animationFrame = null;
      const progress = getStageScrollProgress(stage);

      timeline.seek(progress * timelineDuration, true);
      syncReadiness(progress, readiness, false);
    };

    const scheduleSync = () => {
      if (animationFrame !== null) {
        return;
      }

      animationFrame = window.requestAnimationFrame(syncTimeline);
    };

    syncTimeline();
    window.addEventListener("scroll", scheduleSync, { passive: true });
    window.addEventListener("resize", scheduleSync);

    return () => {
      window.removeEventListener("scroll", scheduleSync);
      window.removeEventListener("resize", scheduleSync);

      if (animationFrame !== null) {
        window.cancelAnimationFrame(animationFrame);
      }

      timeline.revert();
    };
  }, [readiness, reduceMotion, stageRef]);
}
