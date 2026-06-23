import type { ComponentType } from "react";

export type HomeLink = {
  label: string;
  href: string;
};

export type ArchiveDirectoryItem = HomeLink & {
  description: string;
};

export type CourseBookTone = "blue" | "orange" | "green" | "pink" | "purple" | "cyan";

export type CourseBook = HomeLink & {
  code: string;
  subtitle: string;
  meta: string;
  tone: CourseBookTone;
};

export type VisionNote = {
  title: string;
  body: string;
  tone: "yellow" | "pink" | "blue" | "green";
  tilt: "left" | "right" | "none";
};

export type VisionFeature = {
  title: string;
  body: string;
  icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
};

export type GuaranteeItem = VisionFeature;

export type RecentUpdate = {
  title: string;
  course: string;
  href: string;
  label: string;
};
