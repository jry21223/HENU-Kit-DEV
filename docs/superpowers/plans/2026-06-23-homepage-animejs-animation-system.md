# Homepage Anime.js Animation System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the public homepage animation system around Anime.js while keeping `/workspace` as the separate static student work area.

**Architecture:** Keep the existing React component split under `apps/web/components/home`, but replace homepage Motion usage with Anime.js timeline hooks and stable `data-home-anim` selectors. React renders semantic structure, CSS defines final static states and paper/archive visuals, and Anime.js controls scroll-synced hero/archive motion plus precise staggered details.

**Tech Stack:** Next.js 16, React 19, TypeScript, Tailwind CSS, CSS Modules, Anime.js 4, Playwright visual checks.

---

## Context

Read these before executing:

- Spec: `docs/superpowers/specs/2026-06-23-homepage-animejs-animation-system-design.md`
- Existing homepage composition: `apps/web/components/home/home-page.tsx`
- Existing desktop archive animation: `apps/web/components/home/archive-book-reveal.tsx`
- Existing mobile archive intro: `apps/web/components/home/mobile-archive-intro.tsx`
- Existing visual CSS: `apps/web/components/home/home-visuals.module.css`
- Existing homepage data: `apps/web/components/home/home-data.ts`
- Existing visual test: `apps/web/tests/homepage.visual.spec.ts`
- Current pending workspace split files: `apps/web/app/workspace/page.tsx`, `apps/web/components/home/home-data.ts`, `apps/web/components/home/home-nav.tsx`

## File Structure

- Modify: `apps/web/package.json`
  - Add `animejs`.
  - Remove `motion` after all homepage imports are migrated.
- Modify: `package-lock.json`
  - Updated by dependency install/uninstall commands.
- Create: `apps/web/components/home/home-animation-selectors.ts`
  - Owns stable animation ids, selector helpers, and `data-home-anim` prop helper.
- Create: `apps/web/components/home/use-home-anime-timeline.ts`
  - Owns desktop archive scroll progress, Anime.js timeline creation, seek, cleanup, and accessibility readiness callbacks.
- Create: `apps/web/components/home/use-home-anime-in-view.ts`
  - Owns lightweight section entry animations for notes, practice cards, membership ticket, LangBot note, and guarantee seals.
- Modify: `apps/web/components/home/archive-book-reveal.tsx`
  - Remove `motion/react`.
  - Add stable `data-home-anim` markers.
  - Use `useHomeAnimeTimeline`.
  - Add scan line, page highlight, and focus readiness based on scroll progress.
- Modify: `apps/web/components/home/mobile-archive-intro.tsx`
  - Remove `motion/react`.
  - Use `useHomeAnimeInView` or CSS static reduced-motion behavior.
- Modify: `apps/web/components/home/pdf-course-book.tsx`
  - Add animation markers for course book, spine, gloss, and body groups.
- Modify: `apps/web/components/home/community-sticky-notes.tsx`
  - Add section ref and note markers for Anime.js entry.
- Modify: `apps/web/components/home/practice-vision-section.tsx`
  - Add section ref and practice mark markers.
- Modify: `apps/web/components/home/membership-ticket-section.tsx`
  - Add section ref and ticket/stamp markers.
- Modify: `apps/web/components/home/sales-assistant-note.tsx`
  - Add section ref and note markers.
- Modify: `apps/web/components/home/guarantee-section.tsx`
  - Add section ref and seal markers.
- Modify: `apps/web/components/home/home-visuals.module.css`
  - Add base static states for Anime.js targets.
  - Add scan line, page highlight, stamp, seal, and reduced-motion rules.
- Modify: `apps/web/tests/homepage.visual.spec.ts`
  - Update assertions for `/workspace` CTA.
  - Add assertions for precision animation markers and post-book sections.

## Task 1: Settle Workspace Split Before Animation Work

**Files:**

- Add: `apps/web/app/workspace/page.tsx`
- Modify: `apps/web/components/home/home-data.ts`
- Modify: `apps/web/components/home/home-nav.tsx`

- [ ] **Step 1: Confirm pending workspace split files**

Run:

```powershell
git status --short
```

Expected output includes exactly these workspace split changes before any Anime.js work:

```txt
 M apps/web/components/home/home-data.ts
 M apps/web/components/home/home-nav.tsx
?? apps/web/app/workspace/
```

- [ ] **Step 2: Verify homepage and workspace routes locally**

Run:

```powershell
$port = 3010
while (Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue) { $port++ }
$proc = Start-Process -FilePath "npx.cmd" -ArgumentList "next", "dev", "-p", "$port" -WorkingDirectory (Join-Path (Get-Location) "apps\web") -WindowStyle Hidden -PassThru
Start-Sleep -Seconds 10
$homeStatus = (Invoke-WebRequest -Uri "http://127.0.0.1:$port/" -UseBasicParsing -TimeoutSec 15).StatusCode
$workspaceStatus = (Invoke-WebRequest -Uri "http://127.0.0.1:$port/workspace" -UseBasicParsing -TimeoutSec 15).StatusCode
Stop-Process -Id $proc.Id -ErrorAction SilentlyContinue
Write-Output "home=$homeStatus workspace=$workspaceStatus"
```

Expected:

```txt
home=200 workspace=200
```

- [ ] **Step 3: Run type check**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: exits 0 and prints `tsc --noEmit`.

- [ ] **Step 4: Commit only the workspace split**

Run:

```powershell
git add apps/web/app/workspace/page.tsx apps/web/components/home/home-data.ts apps/web/components/home/home-nav.tsx
git commit -m "feat(web): restore student workspace route"
```

Expected: commit succeeds and `git status --short` no longer lists those three files.

## Task 2: Install Anime.js and Verify Public API

**Files:**

- Modify: `apps/web/package.json`
- Modify: `package-lock.json`

- [ ] **Step 1: Install Anime.js in the web workspace**

Run:

```powershell
npm install --workspace @final-review/web animejs
```

Expected: command exits 0 and `apps/web/package.json` contains an `animejs` dependency.

- [ ] **Step 2: Verify required Anime.js exports**

Run:

```powershell
node -e "import('animejs').then((m) => { for (const key of ['animate','createTimeline','stagger']) { if (typeof m[key] === 'undefined') throw new Error('Missing export: ' + key); } console.log('animejs exports ok'); })"
```

Expected:

```txt
animejs exports ok
```

- [ ] **Step 3: Run type check**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: exits 0.

- [ ] **Step 4: Commit dependency install**

Run:

```powershell
git add apps/web/package.json package-lock.json
git commit -m "feat(web): add Anime.js for homepage animation"
```

Expected: commit succeeds with package files only.

## Task 3: Add Animation Selectors and First Failing Tests

**Files:**

- Create: `apps/web/components/home/home-animation-selectors.ts`
- Modify: `apps/web/tests/homepage.visual.spec.ts`

- [ ] **Step 1: Create animation selector helper**

Create `apps/web/components/home/home-animation-selectors.ts` with:

```ts
export const homeAnim = {
  archiveBook: "archive-book",
  archiveCover: "archive-cover",
  archiveInside: "archive-inside",
  archiveBase: "archive-base",
  archiveSpine: "archive-spine",
  archivePage: "archive-page",
  archiveDirectoryLine: "archive-directory-line",
  archiveDirectoryScan: "archive-directory-scan",
  archivePageHighlight: "archive-page-highlight",
  archiveIntroCopy: "archive-intro-copy",
  archiveOpenCopy: "archive-open-copy",
  archiveClosingCopy: "archive-closing-copy",
  courseBook: "course-book",
  courseBookSpine: "course-book-spine",
  courseBookGloss: "course-book-gloss",
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
```

- [ ] **Step 2: Extend Playwright test with failing selector assertions**

Modify `apps/web/tests/homepage.visual.spec.ts` by adding this test after `homepage renders product vision on desktop`:

```ts
test("homepage exposes precision animation markers", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await expect(page.locator('[data-home-anim="archive-book"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-cover"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-directory-scan"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-directory-line"]')).toHaveCount(6);
  await expect(page.locator('[data-home-anim="course-book"]')).toHaveCount(6);
  await expect(page.locator('[data-home-anim="community-note"]')).toHaveCount(4);
  await expect(page.locator('[data-home-anim="practice-card"]')).toHaveCount(4);
  await expect(page.locator('[data-home-anim="membership-stamp"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="sales-note"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="guarantee-seal"]')).toHaveCount(4);
});
```

- [ ] **Step 3: Update CTA assertions for `/workspace`**

In `homepage renders product vision on desktop`, add this after the heading assertion:

```ts
  await expect(page.getByRole("link", { name: /进入工作区/ })).toHaveAttribute("href", "/workspace");
  await expect(page.getByRole("link", { name: /浏览课程资料/ })).toHaveAttribute("href", "/courses");
```

- [ ] **Step 4: Run the focused visual test and confirm it fails for missing markers**

Run with a local dev server already serving the current branch:

```powershell
$env:HOME_URL="http://127.0.0.1:3010/"
npx playwright test apps/web/tests/homepage.visual.spec.ts -g "precision animation markers"
```

Expected: fails because `[data-home-anim="archive-book"]` and related markers are not present yet.

- [ ] **Step 5: Commit failing test and selector helper**

Run:

```powershell
git add apps/web/components/home/home-animation-selectors.ts apps/web/tests/homepage.visual.spec.ts
git commit -m "test(web): specify homepage animation markers"
```

Expected: commit succeeds. This commit intentionally contains a failing visual marker test.

## Task 4: Add Static Animation Markup and CSS States

**Files:**

- Modify: `apps/web/components/home/archive-book-reveal.tsx`
- Modify: `apps/web/components/home/pdf-course-book.tsx`
- Modify: `apps/web/components/home/community-sticky-notes.tsx`
- Modify: `apps/web/components/home/practice-vision-section.tsx`
- Modify: `apps/web/components/home/membership-ticket-section.tsx`
- Modify: `apps/web/components/home/sales-assistant-note.tsx`
- Modify: `apps/web/components/home/guarantee-section.tsx`
- Modify: `apps/web/components/home/home-visuals.module.css`

- [ ] **Step 1: Import animation helpers where markers are needed**

Add this import to each component listed in this task that renders animation targets:

```ts
import { homeAnimAttr } from "./home-animation-selectors";
```

If a file already imports only data and styles, place the new import after local data imports and before CSS module imports.

- [ ] **Step 2: Mark desktop archive targets**

In `apps/web/components/home/archive-book-reveal.tsx`, add attributes to the matching elements:

```tsx
<section ref={ref} className={styles.bookStage} aria-label="课程资料档案册">
```

stays unchanged.

The intro copy panel becomes:

```tsx
<div
  className={styles.bookCopyPanel}
  data-testid="archive-copy-intro"
  {...homeAnimAttr("archiveIntroCopy")}
  aria-hidden={introAriaHidden}
>
```

The open copy panel becomes:

```tsx
<div
  className={styles.bookCopyPanel}
  data-testid="archive-copy-open"
  {...homeAnimAttr("archiveOpenCopy")}
  aria-hidden="true"
>
```

The closing copy panel becomes:

```tsx
<div
  className={styles.bookCopyPanel}
  data-testid="archive-copy-closing"
  {...homeAnimAttr("archiveClosingCopy")}
  aria-hidden="true"
>
```

The archive book container becomes:

```tsx
<div className={styles.archiveBook} data-testid="archive-book" {...homeAnimAttr("archiveBook")}>
```

The base, inside, cover, spine, and pages get:

```tsx
<div className={styles.bookBase} {...homeAnimAttr("archiveBase")} aria-hidden="true" />
<div className={styles.bookInside} {...homeAnimAttr("archiveInside")}>
<div className={`${styles.bookPage} ${styles.directoryPage} p-4 xl:p-5`} {...homeAnimAttr("archivePage")}>
<div className={`${styles.bookPage} ${styles.coursePage} p-4`} {...homeAnimAttr("archivePage")}>
<div className={styles.bookCover} data-testid="archive-cover" {...homeAnimAttr("archiveCover")} aria-hidden="true">
<div className={styles.bookSpine} data-testid="archive-seam" {...homeAnimAttr("archiveSpine")} aria-hidden="true" />
```

Add a scan line and page highlight inside the directory page before the `资料档案` eyebrow:

```tsx
<span className={styles.directoryScan} {...homeAnimAttr("archiveDirectoryScan")} aria-hidden="true" />
<span className={styles.pageHighlight} {...homeAnimAttr("archivePageHighlight")} aria-hidden="true" />
```

Each directory link gets:

```tsx
<Link
  key={item.label}
  className={`${styles.directoryLine} group block py-1.5`}
  href={item.href}
  tabIndex={contentTabIndex}
  {...homeAnimAttr("archiveDirectoryLine")}
>
```

- [ ] **Step 3: Mark PDF course book internals**

In `apps/web/components/home/pdf-course-book.tsx`, import `homeAnimAttr` and change the root link to:

```tsx
<Link
  className={`${styles.courseBook} ${compact ? styles.courseBookCompact : ""} ${toneClass[course.tone]}`}
  href={course.href}
  style={{ "--book-tilt": tilt[course.tone] } as CSSProperties}
  tabIndex={tabIndex}
  {...homeAnimAttr("courseBook")}
>
```

Add these two decorative spans as the first children inside the link:

```tsx
<span className={styles.courseBookSpine} {...homeAnimAttr("courseBookSpine")} aria-hidden="true" />
<span className={styles.courseBookGloss} {...homeAnimAttr("courseBookGloss")} aria-hidden="true" />
```

- [ ] **Step 4: Mark post-book sections**

In `community-sticky-notes.tsx`, each note article becomes:

```tsx
<article
  key={note.title}
  className={`${styles.stickyNote} ${toneClass[note.tone]}`}
  style={noteStyle}
  {...homeAnimAttr("communityNote")}
>
```

In `practice-vision-section.tsx`, each card article becomes:

```tsx
<article key={feature.title} className={`${styles.paperCard} p-5`} {...homeAnimAttr("practiceCard")}>
  <span className={styles.practiceMark} {...homeAnimAttr("practiceMark")} aria-hidden="true" />
```

In `membership-ticket-section.tsx`, add to the ticket wrapper:

```tsx
<div className={`${styles.ticket} p-6 lg:p-10`} {...homeAnimAttr("membershipTicket")}>
  <span className={styles.membershipStamp} {...homeAnimAttr("membershipStamp")} aria-hidden="true">PASS</span>
```

In `sales-assistant-note.tsx`, add to the colored note wrapper:

```tsx
<div className="rotate-[-1deg] rounded-[2rem] border border-[#2b2117]/12 bg-[#d8f1ff] p-6 shadow-[0_22px_64px_rgba(71,49,27,0.12)] md:p-8" {...homeAnimAttr("salesNote")}>
```

In `guarantee-section.tsx`, add this span as the first child inside each guarantee article:

```tsx
<span className={styles.guaranteeSeal} {...homeAnimAttr("guaranteeSeal")} aria-hidden="true" />
```

- [ ] **Step 5: Add CSS for new static targets**

Append to `apps/web/components/home/home-visuals.module.css`:

```css
.directoryScan,
.pageHighlight,
.courseBookSpine,
.courseBookGloss,
.practiceMark,
.membershipStamp,
.guaranteeSeal {
  pointer-events: none;
}

.directoryScan {
  position: absolute;
  right: 18px;
  left: 18px;
  top: 18px;
  z-index: 3;
  height: 2px;
  border-radius: 999px;
  background: linear-gradient(90deg, transparent, rgb(47 107 88 / 0.7), transparent);
  opacity: 0;
  transform: translateY(0);
}

.pageHighlight {
  position: absolute;
  inset: 12px;
  z-index: 1;
  border-radius: 22px;
  background: radial-gradient(circle at 28% 12%, rgb(47 107 88 / 0.14), transparent 38%);
  opacity: 0;
}

.bookPage > *:not(.directoryScan):not(.pageHighlight) {
  position: relative;
  z-index: 2;
}

.courseBookSpine {
  position: absolute;
  inset: 0 auto 0 0;
  z-index: 1;
  width: 18px;
  background: rgb(43 33 23 / 0.14);
}

.courseBookGloss {
  position: absolute;
  inset: 10px 10px 10px auto;
  z-index: 1;
  width: 10px;
  border-radius: 999px;
  background: rgb(255 255 255 / 0.44);
}

.practiceMark {
  position: absolute;
  right: 18px;
  top: 18px;
  width: 30px;
  height: 18px;
  border-right: 3px solid rgb(47 107 88 / 0.72);
  border-bottom: 3px solid rgb(47 107 88 / 0.72);
  opacity: 0;
  transform: rotate(38deg) scale(0.72);
}

.membershipStamp {
  position: absolute;
  right: 28px;
  top: 24px;
  z-index: 2;
  display: grid;
  width: 78px;
  height: 78px;
  place-items: center;
  border: 2px solid rgb(183 92 50 / 0.68);
  border-radius: 999px;
  color: rgb(183 92 50 / 0.78);
  font-family: var(--font-geist-mono), ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  font-weight: 800;
  letter-spacing: 0.16em;
  opacity: 0;
  transform: rotate(-12deg) scale(0.75);
}

.guaranteeSeal {
  position: absolute;
  right: 16px;
  top: 16px;
  width: 34px;
  height: 34px;
  border: 2px solid rgb(47 107 88 / 0.45);
  border-radius: 999px;
  opacity: 0;
  transform: scale(0.72);
}

.guaranteeSeal::before {
  content: "";
  position: absolute;
  inset: 8px;
  border-radius: inherit;
  background: rgb(47 107 88 / 0.2);
}
```

Also replace the old `.courseBook::before` and `.courseBook::after` rules with comments that preserve the previous visual through real spans:

```css
.courseBook::before,
.courseBook::after {
  content: none;
}
```

Add `position: relative; overflow: hidden;` to `.paperCard` and guarantee articles by adding a reusable class target:

```css
.paperCard {
  position: relative;
  overflow: hidden;
}
```

- [ ] **Step 6: Run marker test**

Run:

```powershell
$env:HOME_URL="http://127.0.0.1:3010/"
npx playwright test apps/web/tests/homepage.visual.spec.ts -g "precision animation markers"
```

Expected: passes.

- [ ] **Step 7: Run type check and commit**

Run:

```powershell
npm --workspace @final-review/web run lint
git add apps/web/components/home apps/web/tests/homepage.visual.spec.ts
git commit -m "feat(web): add homepage animation targets"
```

Expected: type check exits 0 and commit succeeds.

## Task 5: Replace Desktop Motion Logic with Anime.js Timeline

**Files:**

- Create: `apps/web/components/home/use-home-anime-timeline.ts`
- Modify: `apps/web/components/home/archive-book-reveal.tsx`
- Modify: `apps/web/components/home/home-visuals.module.css`

- [ ] **Step 1: Create desktop timeline hook**

Create `apps/web/components/home/use-home-anime-timeline.ts`:

```ts
"use client";

import { createTimeline, stagger } from "animejs";
import { RefObject, useEffect } from "react";
import { homeAnimSelector } from "./home-animation-selectors";

export const archiveProgress = {
  introEnd: 0.34,
  straightStart: 0.3,
  copyEnd: 0.5,
  openStart: 0.68,
  openEnd: 0.86,
  closingStart: 0.84,
} as const;

type ArchiveReadiness = {
  setContentReady: (ready: boolean) => void;
  setIntroReady: (ready: boolean) => void;
  setOpenCopyReady: (ready: boolean) => void;
  setClosingCopyReady: (ready: boolean) => void;
};

type TimelineHandle = {
  add: (...args: unknown[]) => TimelineHandle;
  seek: (time: number) => unknown;
  pause: () => unknown;
  revert: () => unknown;
  duration: number;
};

function clamp01(value: number) {
  return Math.min(Math.max(value, 0), 1);
}

function readArchiveProgress(stage: HTMLElement) {
  const stageTop = window.scrollY + stage.getBoundingClientRect().top;
  const scrollableDistance = Math.max(stage.offsetHeight - window.innerHeight, 1);
  return clamp01((window.scrollY - stageTop) / scrollableDistance);
}

function setReadyState(progress: number, readiness: ArchiveReadiness) {
  readiness.setContentReady(progress >= archiveProgress.openStart && progress <= archiveProgress.openEnd);
  readiness.setIntroReady(progress < archiveProgress.introEnd);
  readiness.setOpenCopyReady(progress >= archiveProgress.straightStart && progress <= archiveProgress.copyEnd);
  readiness.setClosingCopyReady(progress >= archiveProgress.closingStart);
}

export function useHomeAnimeTimeline({
  reduceMotion,
  readiness,
  stageRef,
}: {
  reduceMotion: boolean;
  readiness: ArchiveReadiness;
  stageRef: RefObject<HTMLElement | null>;
}) {
  useEffect(() => {
    const stage = stageRef.current;

    if (!stage || reduceMotion) {
      readiness.setContentReady(true);
      readiness.setIntroReady(true);
      readiness.setOpenCopyReady(false);
      readiness.setClosingCopyReady(false);
      return;
    }

    const q = (name: Parameters<typeof homeAnimSelector>[0]) => stage.querySelectorAll(homeAnimSelector(name));

    const timeline = createTimeline({
      autoplay: false,
      defaults: { ease: "inOutCubic" },
    }) as unknown as TimelineHandle;

    timeline
      .add(q("archiveBook"), { x: [300, 0, 0, 0, 0, 0], y: [220, 0, 0, 0, 0, 18], rotate: [-12, 0, 0, 0, 0, 2] }, 0)
      .add(q("archiveIntroCopy"), { opacity: [1, 1, 0], translateY: [0, 0, -18] }, 0)
      .add(q("archiveOpenCopy"), { opacity: [0, 1, 1, 0], translateY: [12, 0, 0, -10] }, 300)
      .add(q("archiveCover"), { rotateY: [0, 0, -176, -176, 0], opacity: [1, 1, 0, 0, 1] }, 680)
      .add(q("archiveBase"), { opacity: [0, 1, 1, 0] }, 680)
      .add(q("archiveInside"), { opacity: [0, 1, 1, 0] }, 680)
      .add(q("archivePage"), { opacity: [0, 1, 1, 0], translateY: [18, 0, 0, 12] }, 700)
      .add(q("archivePageHighlight"), { opacity: [0, 1, 0.7, 0] }, 720)
      .add(q("archiveDirectoryScan"), { opacity: [0, 1, 1, 0], translateY: [0, 190, 260, 300] }, 735)
      .add(q("archiveDirectoryLine"), { opacity: [0, 1], translateX: [-12, 0] }, 755)
      .add(q("courseBook"), { opacity: [0, 1], translateY: [18, 0], scale: [0.92, 1], rotate: stagger([-4, 3]) }, 770)
      .add(q("archiveClosingCopy"), { opacity: [0, 1, 1], translateY: [18, 0, 0] }, 850);

    const update = () => {
      const progress = readArchiveProgress(stage);
      setReadyState(progress, readiness);
      timeline.seek(progress * timeline.duration);
    };

    let frame = 0;
    const requestUpdate = () => {
      window.cancelAnimationFrame(frame);
      frame = window.requestAnimationFrame(update);
    };

    update();
    window.addEventListener("scroll", requestUpdate, { passive: true });
    window.addEventListener("resize", requestUpdate);

    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener("scroll", requestUpdate);
      window.removeEventListener("resize", requestUpdate);
      timeline.pause();
      timeline.revert();
    };
  }, [readiness, reduceMotion, stageRef]);
}
```

- [ ] **Step 2: Remove Motion imports from desktop archive component**

In `apps/web/components/home/archive-book-reveal.tsx`, replace:

```ts
import { motion, useMotionValueEvent, useScroll, useTransform } from "motion/react";
import { useRef, useState } from "react";
```

with:

```ts
import { useMemo, useRef, useState } from "react";
import { archiveProgress, useHomeAnimeTimeline } from "./use-home-anime-timeline";
```

- [ ] **Step 3: Replace Motion state initialization**

Replace the progress constants in `archive-book-reveal.tsx` with usage from the hook:

```ts
const INTRO_END = archiveProgress.introEnd;
const STRAIGHT_START = archiveProgress.straightStart;
const COPY_END = archiveProgress.copyEnd;
const OPEN_START = archiveProgress.openStart;
const OPEN_END = archiveProgress.openEnd;
```

Remove all `useScroll`, `useTransform`, and `useMotionValueEvent` code.

Add this readiness object before `contentFocusable`:

```ts
  const readiness = useMemo(
    () => ({
      setContentReady,
      setIntroReady,
      setOpenCopyReady,
      setClosingCopyReady,
    }),
    [],
  );

  useHomeAnimeTimeline({
    reduceMotion,
    readiness,
    stageRef: ref,
  });
```

- [ ] **Step 4: Convert all `motion.div` elements to `div`**

In `archive-book-reveal.tsx`, replace every `<motion.div` with `<div` and every `</motion.div>` with `</div>`.

Remove inline `style` props that previously referenced Motion values:

```tsx
style={{
  opacity: reduceMotion ? 1 : introOpacity,
  pointerEvents: introFocusable ? "auto" : "none",
  visibility: introFocusable ? "visible" : "hidden",
}}
```

becomes:

```tsx
style={{
  pointerEvents: introFocusable ? "auto" : "none",
  visibility: introFocusable ? "visible" : "hidden",
}}
```

Apply the same pattern for open copy, closing copy, archive book, book base, book inside, pages, and cover: keep accessibility-related `pointerEvents` and `visibility`; remove animated Motion value references.

- [ ] **Step 5: Add CSS default animation states**

Append to `home-visuals.module.css`:

```css
.bookCopyPanel[data-home-anim="archive-open-copy"],
.bookCopyPanel[data-home-anim="archive-closing-copy"] {
  opacity: 0;
}

.bookBase,
.bookInside,
.bookPage,
.pageHighlight,
.directoryLine,
.courseBook {
  will-change: opacity, transform;
}

.bookCover {
  will-change: opacity, transform;
}

@media (prefers-reduced-motion: reduce) {
  .bookCopyPanel,
  .bookBase,
  .bookInside,
  .bookPage,
  .directoryLine,
  .courseBook {
    opacity: 1 !important;
    transform: none !important;
  }

  .bookCover {
    opacity: 0 !important;
    transform: rotateY(-176deg) !important;
  }
}
```

- [ ] **Step 6: Run focused desktop visual test**

Run:

```powershell
$env:HOME_URL="http://127.0.0.1:3010/"
npx playwright test apps/web/tests/homepage.visual.spec.ts -g "product vision on desktop"
```

Expected: desktop test passes, including the states at 0, 0.6, 0.8, and 0.93 scroll progress.

- [ ] **Step 7: Run type check and commit**

Run:

```powershell
npm --workspace @final-review/web run lint
git add apps/web/components/home/archive-book-reveal.tsx apps/web/components/home/use-home-anime-timeline.ts apps/web/components/home/home-visuals.module.css
git commit -m "feat(web): drive archive reveal with Anime.js"
```

Expected: type check exits 0 and commit succeeds.

## Task 6: Replace Mobile Motion and Add In-View Section Animations

**Files:**

- Create: `apps/web/components/home/use-home-anime-in-view.ts`
- Modify: `apps/web/components/home/mobile-archive-intro.tsx`
- Modify: `apps/web/components/home/community-sticky-notes.tsx`
- Modify: `apps/web/components/home/practice-vision-section.tsx`
- Modify: `apps/web/components/home/membership-ticket-section.tsx`
- Modify: `apps/web/components/home/sales-assistant-note.tsx`
- Modify: `apps/web/components/home/guarantee-section.tsx`
- Modify: `apps/web/components/home/home-visuals.module.css`

- [ ] **Step 1: Create in-view animation hook**

Create `apps/web/components/home/use-home-anime-in-view.ts`:

```ts
"use client";

import { animate, stagger } from "animejs";
import { RefObject, useEffect } from "react";

export function useHomeAnimeInView({
  reduceMotion,
  rootRef,
  selector,
}: {
  reduceMotion: boolean;
  rootRef: RefObject<HTMLElement | null>;
  selector: string;
}) {
  useEffect(() => {
    const root = rootRef.current;

    if (!root || reduceMotion) {
      return;
    }

    const targets = root.querySelectorAll(selector);

    if (targets.length === 0) {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) {
            continue;
          }

          animate(targets, {
            opacity: [0, 1],
            translateY: [18, 0],
            rotate: [-2, 0],
            scale: [0.96, 1],
            delay: stagger(70),
            duration: 560,
            ease: "outCubic",
          });

          observer.disconnect();
        }
      },
      { rootMargin: "0px 0px -18% 0px", threshold: 0.18 },
    );

    observer.observe(root);

    return () => {
      observer.disconnect();
    };
  }, [reduceMotion, rootRef, selector]);
}
```

- [ ] **Step 2: Remove Motion from mobile intro**

In `mobile-archive-intro.tsx`, replace:

```ts
import { motion } from "motion/react";
```

with:

```ts
import { useRef } from "react";
import { homeAnimSelector } from "./home-animation-selectors";
import { useHomeAnimeInView } from "./use-home-anime-in-view";
```

Add inside `MobileArchiveIntro`:

```ts
  const sectionRef = useRef<HTMLElement>(null);

  useHomeAnimeInView({
    reduceMotion,
    rootRef: sectionRef,
    selector: homeAnimSelector("courseBook"),
  });
```

Change the section to:

```tsx
<section
  ref={sectionRef}
  aria-labelledby="mobile-archive-title"
  className={`${styles.mobileArchive} mx-auto w-[min(720px,calc(100%-32px))] pb-16`}
>
```

Replace both `<motion.div>` wrappers with plain `<div>` wrappers and remove `initial`, `transition`, `viewport`, and `whileInView` props.

- [ ] **Step 3: Add section refs and hook calls to post-book sections**

In each post-book section file, add:

```ts
"use client";

import { useRef } from "react";
import { homeAnimSelector } from "./home-animation-selectors";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";
import { useHomeAnimeInView } from "./use-home-anime-in-view";
```

Then inside each component add the matching ref and hook:

```ts
  const sectionRef = useRef<HTMLElement>(null);
  const reduceMotion = usePrefersReducedMotion();

  useHomeAnimeInView({
    reduceMotion,
    rootRef: sectionRef,
    selector: homeAnimSelector("communityNote"),
  });
```

Use these selectors per component:

```txt
CommunityStickyNotes -> communityNote
PracticeVisionSection -> practiceCard
MembershipTicketSection -> membershipTicket
SalesAssistantNote -> salesNote
GuaranteeSection -> guaranteeSeal
```

Add `ref={sectionRef}` to the top-level `<section>` in each component.

- [ ] **Step 4: Add CSS initial states for in-view targets**

Append to `home-visuals.module.css`:

```css
[data-home-anim="community-note"],
[data-home-anim="practice-card"],
[data-home-anim="membership-ticket"],
[data-home-anim="sales-note"],
[data-home-anim="guarantee-seal"] {
  opacity: 0;
  will-change: opacity, transform;
}

@media (prefers-reduced-motion: reduce) {
  [data-home-anim="community-note"],
  [data-home-anim="practice-card"],
  [data-home-anim="membership-ticket"],
  [data-home-anim="sales-note"],
  [data-home-anim="guarantee-seal"] {
    opacity: 1 !important;
    transform: none !important;
  }
}
```

- [ ] **Step 5: Run mobile and marker tests**

Run:

```powershell
$env:HOME_URL="http://127.0.0.1:3010/"
npx playwright test apps/web/tests/homepage.visual.spec.ts -g "mobile|precision animation markers"
```

Expected: both matching tests pass.

- [ ] **Step 6: Run type check and commit**

Run:

```powershell
npm --workspace @final-review/web run lint
git add apps/web/components/home
git commit -m "feat(web): animate homepage sections with Anime.js"
```

Expected: type check exits 0 and commit succeeds.

## Task 7: Remove Motion Dependency

**Files:**

- Modify: `apps/web/package.json`
- Modify: `package-lock.json`

- [ ] **Step 1: Verify no Motion imports remain**

Run:

```powershell
rg -n "motion/react|<motion|useScroll|useTransform|useMotionValueEvent" apps/web
```

Expected: exits 1 with no matches.

- [ ] **Step 2: Uninstall Motion from web workspace**

Run:

```powershell
npm uninstall --workspace @final-review/web motion
```

Expected: command exits 0 and `apps/web/package.json` no longer contains `"motion"`.

- [ ] **Step 3: Run type check and build**

Run:

```powershell
npm --workspace @final-review/web run lint
npm --workspace @final-review/web run build
```

Expected: both commands exit 0.

- [ ] **Step 4: Commit dependency cleanup**

Run:

```powershell
git add apps/web/package.json package-lock.json
git commit -m "chore(web): remove unused Motion dependency"
```

Expected: commit succeeds with package files only.

## Task 8: Expand Visual Verification

**Files:**

- Modify: `apps/web/tests/homepage.visual.spec.ts`

- [ ] **Step 1: Add post-book section visibility test**

Add this test to `homepage.visual.spec.ts`:

```ts
test("homepage reveals product sections after archive", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await page.getByRole("heading", { name: "资料会继续长出来" }).scrollIntoViewIfNeeded();
  await expect(page.getByRole("heading", { name: "资料会继续长出来" })).toBeVisible();
  await expect(page.locator('[data-home-anim="community-note"]')).toHaveCount(4);

  await page.getByRole("heading", { name: "资料旁边就是练习" }).scrollIntoViewIfNeeded();
  await expect(page.locator('[data-home-anim="practice-card"]')).toHaveCount(4);

  await page.getByRole("heading", { name: "贡献、权益和成本控制" }).scrollIntoViewIfNeeded();
  await expect(page.locator('[data-home-anim="membership-stamp"]')).toHaveCount(1);

  await page.getByRole("heading", { name: "资料保障要讲清楚" }).scrollIntoViewIfNeeded();
  await expect(page.locator('[data-home-anim="guarantee-seal"]')).toHaveCount(4);
});
```

- [ ] **Step 2: Run complete homepage visual spec**

Run:

```powershell
$env:HOME_URL="http://127.0.0.1:3010/"
npx playwright test apps/web/tests/homepage.visual.spec.ts
```

Expected: all tests in `homepage.visual.spec.ts` pass.

- [ ] **Step 3: Capture desktop and mobile screenshots**

Run:

```powershell
New-Item -ItemType Directory -Force artifacts | Out-Null
npx playwright screenshot --channel chrome --viewport-size=1440,1100 http://127.0.0.1:3010/ .\artifacts\homepage-animejs-desktop.png
npx playwright screenshot --channel chrome --viewport-size=390,900 http://127.0.0.1:3010/ .\artifacts\homepage-animejs-mobile.png
```

Expected:

```txt
artifacts\homepage-animejs-desktop.png
artifacts\homepage-animejs-mobile.png
```

Both screenshots show nonblank homepage states and visible CTAs.

- [ ] **Step 4: Commit visual verification**

Run:

```powershell
git add apps/web/tests/homepage.visual.spec.ts
git commit -m "test(web): verify Anime.js homepage animation states"
```

Expected: commit succeeds with the Playwright test file.

## Task 9: Final Verification

**Files:**

- No source edits expected.

- [ ] **Step 1: Run type check**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: exits 0.

- [ ] **Step 2: Run production build**

Run:

```powershell
npm --workspace @final-review/web run build
```

Expected: exits 0 and route output includes `/` and `/workspace`.

- [ ] **Step 3: Run full homepage visual test**

Run:

```powershell
$env:HOME_URL="http://127.0.0.1:3010/"
npx playwright test apps/web/tests/homepage.visual.spec.ts
```

Expected: all homepage visual tests pass.

- [ ] **Step 4: Confirm dependency boundary**

Run:

```powershell
rg -n "motion/react|<motion|useScroll|useTransform|useMotionValueEvent" apps/web
rg -n '"motion"' apps/web/package.json
rg -n '"animejs"' apps/web/package.json
```

Expected:

```txt
apps/web/package.json contains "animejs"
no motion/react or "motion" dependency matches
```

- [ ] **Step 5: Confirm worktree state**

Run:

```powershell
git status --short --branch
```

Expected: branch shows no uncommitted source changes unless generated screenshots under `artifacts/` are intentionally ignored or unstaged.

## Self-Review

- Spec coverage:
  - Anime.js as homepage animation system: Tasks 2, 5, 6, 7.
  - Stable animation selectors: Tasks 3 and 4.
  - Desktop sequence of露角、摆正、退文案、翻开、目录扫描、PDF 入位: Tasks 5 and 8.
  - Mobile simplified animation: Task 6.
  - Reduced motion content availability: Task 5 preserves static CSS rules and existing reduced-motion Playwright test remains active.
  - Workspace route separation: Task 1.
  - Workspace does not inherit homepage animation: Task 1 keeps `/workspace` separate and Tasks 2-8 only touch `components/home`.
- Placeholder scan: no unresolved work markers or unresolved file paths are present in this plan.
- Type consistency:
  - `homeAnim`, `homeAnimAttr`, and `homeAnimSelector` are defined in Task 3 and reused in Tasks 4-6.
  - `archiveProgress` is defined in Task 5 and reused in `ArchiveBookReveal`.
  - Playwright selectors match the `data-home-anim` values defined in Task 3.
