"use client";

import { createTimeline } from "animejs";
import { useEffect, type Dispatch, type RefObject, type SetStateAction } from "react";
import { homeAnimSelector } from "./home-animation-selectors";

export const archiveProgress = {
  introEnd: 0.34,
  straightStart: 0.3,
  copyEnd: 0.5,
  openStart: 0.68,
  openEnd: 0.86,
  closingStart: 0.76,
} as const;

type ReadinessSetter = Dispatch<SetStateAction<boolean>>;

type ArchiveReadiness = {
  setClosingCopyVisible: ReadinessSetter;
  setContentReady: ReadinessSetter;
  setIntroReady: ReadinessSetter;
  setOpenCopyReady: ReadinessSetter;
  setPageVisible: ReadinessSetter;
};

type UseHomeAnimeTimelineOptions = {
  readiness: ArchiveReadiness;
  reduceMotion: boolean;
  stageRef: RefObject<HTMLElement | null>;
};

const timelineDuration = 1000;
const pageVisualEnd = 0.9;

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
    setReady(readiness.setOpenCopyReady, false);
    setReady(readiness.setPageVisible, true);
    setReady(readiness.setClosingCopyVisible, false);
    return;
  }

  setReady(readiness.setContentReady, progress >= archiveProgress.openStart && progress <= archiveProgress.openEnd);
  setReady(readiness.setIntroReady, progress < archiveProgress.introEnd);
  setReady(readiness.setOpenCopyReady, progress >= archiveProgress.straightStart && progress <= archiveProgress.copyEnd);
  setReady(readiness.setPageVisible, progress >= archiveProgress.openStart && progress <= pageVisualEnd);
  setReady(readiness.setClosingCopyVisible, progress >= archiveProgress.closingStart);
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
  const pages = Array.from(stage.querySelectorAll<HTMLElement>(homeAnimSelector("archivePage")));
  const introCopy = stage.querySelector<HTMLElement>(homeAnimSelector("archiveIntroCopy"));
  const openCopy = stage.querySelector<HTMLElement>(homeAnimSelector("archiveOpenCopy"));
  const closingCopy = stage.querySelector<HTMLElement>(homeAnimSelector("archiveClosingCopy"));
  const timeline = createTimeline({ autoplay: false, defaults: { ease: "linear" } });

  if (book) {
    timeline
      .add(
        book,
        {
          duration: at(archiveProgress.straightStart),
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
          duration: durationBetween(archiveProgress.openStart, archiveProgress.openEnd),
          opacity: [1, 0],
          rotateY: [0, -176],
        },
        at(archiveProgress.openStart),
      )
      .add(
        cover,
        {
          duration: durationBetween(0.94, 1),
          opacity: [0, 1],
          rotateY: [-176, 0],
        },
        at(0.94),
      );
  }

  if (base) {
    timeline
      .add(
        base,
        {
          duration: durationBetween(archiveProgress.openStart, 0.76),
          opacity: [0, 1],
        },
        at(archiveProgress.openStart),
      )
      .add(
        base,
        {
          duration: durationBetween(archiveProgress.openEnd, 0.94),
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
          duration: durationBetween(archiveProgress.openStart, 0.76),
          opacity: [0, 1],
        },
        at(archiveProgress.openStart),
      )
      .add(
        inside,
        {
          duration: durationBetween(archiveProgress.openEnd, 0.94),
          opacity: [1, 0],
        },
        at(archiveProgress.openEnd),
      );
  }

  if (pages.length > 0) {
    timeline
      .add(
        pages,
        {
          duration: durationBetween(archiveProgress.openStart, 0.76),
          opacity: [0, 1],
          y: [18, 0],
        },
        at(archiveProgress.openStart),
      )
      .add(
        pages,
        {
          duration: durationBetween(archiveProgress.openEnd, 0.9),
          opacity: [1, 0],
          y: [0, 12],
        },
        at(archiveProgress.openEnd),
      );
  }

  if (introCopy) {
    timeline.add(
      introCopy,
      {
        duration: durationBetween(0.22, archiveProgress.introEnd),
        opacity: [1, 0],
      },
      at(0.22),
    );
  }

  if (openCopy) {
    timeline
      .add(
        openCopy,
        {
          duration: durationBetween(0.28, 0.36),
          opacity: [0, 1],
        },
        at(0.28),
      )
      .add(
        openCopy,
        {
          duration: durationBetween(0.43, archiveProgress.copyEnd),
          opacity: [1, 0],
        },
        at(0.43),
      );
  }

  if (closingCopy) {
    timeline.add(
      closingCopy,
      {
        duration: durationBetween(0.76, 0.9),
        opacity: [0, 1],
        y: [18, 0],
      },
      at(0.76),
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
