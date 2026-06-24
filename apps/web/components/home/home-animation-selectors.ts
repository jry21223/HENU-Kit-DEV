export const homeAnim = {
  archiveBook: "archive-book",
  archiveCover: "archive-cover",
  archiveCoverFront: "archive-cover-front",
  archiveCoverShadow: "archive-cover-shadow",
  archiveInside: "archive-inside",
  archiveLeftPanel: "archive-left-panel",
  archiveRightPanel: "archive-right-panel",
  archiveBase: "archive-base",
  archiveSpine: "archive-spine",
  archiveSpineShadow: "archive-spine-shadow",
  archivePage: "archive-page",
  archiveDirectoryLine: "archive-directory-line",
  archiveDirectoryScan: "archive-directory-scan",
  archivePageHighlight: "archive-page-highlight",
  archiveIntroCopy: "archive-intro-copy",
  archiveClosingCopy: "archive-closing-copy",
  courseBook: "course-book",
  courseBookSpine: "course-book-spine",
  courseBookGloss: "course-book-gloss",
  mobileCourseBook: "mobile-course-book",
  communityNote: "community-note",
  practiceCard: "practice-card",
  practiceMark: "practice-mark",
  membershipTicket: "membership-ticket",
  membershipStamp: "membership-stamp",
  salesNote: "sales-note",
  guaranteeSeal: "guarantee-seal",
} as const;

export type HomeAnimName = keyof typeof homeAnim;

export function homeAnimAttr(name: HomeAnimName) {
  return { "data-home-anim": homeAnim[name] };
}

export function homeAnimSelector(name: HomeAnimName) {
  return `[data-home-anim="${homeAnim[name]}"]`;
}
