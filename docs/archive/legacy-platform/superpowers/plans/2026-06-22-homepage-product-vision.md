# Homepage Product Vision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the approved product-vision homepage for `apps/web`, with a SuperrBook-inspired scroll reveal that turns a course-material archive book into a full learning-platform story.

**Architecture:** Replace the current single-file dashboard-style homepage with small, focused home components. Keep static narrative sections as server-rendered React where possible, and isolate scroll-driven animation inside client components using `motion/react`. Use CSS modules for complex paper/book visuals so `globals.css` stays focused on tokens and page-wide base styling.

**Tech Stack:** Next.js 16, React 19, Tailwind CSS 3, CSS Modules, `motion/react`, existing `lucide-react`, Playwright for visual QA.

---

## Context

Read these before executing any task:

- Spec: `docs/superpowers/specs/2026-06-22-homepage-product-vision-design.md`
- Existing homepage: `apps/web/app/page.tsx`
- Existing global styles: `apps/web/app/globals.css`
- Existing shell patterns: `apps/web/components/layout/site-shell.tsx`
- Existing UI helpers: `apps/web/components/ui/button.tsx`, `apps/web/components/ui/button-link.tsx`, `apps/web/lib/utils.ts`
- Reference URL: `https://www.superr.ai/?ref=lapaninja`

The reference should guide interaction and visual rhythm: large whitespace, physical notebook object, scroll-driven straighten/open interaction, sticky notes, colorful notebooks. Do not copy its children-focused wording or exact visual assets.

## Target File Structure

- Modify: `apps/web/package.json`
  - Add `motion` dependency.
  - Add `@playwright/test` dev dependency for visual QA.
- Modify: `package-lock.json`
  - Updated by npm install commands.
- Replace: `apps/web/app/page.tsx`
  - Server component composition only.
- Modify: `apps/web/app/globals.css`
  - Homepage background and typography token refinements only.
- Create: `apps/web/components/home/home-types.ts`
  - Shared types for home page data.
- Create: `apps/web/components/home/home-data.ts`
  - Product vision data arrays and route targets.
- Create: `apps/web/components/home/home-page.tsx`
  - Main page composition.
- Create: `apps/web/components/home/home-nav.tsx`
  - Lightweight homepage navigation.
- Create: `apps/web/components/home/hero-intro.tsx`
  - First viewport copy, search affordance, CTAs.
- Create: `apps/web/components/home/archive-book-reveal.tsx`
  - Desktop scroll-driven archive-book animation. Client component.
- Create: `apps/web/components/home/mobile-archive-intro.tsx`
  - Mobile simplified archive-book intro. Client component only if needed.
- Create: `apps/web/components/home/pdf-course-book.tsx`
  - Reusable colorful 2.5D PDF course book link.
- Create: `apps/web/components/home/community-sticky-notes.tsx`
  - Sticky-note community and co-creation section.
- Create: `apps/web/components/home/practice-vision-section.tsx`
  - Practice, wrong-book, weakness stats, AI support section.
- Create: `apps/web/components/home/membership-ticket-section.tsx`
  - Points, membership, course packages, AI cost-control section.
- Create: `apps/web/components/home/sales-assistant-note.tsx`
  - LangBot consultation entry.
- Create: `apps/web/components/home/guarantee-section.tsx`
  - Material guarantee and safety boundary section.
- Create: `apps/web/components/home/final-home-cta.tsx`
  - Final course-library CTA and recent-update links.
- Create: `apps/web/components/home/home-visuals.module.css`
  - Archive book, PDF books, sticky notes, tickets, paper texture, reduced-motion helpers.
- Create: `apps/web/tests/homepage.visual.spec.ts`
  - Playwright desktop/mobile smoke and screenshot assertions.

## Task 1: Dependencies and Home Data Contract

**Files:**

- Modify: `apps/web/package.json`
- Modify: `package-lock.json`
- Create: `apps/web/components/home/home-types.ts`
- Create: `apps/web/components/home/home-data.ts`

- [ ] **Step 1: Install runtime animation dependency**

Run:

```powershell
npm install --workspace @final-review/web motion
```

Expected: command exits 0, `apps/web/package.json` contains `"motion"`, and `package-lock.json` updates.

- [ ] **Step 2: Install Playwright test dependency**

Run:

```powershell
npm install --workspace @final-review/web --save-dev @playwright/test
```

Expected: command exits 0, `apps/web/package.json` contains `"@playwright/test"` under `devDependencies`, and `package-lock.json` updates.

- [ ] **Step 3: Create shared home data types**

Create `apps/web/components/home/home-types.ts`:

```ts
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
```

- [ ] **Step 4: Create product vision data**

Create `apps/web/components/home/home-data.ts`:

```ts
import {
  BadgeCheck,
  BookMarked,
  Bot,
  BrainCircuit,
  ClipboardCheck,
  Coins,
  FileQuestion,
  FileText,
  Gift,
  GraduationCap,
  MessageSquareText,
  NotebookPen,
  PenLine,
  ShieldCheck,
  Sparkles,
  Tags,
  UsersRound,
} from "lucide-react";
import type {
  ArchiveDirectoryItem,
  CourseBook,
  GuaranteeItem,
  RecentUpdate,
  VisionFeature,
  VisionNote,
} from "./home-types";

export const archiveDirectory: ArchiveDirectoryItem[] = [
  { label: "课程资料", href: "/courses", description: "按课程进入讲义、真题、解析和实验资料。" },
  { label: "历年真题", href: "/courses", description: "围绕课程归档考前练习和解析入口。" },
  { label: "实验资料", href: "/courses", description: "实验报告、代码材料和环境说明集中维护。" },
  { label: "复习笔记", href: "/courses", description: "考前速记、重点整理和同学贡献内容。" },
  { label: "课程包", href: "/courses", description: "按课程打包解锁 paid 资料和复习组合。" },
  { label: "刷题练习", href: "/courses", description: "从课程题目进入练习、提交和错题沉淀。" },
  { label: "我的下载", href: "/me/downloads", description: "登录后回看自己的资料下载记录。" },
];

export const courseBooks: CourseBook[] = [
  { label: "数据结构", code: "CS201", subtitle: "Data Structures", meta: "讲义 / 真题 / 题目", href: "/courses", tone: "blue" },
  { label: "操作系统", code: "CS301", subtitle: "Operating Systems", meta: "实验 / 速背 / 错题", href: "/courses", tone: "orange" },
  { label: "计算机网络", code: "CS302", subtitle: "Computer Networks", meta: "协议 / 真题 / 讲义", href: "/courses", tone: "green" },
  { label: "软件工程", code: "SE301", subtitle: "Software Engineering", meta: "复习包 / 案例 / 笔记", href: "/courses", tone: "pink" },
  { label: "数据库系统", code: "DB202", subtitle: "Database Systems", meta: "SQL / 讲义 / 实验", href: "/courses", tone: "purple" },
  { label: "编译原理", code: "CS401", subtitle: "Compilers", meta: "语法 / 题目 / 资料", href: "/courses", tone: "cyan" },
];

export const communityNotes: VisionNote[] = [
  { title: "Wiki 共创", body: "一起整理知识点、资料说明和课程复习脉络。", tone: "yellow", tilt: "left" },
  { title: "博客经验", body: "沉淀复习路线、考前经验和避坑提醒。", tone: "pink", tilt: "right" },
  { title: "课程帖子", body: "围绕资料补充、勘误、提问和讨论展开。", tone: "blue", tilt: "none" },
  { title: "学习动态", body: "记录资料更新、同学贡献和课程相关活动。", tone: "green", tilt: "right" },
];

export const practiceFeatures: VisionFeature[] = [
  { title: "在线刷题", body: "从课程题目进入练习，提交后得到即时反馈。", icon: ClipboardCheck },
  { title: "错题本", body: "把做错的题目沉淀下来，复习时回到具体课程。", icon: NotebookPen },
  { title: "薄弱点统计", body: "按课程和知识点统计薄弱环节，减少盲目复习。", icon: BrainCircuit },
  { title: "AI 针对性强化", body: "围绕错题和资料给出复习提示，生成内容先进入审核边界。", icon: Bot },
];

export const membershipFeatures: VisionFeature[] = [
  { title: "创作者积分", body: "资料补充、内容共创和课程经验沉淀都能进入激励体系。", icon: Coins },
  { title: "会员权益", body: "会员和积分共同控制资料权益、课程包权益和 AI 使用额度。", icon: BadgeCheck },
  { title: "课程包", body: "以课程为单位组织 paid 资料，解锁仍由服务端校验。", icon: Gift },
  { title: "成本控制", body: "AI 能力按积分或会员额度使用，避免无边界消耗。", icon: Tags },
];

export const guaranteeItems: GuaranteeItem[] = [
  { title: "资料稳定供应", body: "课程 PDF 按资料库规则维护，避免临时网盘式失效。", icon: BookMarked },
  { title: "轻水印", body: "下载资料带有不影响阅读的来源标识。", icon: Sparkles },
  { title: "权限校验", body: "free、login_required、paid 资料都由 Go API 服务端判定。", icon: ShieldCheck },
  { title: "审核边界", body: "AI 草稿和贡献内容先进入审核流程，再成为正式内容。", icon: PenLine },
];

export const salesFeatures: VisionFeature[] = [
  { title: "群内咨询", body: "面向 QQ/微信群访客解释课程包和平台权益。", icon: MessageSquareText },
  { title: "购买引导", body: "把咨询转成注册、课程包购买和售后分流。", icon: UsersRound },
];

export const recentUpdates: RecentUpdate[] = [
  { title: "数据结构期末真题解析", course: "数据结构", href: "/courses", label: "真题解析" },
  { title: "操作系统考前速背清单", course: "操作系统", href: "/courses", label: "复习笔记" },
  { title: "计算机网络协议分层讲义", course: "计算机网络", href: "/courses", label: "讲义资料" },
  { title: "软件工程复习包说明", course: "软件工程", href: "/courses", label: "课程包" },
];

export const heroLinks = {
  primary: { label: "进入课程资料", href: "/courses" },
  secondary: { label: "查看资料保障", href: "#guarantee" },
};

export const directoryIcons = {
  materials: FileText,
  quiz: FileQuestion,
  course: GraduationCap,
};
```

- [ ] **Step 5: Verify types compile**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: command exits 0.

- [ ] **Step 6: Commit**

```powershell
git add apps/web/package.json package-lock.json apps/web/components/home/home-types.ts apps/web/components/home/home-data.ts
git commit -m "feat(web): add homepage vision data contract"
```

## Task 2: Static Homepage Composition

**Files:**

- Replace: `apps/web/app/page.tsx`
- Create: `apps/web/components/home/home-page.tsx`
- Create: `apps/web/components/home/home-nav.tsx`
- Create: `apps/web/components/home/hero-intro.tsx`
- Create: `apps/web/components/home/final-home-cta.tsx`

- [ ] **Step 1: Replace homepage entry with a server composition**

Replace `apps/web/app/page.tsx` with:

```tsx
import { HomePage } from "@/components/home/home-page";

export default function Page() {
  return <HomePage />;
}
```

- [ ] **Step 2: Create homepage navigation**

Create `apps/web/components/home/home-nav.tsx`:

```tsx
import Link from "next/link";
import { BookOpen, Download } from "lucide-react";

const links = [
  { label: "课程资料", href: "/courses" },
  { label: "社区共创", href: "#community" },
  { label: "刷题 AI", href: "#practice" },
  { label: "资料保障", href: "#guarantee" },
];

export function HomeNav() {
  return (
    <header className="sticky top-3 z-40 mx-auto flex w-[min(1120px,calc(100%-24px))] items-center justify-between rounded-full border border-[#2b2117]/12 bg-[#fffaf2]/86 px-3 py-2 shadow-[0_16px_48px_rgba(71,49,27,0.12)] backdrop-blur-md">
      <Link className="flex min-w-0 items-center gap-2 rounded-full pr-2 text-sm font-semibold text-[#2b2117]" href="/">
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-[#2f6b58] text-white">
          <BookOpen className="size-4" aria-hidden="true" />
        </span>
        <span className="hidden sm:inline">软件学院资料库</span>
      </Link>
      <nav className="hidden items-center gap-1 text-sm text-[#6f604f] md:flex">
        {links.map((link) => (
          <Link key={link.href} className="rounded-full px-3 py-2 transition hover:bg-[#f0e4d2] hover:text-[#2b2117]" href={link.href}>
            {link.label}
          </Link>
        ))}
      </nav>
      <Link className="inline-flex items-center gap-2 rounded-full border border-[#2b2117]/16 bg-white px-3 py-2 text-sm font-medium text-[#2b2117] shadow-sm transition hover:-translate-y-0.5 hover:shadow-md" href="/me/downloads">
        <Download className="size-4" aria-hidden="true" />
        <span className="hidden sm:inline">我的下载</span>
      </Link>
    </header>
  );
}
```

- [ ] **Step 3: Create hero intro**

Create `apps/web/components/home/hero-intro.tsx`:

```tsx
import Link from "next/link";
import { ArrowRight, Search } from "lucide-react";
import { heroLinks } from "./home-data";

export function HeroIntro() {
  return (
    <section className="relative mx-auto grid min-h-[calc(100dvh-88px)] w-[min(1160px,calc(100%-32px))] items-start gap-8 pb-8 pt-20 lg:grid-cols-[0.9fr_1.1fr] lg:pt-28">
      <div className="z-10 max-w-xl">
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Final Review Platform</p>
        <h1 className="mt-5 text-5xl font-black leading-[0.95] tracking-tight text-[#2b2117] sm:text-6xl lg:text-7xl">
          打开你的期末复习资料册
        </h1>
        <p className="mt-6 max-w-lg text-base leading-7 text-[#685b4b] sm:text-lg">
          按课程找到讲义、真题、实验资料和复习包，围绕资料继续刷题、讨论和共创。
        </p>
        <div className="mt-7 flex max-w-lg items-center rounded-2xl border border-[#2b2117]/14 bg-white/86 p-2 shadow-[0_20px_60px_rgba(71,49,27,0.11)]">
          <Search className="ml-2 size-5 shrink-0 text-[#9a7154]" aria-hidden="true" />
          <input className="min-w-0 flex-1 bg-transparent px-3 py-2 text-sm text-[#2b2117] outline-none placeholder:text-[#9b8b78]" placeholder="搜索课程、讲义、真题、实验资料" type="search" />
          <Link className="inline-flex shrink-0 items-center rounded-xl bg-[#2f6b58] px-4 py-2 text-sm font-semibold text-white transition hover:bg-[#285a4b]" href={heroLinks.primary.href}>
            搜索
          </Link>
        </div>
        <div className="mt-6 flex flex-wrap gap-3">
          <Link className="inline-flex items-center gap-2 rounded-full bg-[#2b2117] px-5 py-3 text-sm font-semibold text-white shadow-[0_16px_40px_rgba(43,33,23,0.2)] transition hover:-translate-y-0.5" href={heroLinks.primary.href}>
            {heroLinks.primary.label}
            <ArrowRight className="size-4" aria-hidden="true" />
          </Link>
          <Link className="inline-flex items-center rounded-full border border-[#2b2117]/18 bg-white/72 px-5 py-3 text-sm font-semibold text-[#2b2117] transition hover:-translate-y-0.5 hover:bg-white" href={heroLinks.secondary.href}>
            {heroLinks.secondary.label}
          </Link>
        </div>
      </div>
      <div className="hidden lg:block" aria-hidden="true" />
    </section>
  );
}
```

- [ ] **Step 4: Create final CTA**

Create `apps/web/components/home/final-home-cta.tsx`:

```tsx
import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { recentUpdates } from "./home-data";

export function FinalHomeCta() {
  return (
    <section className="mx-auto grid w-[min(1120px,calc(100%-32px))] gap-8 rounded-[2rem] border border-[#2b2117]/12 bg-[#fffaf2] p-6 shadow-[0_24px_90px_rgba(71,49,27,0.12)] lg:grid-cols-[0.9fr_1.1fr] lg:p-10">
      <div>
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Course Library</p>
        <h2 className="mt-3 text-3xl font-black tracking-tight text-[#2b2117] sm:text-4xl">从一门课开始复习</h2>
        <p className="mt-4 text-sm leading-7 text-[#6f604f]">
          首页讲完整产品愿景，真正的学习路径仍从课程资料库开始。
        </p>
        <Link className="mt-6 inline-flex items-center gap-2 rounded-full bg-[#2f6b58] px-5 py-3 text-sm font-semibold text-white transition hover:-translate-y-0.5 hover:bg-[#285a4b]" href="/courses">
          进入课程资料库
          <ArrowRight className="size-4" aria-hidden="true" />
        </Link>
      </div>
      <div className="grid gap-3">
        {recentUpdates.map((item) => (
          <Link key={`${item.course}-${item.title}`} className="group flex items-center justify-between gap-4 rounded-2xl border border-[#2b2117]/10 bg-white/72 p-4 transition hover:-translate-y-0.5 hover:bg-white" href={item.href}>
            <span className="min-w-0">
              <span className="block truncate text-sm font-semibold text-[#2b2117]">{item.title}</span>
              <span className="mt-1 block text-xs text-[#7a6a58]">{item.course} / {item.label}</span>
            </span>
            <ArrowRight className="size-4 shrink-0 text-[#b75c32] transition group-hover:translate-x-1" aria-hidden="true" />
          </Link>
        ))}
      </div>
    </section>
  );
}
```

- [ ] **Step 5: Create page composition**

Create `apps/web/components/home/home-page.tsx`:

```tsx
import { FinalHomeCta } from "./final-home-cta";
import { HeroIntro } from "./hero-intro";
import { HomeNav } from "./home-nav";

export function HomePage() {
  return (
    <main className="home-page min-h-[100dvh] overflow-x-hidden text-[#2b2117]">
      <HomeNav />
      <HeroIntro />
      <FinalHomeCta />
      <footer className="mx-auto w-[min(1120px,calc(100%-32px))] py-10 text-center text-xs text-[#85745f]">
        软件学院资料库 / 课程资料、刷题、共创和资料保障
      </footer>
    </main>
  );
}
```

- [ ] **Step 6: Verify composition compiles**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: command exits 0.

- [ ] **Step 7: Commit**

```powershell
git add apps/web/app/page.tsx apps/web/components/home/home-page.tsx apps/web/components/home/home-nav.tsx apps/web/components/home/hero-intro.tsx apps/web/components/home/final-home-cta.tsx
git commit -m "feat(web): scaffold homepage product vision layout"
```

## Task 3: Visual Primitives and CSS Module

**Files:**

- Create: `apps/web/components/home/home-visuals.module.css`
- Create: `apps/web/components/home/pdf-course-book.tsx`

- [ ] **Step 1: Create home visual CSS module**

Create `apps/web/components/home/home-visuals.module.css` with these class groups:

```css
.courseBook {
  position: relative;
  display: flex;
  min-height: 176px;
  flex-direction: column;
  justify-content: space-between;
  overflow: hidden;
  border-radius: 18px 14px 14px 18px;
  border: 2px solid rgb(43 33 23 / 0.18);
  padding: 18px;
  color: #2b2117;
  box-shadow:
    8px 10px 0 rgb(43 33 23 / 0.12),
    0 22px 48px rgb(71 49 27 / 0.16);
  transform: rotate(var(--book-tilt, -2deg));
  transition:
    transform 220ms ease,
    box-shadow 220ms ease;
}

.courseBook:hover {
  transform: translateY(-6px) rotate(var(--book-tilt, -2deg));
  box-shadow:
    10px 14px 0 rgb(43 33 23 / 0.14),
    0 30px 68px rgb(71 49 27 / 0.2);
}

.courseBook::before {
  content: "";
  position: absolute;
  inset: 0 auto 0 0;
  width: 18px;
  background: rgb(43 33 23 / 0.14);
}

.courseBook::after {
  content: "";
  position: absolute;
  inset: 10px 10px 10px auto;
  width: 10px;
  border-radius: 999px;
  background: rgb(255 255 255 / 0.44);
}

.toneBlue { background: linear-gradient(135deg, #7fc7ff 0%, #3978e8 100%); }
.toneOrange { background: linear-gradient(135deg, #ffd166 0%, #ef7d31 100%); }
.toneGreen { background: linear-gradient(135deg, #a9e67d 0%, #30a66a 100%); }
.tonePink { background: linear-gradient(135deg, #ff9bc6 0%, #eb4f8a 100%); }
.tonePurple { background: linear-gradient(135deg, #b6a0ff 0%, #6f55dc 100%); }
.toneCyan { background: linear-gradient(135deg, #8de7e2 0%, #2aa6c8 100%); }

.stickyNote {
  position: relative;
  min-height: 210px;
  border-radius: 10px;
  padding: 28px 24px;
  box-shadow: 0 24px 54px rgb(71 49 27 / 0.16);
  transform: rotate(var(--note-tilt, 0deg));
}

.stickyNote::before {
  content: "";
  position: absolute;
  inset: 0 0 auto 0;
  height: 22px;
  border-radius: inherit;
  background: rgb(255 255 255 / 0.24);
}

.noteYellow { background: linear-gradient(180deg, #ffe36b 0%, #ffd242 100%); }
.notePink { background: linear-gradient(180deg, #ffb5cc 0%, #ff8eb3 100%); }
.noteBlue { background: linear-gradient(180deg, #a8d9ff 0%, #73c2ff 100%); }
.noteGreen { background: linear-gradient(180deg, #bdf08b 0%, #90d85c 100%); }

.paperCard {
  border: 1px solid rgb(43 33 23 / 0.12);
  border-radius: 24px;
  background:
    linear-gradient(rgb(47 107 88 / 0.08) 1px, transparent 1px),
    #fffaf2;
  background-size: 100% 30px;
  box-shadow: 0 20px 58px rgb(71 49 27 / 0.12);
}

.ticket {
  position: relative;
  border: 1px solid rgb(43 33 23 / 0.14);
  border-radius: 28px;
  background: #fff7df;
  box-shadow: 0 22px 64px rgb(71 49 27 / 0.13);
}

.ticket::before,
.ticket::after {
  content: "";
  position: absolute;
  top: 50%;
  width: 28px;
  height: 28px;
  border-radius: 999px;
  background: #f7efe4;
  transform: translateY(-50%);
}

.ticket::before { left: -14px; }
.ticket::after { right: -14px; }

@media (prefers-reduced-motion: reduce) {
  .courseBook,
  .courseBook:hover,
  .stickyNote {
    transform: none;
    transition: none;
  }
}
```

- [ ] **Step 2: Create course PDF book primitive**

Create `apps/web/components/home/pdf-course-book.tsx`:

```tsx
import Link from "next/link";
import { ArrowRight } from "lucide-react";
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

export function PdfCourseBook({ course }: { course: CourseBook }) {
  return (
    <Link
      className={`${styles.courseBook} ${toneClass[course.tone]}`}
      href={course.href}
      style={{ "--book-tilt": tilt[course.tone] } as React.CSSProperties}
    >
      <span className="relative z-10 flex items-center justify-between gap-3 font-mono text-xs font-semibold text-[#2b2117]/78">
        <span>{course.code}</span>
        <span>PDF</span>
      </span>
      <span className="relative z-10">
        <span className="block text-2xl font-black tracking-tight text-[#2b2117]">{course.label}</span>
        <span className="mt-1 block font-mono text-xs text-[#2b2117]/70">{course.subtitle}</span>
      </span>
      <span className="relative z-10 flex items-center justify-between gap-2 text-xs font-semibold text-[#2b2117]/72">
        <span>{course.meta}</span>
        <ArrowRight className="size-4" aria-hidden="true" />
      </span>
    </Link>
  );
}
```

- [ ] **Step 3: Verify primitive compiles**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: command exits 0.

- [ ] **Step 4: Commit**

```powershell
git add apps/web/components/home/home-visuals.module.css apps/web/components/home/pdf-course-book.tsx
git commit -m "feat(web): add homepage paper visual primitives"
```

## Task 4: Desktop Archive Book Reveal

**Files:**

- Create: `apps/web/components/home/archive-book-reveal.tsx`
- Modify: `apps/web/components/home/home-visuals.module.css`
- Modify: `apps/web/components/home/home-page.tsx`

- [ ] **Step 1: Add archive book CSS classes**

Append to `apps/web/components/home/home-visuals.module.css`:

```css
.bookStage {
  position: relative;
  min-height: 220vh;
}

.bookSticky {
  position: sticky;
  top: 0;
  display: grid;
  min-height: 100dvh;
  place-items: center;
  overflow: hidden;
  padding: 96px 24px 48px;
}

.archiveBook {
  position: relative;
  width: min(1040px, 90vw);
  height: min(640px, 58vw);
  min-height: 520px;
  perspective: 1800px;
}

.bookBase,
.bookCover,
.bookInside {
  position: absolute;
  inset: 0;
  border-radius: 44px;
}

.bookBase {
  background: linear-gradient(135deg, #c68b56 0%, #9f6c42 100%);
  box-shadow:
    0 42px 110px rgb(70 45 22 / 0.28),
    inset 0 0 0 2px rgb(255 255 255 / 0.25);
}

.bookCover {
  transform-origin: left center;
  border: 2px solid rgb(43 33 23 / 0.16);
  background:
    radial-gradient(circle at 22% 24%, rgb(255 255 255 / 0.28), transparent 26%),
    linear-gradient(135deg, #d99c5d 0%, #b67846 100%);
  box-shadow:
    inset 0 0 0 2px rgb(255 255 255 / 0.18),
    0 28px 72px rgb(70 45 22 / 0.26);
  backface-visibility: hidden;
}

.bookInside {
  display: grid;
  grid-template-columns: 0.92fr 1.08fr;
  gap: 18px;
  padding: 26px;
  background: linear-gradient(90deg, #b77b47 0 49.4%, #8d5f39 49.4% 50.6%, #b77b47 50.6% 100%);
}

.bookPage {
  overflow: hidden;
  border: 1px solid rgb(43 33 23 / 0.12);
  border-radius: 32px;
  background: #fffaf2;
  box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.62);
}

.directoryLine {
  border-bottom: 1px solid rgb(43 33 23 / 0.12);
}

@media (max-width: 1023px) {
  .bookStage {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .bookStage {
    min-height: auto;
  }

  .bookSticky {
    position: relative;
  }
}
```

- [ ] **Step 2: Create desktop reveal component**

Create `apps/web/components/home/archive-book-reveal.tsx`:

```tsx
"use client";

import Link from "next/link";
import { motion, useReducedMotion, useScroll, useTransform } from "motion/react";
import { useRef } from "react";
import { archiveDirectory, courseBooks } from "./home-data";
import { PdfCourseBook } from "./pdf-course-book";
import styles from "./home-visuals.module.css";

export function ArchiveBookReveal() {
  const ref = useRef<HTMLElement>(null);
  const reduceMotion = useReducedMotion();
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end end"],
  });

  const rotate = useTransform(scrollYProgress, [0, 0.32], reduceMotion ? [0, 0] : [-10, 0]);
  const y = useTransform(scrollYProgress, [0, 0.32], reduceMotion ? [0, 0] : [120, 0]);
  const scale = useTransform(scrollYProgress, [0, 0.32], reduceMotion ? [1, 1] : [0.86, 1]);
  const coverRotate = useTransform(scrollYProgress, [0.35, 0.78], reduceMotion ? [-172, -172] : [0, -172]);
  const contentOpacity = useTransform(scrollYProgress, [0.62, 0.78], [0, 1]);
  const contentY = useTransform(scrollYProgress, [0.62, 0.78], reduceMotion ? [0, 0] : [24, 0]);

  return (
    <section ref={ref} className={styles.bookStage} aria-label="课程资料档案册展开演示">
      <div className={styles.bookSticky}>
        <motion.div className={styles.archiveBook} style={{ rotate, scale, y }}>
          <div className={styles.bookBase} />
          <div className={styles.bookInside}>
            <motion.div className={`${styles.bookPage} p-8`} style={{ opacity: contentOpacity, y: contentY }}>
              <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Directory</p>
              <h2 className="mt-3 text-4xl font-black tracking-tight text-[#2b2117]">资料目录</h2>
              <div className="mt-6 grid gap-1">
                {archiveDirectory.map((item) => (
                  <Link key={item.label} className={`${styles.directoryLine} group block py-3`} href={item.href}>
                    <span className="flex items-baseline justify-between gap-3">
                      <span className="text-lg font-bold text-[#2b2117] group-hover:text-[#2f6b58]">{item.label}</span>
                      <span className="font-mono text-xs text-[#a26b43]">OPEN</span>
                    </span>
                    <span className="mt-1 block text-sm leading-6 text-[#756653]">{item.description}</span>
                  </Link>
                ))}
              </div>
            </motion.div>

            <motion.div className={`${styles.bookPage} p-8`} style={{ opacity: contentOpacity, y: contentY }}>
              <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Course PDFs</p>
              <h2 className="mt-3 text-4xl font-black tracking-tight text-[#2b2117]">课程入口</h2>
              <div className="mt-7 grid grid-cols-2 gap-5">
                {courseBooks.map((course) => (
                  <PdfCourseBook key={course.label} course={course} />
                ))}
              </div>
            </motion.div>
          </div>

          <motion.div className={styles.bookCover} style={{ rotateY: coverRotate }}>
            <div className="flex h-full flex-col justify-between p-12">
              <div>
                <p className="font-mono text-sm font-semibold uppercase tracking-[0.18em] text-[#593a24]/70">Software College</p>
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
```

- [ ] **Step 3: Insert desktop reveal into page composition**

Modify `apps/web/components/home/home-page.tsx` so it imports and renders the desktop reveal immediately after `<HeroIntro />`:

```tsx
import { ArchiveBookReveal } from "./archive-book-reveal";
```

The relevant body should become:

```tsx
<HomeNav />
<HeroIntro />
<ArchiveBookReveal />
<FinalHomeCta />
```

- [ ] **Step 4: Verify desktop component compiles**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: command exits 0.

- [ ] **Step 5: Commit**

```powershell
git add apps/web/components/home/archive-book-reveal.tsx apps/web/components/home/home-visuals.module.css apps/web/components/home/home-page.tsx
git commit -m "feat(web): add desktop archive book reveal"
```

## Task 5: Mobile Archive Intro

**Files:**

- Create: `apps/web/components/home/mobile-archive-intro.tsx`
- Modify: `apps/web/components/home/home-visuals.module.css`
- Modify: `apps/web/components/home/home-page.tsx`

- [ ] **Step 1: Add mobile archive CSS**

Append to `apps/web/components/home/home-visuals.module.css`:

```css
.mobileArchive {
  display: none;
}

.mobileBook {
  border-radius: 28px;
  border: 2px solid rgb(43 33 23 / 0.14);
  background: linear-gradient(135deg, #d99c5d 0%, #b67846 100%);
  box-shadow: 0 24px 70px rgb(70 45 22 / 0.2);
}

@media (max-width: 1023px) {
  .mobileArchive {
    display: block;
  }
}
```

- [ ] **Step 2: Create mobile intro component**

Create `apps/web/components/home/mobile-archive-intro.tsx`:

```tsx
"use client";

import { motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import { archiveDirectory, courseBooks } from "./home-data";
import { PdfCourseBook } from "./pdf-course-book";
import styles from "./home-visuals.module.css";

export function MobileArchiveIntro() {
  const reduceMotion = useReducedMotion();

  return (
    <section className={`${styles.mobileArchive} mx-auto w-[min(100%-32px,720px)] pb-16`}>
      <motion.div
        className={`${styles.mobileBook} p-5`}
        initial={reduceMotion ? false : { rotate: -4, y: 30, opacity: 0 }}
        whileInView={reduceMotion ? { opacity: 1 } : { rotate: 0, y: 0, opacity: 1 }}
        viewport={{ once: true, amount: 0.35 }}
        transition={{ duration: 0.7, ease: [0.16, 1, 0.3, 1] }}
      >
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.16em] text-[#593a24]/70">Mobile Library</p>
        <h2 className="mt-3 text-3xl font-black leading-tight text-[#2b2117]">资料册已打开</h2>
        <div className="mt-5 grid grid-cols-2 gap-2">
          {archiveDirectory.slice(0, 6).map((item) => (
            <Link key={item.label} className="rounded-2xl bg-white/74 px-3 py-2 text-sm font-semibold text-[#2b2117]" href={item.href}>
              {item.label}
            </Link>
          ))}
        </div>
      </motion.div>

      <div className="mt-6 flex snap-x gap-4 overflow-x-auto pb-5">
        {courseBooks.map((course) => (
          <div key={course.label} className="w-56 shrink-0 snap-start">
            <PdfCourseBook course={course} />
          </div>
        ))}
      </div>
    </section>
  );
}
```

- [ ] **Step 3: Insert mobile intro into page composition**

Modify `apps/web/components/home/home-page.tsx` so it imports and renders the mobile intro immediately after `<ArchiveBookReveal />`:

```tsx
import { MobileArchiveIntro } from "./mobile-archive-intro";
```

The relevant body should become:

```tsx
<HeroIntro />
<ArchiveBookReveal />
<MobileArchiveIntro />
<FinalHomeCta />
```

- [ ] **Step 4: Verify mobile component compiles**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: command exits 0.

- [ ] **Step 5: Commit**

```powershell
git add apps/web/components/home/mobile-archive-intro.tsx apps/web/components/home/home-visuals.module.css apps/web/components/home/home-page.tsx
git commit -m "feat(web): add mobile archive homepage intro"
```

## Task 6: Product Vision Narrative Sections

**Files:**

- Create: `apps/web/components/home/community-sticky-notes.tsx`
- Create: `apps/web/components/home/practice-vision-section.tsx`
- Create: `apps/web/components/home/membership-ticket-section.tsx`
- Create: `apps/web/components/home/sales-assistant-note.tsx`
- Create: `apps/web/components/home/guarantee-section.tsx`
- Modify: `apps/web/components/home/home-page.tsx`

- [ ] **Step 1: Create sticky-note community section**

Create `apps/web/components/home/community-sticky-notes.tsx`:

```tsx
import { communityNotes } from "./home-data";
import styles from "./home-visuals.module.css";

const toneClass = {
  yellow: styles.noteYellow,
  pink: styles.notePink,
  blue: styles.noteBlue,
  green: styles.noteGreen,
};

const tilt = {
  left: "-4deg",
  right: "4deg",
  none: "0deg",
};

export function CommunityStickyNotes() {
  return (
    <section id="community" className="mx-auto min-h-[90dvh] w-[min(1120px,calc(100%-32px))] py-20">
      <div className="mx-auto max-w-2xl text-center">
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Community</p>
        <h2 className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">资料会继续长出来</h2>
        <p className="mt-4 text-sm leading-7 text-[#6f604f]">
          Wiki、博客、帖子和动态都围绕课程资料展开，不做泛社交信息流。
        </p>
      </div>
      <div className="mt-16 grid gap-5 md:grid-cols-4">
        {communityNotes.map((note) => (
          <article
            key={note.title}
            className={`${styles.stickyNote} ${toneClass[note.tone]}`}
            style={{ "--note-tilt": tilt[note.tilt] } as React.CSSProperties}
          >
            <h3 className="relative z-10 text-2xl font-black tracking-tight text-[#2b2117]">{note.title}</h3>
            <p className="relative z-10 mt-5 text-sm leading-7 text-[#493621]">{note.body}</p>
          </article>
        ))}
      </div>
    </section>
  );
}
```

- [ ] **Step 2: Create practice and AI section**

Create `apps/web/components/home/practice-vision-section.tsx`:

```tsx
import { practiceFeatures } from "./home-data";
import styles from "./home-visuals.module.css";

export function PracticeVisionSection() {
  return (
    <section id="practice" className="mx-auto grid w-[min(1120px,calc(100%-32px))] gap-8 py-20 lg:grid-cols-[0.8fr_1.2fr]">
      <div>
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Practice</p>
        <h2 className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">资料旁边就是练习</h2>
        <p className="mt-5 text-sm leading-7 text-[#6f604f]">
          刷题、错题本、薄弱点统计和 AI 强化都回到具体课程，不把 AI 做成首页主入口。
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        {practiceFeatures.map((feature) => {
          const Icon = feature.icon;
          return (
            <article key={feature.title} className={`${styles.paperCard} p-5`}>
              <Icon className="size-6 text-[#2f6b58]" aria-hidden="true" />
              <h3 className="mt-5 text-xl font-black tracking-tight text-[#2b2117]">{feature.title}</h3>
              <p className="mt-3 text-sm leading-7 text-[#6f604f]">{feature.body}</p>
            </article>
          );
        })}
      </div>
    </section>
  );
}
```

- [ ] **Step 3: Create membership ticket section**

Create `apps/web/components/home/membership-ticket-section.tsx`:

```tsx
import { membershipFeatures } from "./home-data";
import styles from "./home-visuals.module.css";

export function MembershipTicketSection() {
  return (
    <section className="mx-auto w-[min(1120px,calc(100%-32px))] py-20">
      <div className={`${styles.ticket} p-6 lg:p-10`}>
        <div className="grid gap-8 lg:grid-cols-[0.8fr_1.2fr]">
          <div>
            <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Points & Membership</p>
            <h2 className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">贡献、权益和成本控制</h2>
            <p className="mt-5 text-sm leading-7 text-[#6f604f]">
              积分会员体系用于连接创作者激励、课程包权益和 AI 使用额度。支付能力按产品边界谨慎表达。
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            {membershipFeatures.map((feature) => {
              const Icon = feature.icon;
              return (
                <article key={feature.title} className="rounded-3xl border border-[#2b2117]/10 bg-white/70 p-5">
                  <Icon className="size-6 text-[#b75c32]" aria-hidden="true" />
                  <h3 className="mt-4 text-lg font-black tracking-tight text-[#2b2117]">{feature.title}</h3>
                  <p className="mt-3 text-sm leading-7 text-[#6f604f]">{feature.body}</p>
                </article>
              );
            })}
          </div>
        </div>
      </div>
    </section>
  );
}
```

- [ ] **Step 4: Create sales assistant note**

Create `apps/web/components/home/sales-assistant-note.tsx`:

```tsx
import { salesFeatures } from "./home-data";

export function SalesAssistantNote() {
  return (
    <section className="mx-auto w-[min(900px,calc(100%-32px))] py-12">
      <div className="rotate-[-1deg] rounded-[2rem] border border-[#2b2117]/12 bg-[#d8f1ff] p-6 shadow-[0_22px_64px_rgba(71,49,27,0.12)] md:p-8">
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#2a6d88]">LangBot</p>
        <h2 className="mt-3 text-3xl font-black tracking-tight text-[#2b2117]">群里的咨询，也能接住</h2>
        <div className="mt-6 grid gap-4 md:grid-cols-2">
          {salesFeatures.map((feature) => {
            const Icon = feature.icon;
            return (
              <article key={feature.title} className="rounded-2xl bg-white/72 p-4">
                <Icon className="size-5 text-[#2a6d88]" aria-hidden="true" />
                <h3 className="mt-3 text-lg font-black text-[#2b2117]">{feature.title}</h3>
                <p className="mt-2 text-sm leading-6 text-[#5b4f44]">{feature.body}</p>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}
```

- [ ] **Step 5: Create guarantee section**

Create `apps/web/components/home/guarantee-section.tsx`:

```tsx
import { guaranteeItems } from "./home-data";

export function GuaranteeSection() {
  return (
    <section id="guarantee" className="mx-auto w-[min(1120px,calc(100%-32px))] py-20">
      <div className="rounded-[2rem] border border-[#2b2117]/12 bg-[#f8efe2] p-6 lg:p-10">
        <div className="max-w-2xl">
          <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Guarantee</p>
          <h2 className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">资料保障要讲清楚</h2>
          <p className="mt-5 text-sm leading-7 text-[#6f604f]">
            首页可以展示产品愿景，但资料下载、AI 草稿和 paid 权限都必须保留服务端与审核边界。
          </p>
        </div>
        <div className="mt-8 grid gap-4 md:grid-cols-4">
          {guaranteeItems.map((item) => {
            const Icon = item.icon;
            return (
              <article key={item.title} className="rounded-3xl border border-[#2b2117]/10 bg-white/70 p-5">
                <Icon className="size-6 text-[#2f6b58]" aria-hidden="true" />
                <h3 className="mt-4 text-lg font-black tracking-tight text-[#2b2117]">{item.title}</h3>
                <p className="mt-3 text-sm leading-7 text-[#6f604f]">{item.body}</p>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}
```

- [ ] **Step 6: Insert narrative sections into page composition**

Modify `apps/web/components/home/home-page.tsx` so it imports and renders all narrative sections between `<MobileArchiveIntro />` and `<FinalHomeCta />`:

```tsx
import { CommunityStickyNotes } from "./community-sticky-notes";
import { GuaranteeSection } from "./guarantee-section";
import { MembershipTicketSection } from "./membership-ticket-section";
import { PracticeVisionSection } from "./practice-vision-section";
import { SalesAssistantNote } from "./sales-assistant-note";
```

The relevant body should become:

```tsx
<HeroIntro />
<ArchiveBookReveal />
<MobileArchiveIntro />
<CommunityStickyNotes />
<PracticeVisionSection />
<MembershipTicketSection />
<SalesAssistantNote />
<GuaranteeSection />
<FinalHomeCta />
```

- [ ] **Step 7: Verify narrative sections compile**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: command exits 0.

- [ ] **Step 8: Commit**

```powershell
git add apps/web/components/home/community-sticky-notes.tsx apps/web/components/home/practice-vision-section.tsx apps/web/components/home/membership-ticket-section.tsx apps/web/components/home/sales-assistant-note.tsx apps/web/components/home/guarantee-section.tsx apps/web/components/home/home-page.tsx
git commit -m "feat(web): add homepage product vision sections"
```

## Task 7: Global Styling, Build, and Visual QA

**Files:**

- Modify: `apps/web/app/globals.css`
- Create: `apps/web/tests/homepage.visual.spec.ts`

- [ ] **Step 1: Update global homepage base styling**

Modify `apps/web/app/globals.css`:

1. Keep existing Tailwind directives.
2. Keep existing CSS variables unless changing them is necessary for contrast.
3. Replace the `body` background block with this:

```css
body {
  margin: 0;
  background:
    radial-gradient(circle at 50% 8%, hsl(38 82% 96% / 0.92), transparent 34rem),
    linear-gradient(hsl(38 36% 84% / 0.2) 1px, transparent 1px),
    linear-gradient(90deg, hsl(38 36% 84% / 0.2) 1px, transparent 1px),
    hsl(var(--background));
  background-attachment: fixed;
  background-size: auto, 48px 48px, 48px 48px, auto;
  color: hsl(var(--foreground));
  font-family:
    Inter, "Microsoft YaHei", "PingFang SC", system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}
```

4. Add this after the `body` block:

```css
.home-page {
  background:
    radial-gradient(circle at 82% 4%, rgb(255 210 66 / 0.32), transparent 22rem),
    radial-gradient(circle at 8% 34%, rgb(127 199 255 / 0.2), transparent 24rem),
    #f7efe4;
}
```

- [ ] **Step 2: Install Playwright browser if missing**

Run:

```powershell
npx playwright install chromium
```

Expected: command exits 0. If the browser is already installed, the command may finish quickly.

- [ ] **Step 3: Create Playwright visual smoke test**

Create `apps/web/tests/homepage.visual.spec.ts`:

```ts
import { expect, test } from "@playwright/test";

test("homepage renders product vision on desktop", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto("http://127.0.0.1:3000/", { waitUntil: "networkidle" });
  await expect(page.getByRole("heading", { name: "打开你的期末复习资料册" })).toBeVisible();
  await expect(page.getByText("软件学院资料库").first()).toBeVisible();
  await page.mouse.wheel(0, 1600);
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "课程入口" })).toBeVisible();
  await expect(page.getByText("数据结构").first()).toBeVisible();
});

test("homepage uses simplified archive on mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto("http://127.0.0.1:3000/", { waitUntil: "networkidle" });
  await expect(page.getByRole("heading", { name: "打开你的期末复习资料册" })).toBeVisible();
  await page.mouse.wheel(0, 800);
  await expect(page.getByRole("heading", { name: "资料册已打开" })).toBeVisible();
  await expect(page.getByText("数据结构").first()).toBeVisible();
});
```

- [ ] **Step 4: Run TypeScript check**

Run:

```powershell
npm --workspace @final-review/web run lint
```

Expected: command exits 0.

- [ ] **Step 5: Run production build**

Run:

```powershell
npm --workspace @final-review/web run build
```

Expected: command exits 0.

- [ ] **Step 6: Start dev server for visual QA**

Run in PowerShell:

```powershell
$proc = Start-Process -FilePath "npm.cmd" -ArgumentList "--workspace", "@final-review/web", "run", "dev" -WorkingDirectory (Get-Location) -WindowStyle Hidden -PassThru
Start-Sleep -Seconds 10
try { Invoke-WebRequest -Uri "http://127.0.0.1:3000" -UseBasicParsing | Select-Object -ExpandProperty StatusCode } finally { $proc.Id | Out-File .next-web-dev.pid -Encoding ascii }
```

Expected: status code `200`. If port 3000 is busy, run `Get-NetTCPConnection -LocalPort 3000` to identify the owner and either reuse the running dev server or start Next on a different port with `npx next dev -p 3001` from `apps/web`.

- [ ] **Step 7: Run Playwright visual smoke test**

Run:

```powershell
npx playwright test apps/web/tests/homepage.visual.spec.ts
```

Expected: both tests pass.

- [ ] **Step 8: Capture desktop and mobile screenshots**

Run:

```powershell
New-Item -ItemType Directory -Force artifacts | Out-Null
npx playwright screenshot --viewport-size=1440,1100 http://127.0.0.1:3000 .\\artifacts\\homepage-desktop.png
npx playwright screenshot --viewport-size=390,900 http://127.0.0.1:3000 .\\artifacts\\homepage-mobile.png
```

Expected: screenshots are created and show nonblank homepage states. Inspect them before reporting completion.

- [ ] **Step 9: Stop dev server**

Run:

```powershell
if (Test-Path .next-web-dev.pid) {
  $pidValue = Get-Content .next-web-dev.pid
  Stop-Process -Id $pidValue -ErrorAction SilentlyContinue
  Remove-Item .next-web-dev.pid
}
```

Expected: dev server process is stopped if this task started it.

- [ ] **Step 10: Commit**

```powershell
git add apps/web/app/globals.css apps/web/tests/homepage.visual.spec.ts package-lock.json apps/web/package.json
git commit -m "test(web): add homepage visual verification"
```

## Final Review Checklist

After all tasks:

- [ ] Run `npm --workspace @final-review/web run lint`.
- [ ] Run `npm --workspace @final-review/web run build`.
- [ ] Run `npx playwright test apps/web/tests/homepage.visual.spec.ts`.
- [ ] Inspect `artifacts/homepage-desktop.png` and `artifacts/homepage-mobile.png`.
- [ ] Verify desktop scroll shows the archive book straightening and opening.
- [ ] Verify mobile shows the simplified archive intro and horizontally scrollable course books.
- [ ] Verify visible copy does not overclaim payment, AI publishing, or community availability.
- [ ] Verify no unrelated files are staged.
- [ ] Run `git status --short --branch`.
