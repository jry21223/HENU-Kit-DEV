import type { CSSProperties } from "react";
import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { homeAnimAttr } from "./home-animation-selectors";
import type { CourseBook } from "./home-types";
import styles from "./home-visuals.module.css";

const toneClass: Record<CourseBook["tone"], string> = {
  blue: styles.toneBlue,
  orange: styles.toneOrange,
  green: styles.toneGreen,
  pink: styles.tonePink,
  purple: styles.tonePurple,
  cyan: styles.toneCyan,
};

const tilt: Record<CourseBook["tone"], string> = {
  blue: "-3deg",
  orange: "2deg",
  green: "-1deg",
  pink: "3deg",
  purple: "-2deg",
  cyan: "1deg",
};

export function PdfCourseBook({
  animationMarked = false,
  compact = false,
  course,
  tabIndex,
}: {
  animationMarked?: boolean;
  compact?: boolean;
  course: CourseBook;
  tabIndex?: number;
}) {
  return (
    <Link
      className={`${styles.courseBook} ${compact ? styles.courseBookCompact : ""} ${toneClass[course.tone]}`}
      href={course.href}
      style={{ "--book-tilt": tilt[course.tone] } as CSSProperties}
      tabIndex={tabIndex}
      {...(animationMarked ? homeAnimAttr("courseBook") : {})}
    >
      <span
        className={styles.courseBookSpine}
        aria-hidden="true"
        {...(animationMarked ? homeAnimAttr("courseBookSpine") : {})}
      />
      <span
        className={styles.courseBookGloss}
        aria-hidden="true"
        {...(animationMarked ? homeAnimAttr("courseBookGloss") : {})}
      />
      <span className="relative z-10 flex items-center justify-between gap-3 font-mono text-xs font-semibold text-[#2b2117]/78">
        <span>{course.code}</span>
        <span>PDF</span>
      </span>
      <span className="relative z-10">
        <span className={`block font-black tracking-tight text-[#2b2117] ${compact ? "text-xl" : "text-2xl"}`}>
          {course.label}
        </span>
        <span className="mt-1 block font-mono text-xs text-[#2b2117]/70">{course.subtitle}</span>
      </span>
      <span className="relative z-10 flex items-center justify-between gap-2 text-xs font-semibold text-[#2b2117]/72">
        <span>{course.meta}</span>
        <ArrowRight className="size-4" aria-hidden="true" />
      </span>
    </Link>
  );
}
