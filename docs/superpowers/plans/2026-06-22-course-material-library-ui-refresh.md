# Course Material Library UI Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a shadcn-inspired, clean course-material-library UI across the Next.js student app and Vue admin console without changing backend behavior.

**Architecture:** Create a shared visual language through Tailwind tokens in `apps/web` and CSS variables in `apps/admin`. Student pages become a course-first material library with PDF and material-guarantee emphasis; admin pages become a material-operations dashboard using the existing Vue + Element Plus stack. Do not share React components with Vue; share visual decisions only.

**Tech Stack:** Next.js 16, React 19, Tailwind CSS, Vue 3, Element Plus, Pinia, TypeScript, `lucide-react` for the student app.

---

## File Map

- Modify `package.json` and `package-lock.json`: add the student-side icon dependency.
- Modify `apps/web/package.json`: add `lucide-react`.
- Modify `apps/web/tailwind.config.ts`: extend shadcn-like color, radius, and shadow tokens.
- Modify `apps/web/app/globals.css`: set global background, font smoothing, selection, and base focus styles.
- Create `apps/web/components/layout/site-shell.tsx`: shared student shell with top navigation.
- Create `apps/web/components/ui/badge.tsx`: small local badge helper for student-side metadata.
- Create `apps/web/components/ui/button-link.tsx`: local link-button styles for student CTAs.
- Create `apps/web/lib/presentation.ts`: material type labels, access labels, file-size formatting, and derived material stats.
- Modify `apps/web/app/page.tsx`: compact hero, course/material entry exposure, material guarantee section.
- Modify `apps/web/app/courses/page.tsx`: course-first catalog with material-library copy and card layout.
- Modify `apps/web/app/courses/[id]/page.tsx`: course detail as material conversion page; no primary quiz CTA.
- Modify `apps/web/app/materials/[id]/page.tsx`: PDF detail/download page with guarantee and light-watermark messaging.
- Modify `apps/web/app/login/page.tsx` and `apps/web/components/auth/login-form.tsx`: shadcn-like login surface while preserving API calls.
- Modify `apps/admin/src/styles/main.css`: introduce design variables, shadcn-like Element Plus overrides, dashboard/sidebar/table/form spacing.
- Modify `apps/admin/src/components/AdminShell.vue`: material-operations sidebar and future extension slots.
- Modify `apps/admin/src/views/LoginView.vue`: simplified login-card style.
- Modify `apps/admin/src/views/DashboardView.vue`: operational summary using current static module rows and existing dashboard structure.
- Modify `apps/admin/src/views/CoursesView.vue`: cleaner forms/tables and course-library language.
- Modify `apps/admin/src/views/MaterialsView.vue`: PDF-first upload and material guarantee language.

---

## Task 1: Student Design Tokens and Dependency

**Files:**
- Modify: `apps/web/package.json`
- Modify: `package-lock.json`
- Modify: `apps/web/tailwind.config.ts`
- Modify: `apps/web/app/globals.css`

- [ ] **Step 1: Add student icon dependency**

Run:

```bash
npm install --workspace @final-review/web lucide-react
```

Expected: `apps/web/package.json` contains `lucide-react`; `package-lock.json` updates. No admin dependency is added.

- [ ] **Step 2: Replace Tailwind token extension**

Update `apps/web/tailwind.config.ts` so the `extend` block contains these tokens:

```ts
extend: {
  borderRadius: {
    xl: "0.875rem",
    "2xl": "1rem",
  },
  boxShadow: {
    soft: "0 18px 60px rgba(15, 23, 42, 0.08)",
  },
  colors: {
    background: "#f7f8f5",
    foreground: "#18181b",
    muted: "#f1f3ee",
    "muted-foreground": "#6b7280",
    card: "#ffffff",
    border: "#dfe4dc",
    primary: "#2f5f51",
    "primary-foreground": "#ffffff",
    accent: "#edf6f1",
    "accent-foreground": "#23483d",
    ink: "#18181b",
    paper: "#f7f8f5",
    sage: "#2f5f51",
    line: "#dfe4dc",
  },
}
```

Keep the existing `content` paths.

- [ ] **Step 3: Update student globals**

Update `apps/web/app/globals.css` with:

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

:root {
  color-scheme: light;
}

* {
  box-sizing: border-box;
}

html {
  scroll-behavior: smooth;
}

body {
  margin: 0;
  background:
    radial-gradient(circle at top left, rgba(47, 95, 81, 0.09), transparent 34rem),
    #f7f8f5;
  color: #18181b;
  font-family:
    Inter, "Microsoft YaHei", "PingFang SC", system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}

a {
  color: inherit;
  text-decoration: none;
}

::selection {
  background: rgba(47, 95, 81, 0.18);
}

:focus-visible {
  outline: 2px solid #2f5f51;
  outline-offset: 2px;
}
```

- [ ] **Step 4: Verify student token changes compile**

Run:

```bash
npm --workspace @final-review/web run lint
```

Expected: `tsc --noEmit` exits 0.

- [ ] **Step 5: Commit**

```bash
git add package.json package-lock.json apps/web/package.json apps/web/tailwind.config.ts apps/web/app/globals.css
git commit -m "feat(web): add course library design tokens"
```

---

## Task 2: Student UI Helpers

**Files:**
- Create: `apps/web/components/layout/site-shell.tsx`
- Create: `apps/web/components/ui/badge.tsx`
- Create: `apps/web/components/ui/button-link.tsx`
- Create: `apps/web/lib/presentation.ts`

- [ ] **Step 1: Create student shell**

Create `apps/web/components/layout/site-shell.tsx`:

```tsx
import Link from "next/link";
import type { ReactNode } from "react";
import { BookOpen, ShieldCheck } from "lucide-react";

export function SiteShell({ children }: { children: ReactNode }) {
  return (
    <main className="min-h-screen px-4 py-4 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex max-w-6xl flex-col gap-6">
        <header className="sticky top-4 z-20 rounded-2xl border border-border/80 bg-card/90 px-4 py-3 shadow-sm backdrop-blur">
          <nav className="flex items-center justify-between gap-4">
            <Link className="flex items-center gap-2 font-semibold tracking-tight" href="/">
              <span className="grid size-8 place-items-center rounded-xl bg-primary text-primary-foreground">
                <BookOpen className="size-4" aria-hidden="true" />
              </span>
              <span>软件学院资料库</span>
            </Link>
            <div className="flex items-center gap-1 text-sm text-muted-foreground sm:gap-2">
              <Link className="rounded-lg px-3 py-2 hover:bg-muted hover:text-foreground" href="/courses">
                课程资料
              </Link>
              <a className="hidden rounded-lg px-3 py-2 hover:bg-muted hover:text-foreground sm:inline-flex" href="/#guarantee">
                资料保障
              </a>
              <Link className="rounded-lg border border-border bg-card px-3 py-2 text-foreground hover:bg-muted" href="/login">
                登录
              </Link>
            </div>
          </nav>
        </header>
        {children}
        <footer className="pb-6 text-center text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <ShieldCheck className="size-3" aria-hidden="true" />
            PDF 稳定供应 · 轻水印 · 持续维护
          </span>
        </footer>
      </div>
    </main>
  );
}
```

- [ ] **Step 2: Create badge helper**

Create `apps/web/components/ui/badge.tsx`:

```tsx
import type { ReactNode } from "react";

type BadgeTone = "default" | "success" | "muted";

const toneClass: Record<BadgeTone, string> = {
  default: "border-border bg-card text-foreground",
  success: "border-emerald-200 bg-emerald-50 text-emerald-700",
  muted: "border-border bg-muted text-muted-foreground",
};

export function Badge({ children, tone = "default" }: { children: ReactNode; tone?: BadgeTone }) {
  return <span className={`inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-medium ${toneClass[tone]}`}>{children}</span>;
}
```

- [ ] **Step 3: Create link-button helper**

Create `apps/web/components/ui/button-link.tsx`:

```tsx
import Link from "next/link";
import type { ComponentPropsWithoutRef, ReactNode } from "react";

type ButtonVariant = "primary" | "secondary" | "ghost";

const variantClass: Record<ButtonVariant, string> = {
  primary: "bg-primary text-primary-foreground shadow-sm hover:bg-[#254d42]",
  secondary: "border border-border bg-card text-foreground hover:bg-muted",
  ghost: "text-muted-foreground hover:bg-muted hover:text-foreground",
};

type ButtonLinkProps = ComponentPropsWithoutRef<typeof Link> & {
  children: ReactNode;
  variant?: ButtonVariant;
};

export function ButtonLink({ children, className = "", variant = "primary", ...props }: ButtonLinkProps) {
  return (
    <Link className={`inline-flex items-center justify-center rounded-xl px-4 py-2 text-sm font-medium transition ${variantClass[variant]} ${className}`} {...props}>
      {children}
    </Link>
  );
}
```

- [ ] **Step 4: Create presentation helpers**

Create `apps/web/lib/presentation.ts`:

```ts
import type { Material } from "@/lib/api";

export const materialTypeLabel: Record<string, string> = {
  knowledge_note: "讲义",
  mock_paper: "模拟卷",
  answer: "答案解析",
  quick_review: "考前速背",
  past_exam: "历年真题",
  other: "资料",
};

export const accessLevelLabel: Record<string, string> = {
  free: "免费",
  login_required: "登录可下载",
  paid: "付费资料",
  member_only: "会员资料",
};

export function formatFileSize(bytes?: number) {
  if (!bytes || bytes <= 0) return "大小未知";
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function labelMaterialType(type: string) {
  return materialTypeLabel[type] ?? type;
}

export function labelAccessLevel(level: string) {
  return accessLevelLabel[level] ?? level;
}

export function summarizeMaterialTypes(materials: Material[]) {
  const counts = new Map<string, number>();
  for (const material of materials) {
    const label = labelMaterialType(material.type);
    counts.set(label, (counts.get(label) ?? 0) + 1);
  }
  return Array.from(counts.entries()).map(([label, count]) => ({ label, count }));
}

export function latestUpdatedAt(materials: Material[]) {
  const dates = materials
    .map((material) => material.updatedAt)
    .filter((value): value is string => Boolean(value))
    .map((value) => new Date(value))
    .filter((value) => !Number.isNaN(value.getTime()));
  if (!dates.length) return "持续维护";
  const latest = new Date(Math.max(...dates.map((date) => date.getTime())));
  return latest.toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" });
}
```

- [ ] **Step 5: Verify helpers compile**

Run:

```bash
npm --workspace @final-review/web run lint
```

Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add apps/web/components/layout/site-shell.tsx apps/web/components/ui/badge.tsx apps/web/components/ui/button-link.tsx apps/web/lib/presentation.ts
git commit -m "feat(web): add course material UI helpers"
```

---

## Task 3: Student Home and Course Catalog

**Files:**
- Modify: `apps/web/app/page.tsx`
- Modify: `apps/web/app/courses/page.tsx`

- [ ] **Step 1: Replace homepage with compact material-library hero**

Use `SiteShell`, `ButtonLink`, `Badge`, and lucide icons. The homepage must include these visible texts:

```tsx
软件学院课程资料库
按课程整理 PDF 资料，稳定供应，下载带轻水印。
进入课程资料库
查看资料保障
最近更新资料
资料保障
按课程整理
PDF 稳定下载
轻水印
持续维护
```

The first viewport must show at least part of the course/material entry area. Do not add a full-screen hero.

- [ ] **Step 2: Update course catalog copy and card structure**

In `apps/web/app/courses/page.tsx`, remove school/major filtering copy. Use this page framing:

```tsx
<p className="text-sm font-medium text-primary">课程资料库</p>
<h1 className="mt-2 text-3xl font-semibold tracking-tight">按课程查找软件学院 PDF 资料</h1>
<p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
  当前资料按课程组织，重点展示讲义、真题、解析和考前速背。后续课程社区会围绕资料补充建议和讨论。
</p>
```

Each course card should include:

```tsx
<Badge tone="success">资料保障</Badge>
<span>{course.grade || "适用年级待补充"}</span>
<p>{course.description || "这门课程的 PDF 资料会按讲义、真题、解析等类型持续整理。"}</p>
<span>PDF 资料入口</span>
<span>轻水印下载</span>
```

- [ ] **Step 3: Verify student catalog build**

Run:

```bash
npm --workspace @final-review/web run build
```

Expected: Next build exits 0. Routes `/` and `/courses` remain present in the route output.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/page.tsx apps/web/app/courses/page.tsx
git commit -m "feat(web): refresh material library landing pages"
```

---

## Task 4: Student Course and Material Detail Pages

**Files:**
- Modify: `apps/web/app/courses/[id]/page.tsx`
- Modify: `apps/web/app/materials/[id]/page.tsx`

- [ ] **Step 1: Make course detail PDF-first**

In `apps/web/app/courses/[id]/page.tsx`, keep the existing API calls. Change the primary copy so it emphasizes material access, not quiz practice. The top action should be a link back to `/courses`; if a quiz link remains, make it secondary text such as `题目练习`.

Include these sections:

```tsx
<Badge tone="success">资料保障</Badge>
<h1>{course.name}</h1>
<p>{course.description || "这门课程的 PDF 资料会持续整理，下载时附带不影响阅读的轻水印。"}</p>
<h2>PDF 资料</h2>
<h2>资料保障</h2>
<h2>课程社区预留</h2>
```

For material cards, display type and access labels via `labelMaterialType` and `labelAccessLevel`.

- [ ] **Step 2: Make material detail download-focused**

In `apps/web/app/materials/[id]/page.tsx`, keep `href={`${apiBaseUrl()}/materials/${material.id}/download`}`. Use `formatFileSize`, `labelMaterialType`, and `labelAccessLevel`. Include the visible guarantee copy:

```tsx
下载前说明
PDF 稳定供应
轻水印不影响阅读
下载权限由服务端校验
```

Do not mention `storage_key` or local paths.

- [ ] **Step 3: Verify detail pages build**

Run:

```bash
npm --workspace @final-review/web run build
```

Expected: Next build exits 0. Dynamic routes `/courses/[id]` and `/materials/[id]` remain dynamic in route output.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/courses/[id]/page.tsx apps/web/app/materials/[id]/page.tsx
git commit -m "feat(web): make detail pages material focused"
```

---

## Task 5: Student Login Polish

**Files:**
- Modify: `apps/web/app/login/page.tsx`
- Modify: `apps/web/components/auth/login-form.tsx`

- [ ] **Step 1: Wrap login page in the student shell**

Use `SiteShell` and replace the current single card with a centered shadcn-like login panel. Keep `LoginForm` unchanged in behavior.

Required visible copy:

```tsx
学生登录
登录后可下载需要权限的 PDF 资料，并获得轻水印下载记录。
```

- [ ] **Step 2: Polish `LoginForm` styles only**

In `apps/web/components/auth/login-form.tsx`, preserve request paths and state logic. Update class names so inputs use:

```tsx
"mt-2 w-full rounded-xl border border-border bg-card px-3 py-2 text-sm shadow-sm"
```

Buttons should use primary/secondary shadcn-like classes. Do not change `/auth/send-code`, `/auth/login`, `/auth/me`, or `/auth/logout` paths in this task.

- [ ] **Step 3: Verify login compiles**

Run:

```bash
npm --workspace @final-review/web run lint
```

Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/login/page.tsx apps/web/components/auth/login-form.tsx
git commit -m "feat(web): polish student login UI"
```

---

## Task 6: Admin Design Tokens and Shell

**Files:**
- Modify: `apps/admin/src/styles/main.css`
- Modify: `apps/admin/src/components/AdminShell.vue`

- [ ] **Step 1: Add admin CSS variables and Element Plus overrides**

At the top of `apps/admin/src/styles/main.css`, add:

```css
:root {
  --fr-bg: #f7f8f5;
  --fr-panel: #ffffff;
  --fr-text: #18181b;
  --fr-muted: #6b7280;
  --fr-border: #dfe4dc;
  --fr-primary: #2f5f51;
  --fr-primary-soft: #edf6f1;
  --fr-radius: 12px;
  --el-color-primary: #2f5f51;
  --el-border-radius-base: 10px;
}
```

Update existing admin CSS to use those variables. Add:

```css
.el-card {
  border-color: var(--fr-border);
  border-radius: var(--fr-radius);
}

.el-button {
  border-radius: 10px;
}

.el-table {
  --el-table-border-color: var(--fr-border);
  --el-table-header-bg-color: #fafbf8;
}
```

- [ ] **Step 2: Refresh admin shell markup**

In `apps/admin/src/components/AdminShell.vue`, use this navigation vocabulary:

```vue
<strong>资料运营工作台</strong>
<span class="muted">软件学院课程资料库</span>
<RouterLink to="/dashboard">运营概览</RouterLink>
<RouterLink to="/courses">课程维护</RouterLink>
<RouterLink to="/materials">PDF 资料</RouterLink>
<span class="nav-section">预留能力</span>
<span class="disabled-nav">内容审核</span>
<span class="disabled-nav">课程社区</span>
```

Preserve `handleLogout` behavior.

- [ ] **Step 3: Verify admin shell compiles**

Run:

```bash
npm --workspace @final-review/admin run lint
```

Expected: `vue-tsc --noEmit` exits 0.

- [ ] **Step 4: Commit**

```bash
git add apps/admin/src/styles/main.css apps/admin/src/components/AdminShell.vue
git commit -m "feat(admin): refresh operations shell"
```

---

## Task 7: Admin Dashboard, Courses, Materials, and Login

**Files:**
- Modify: `apps/admin/src/views/DashboardView.vue`
- Modify: `apps/admin/src/views/CoursesView.vue`
- Modify: `apps/admin/src/views/MaterialsView.vue`
- Modify: `apps/admin/src/views/LoginView.vue`

- [ ] **Step 1: Refresh dashboard content**

In `DashboardView.vue`, use stat labels:

```ts
const stats = [
  { label: "课程入口", value: "按课程组织" },
  { label: "PDF 资料", value: "稳定供应" },
  { label: "资料保障", value: "轻水印" },
];
```

Use rows:

```ts
const rows = [
  { module: "课程资料库", status: "主线", note: "学生按课程进入 PDF 资料。" },
  { module: "资料运营", status: "已接入", note: "支持课程维护、资料上传和归档。" },
  { module: "内容审核", status: "预留", note: "后续用于 AI 草稿和社区内容审核。" },
  { module: "课程社区", status: "预留", note: "后续围绕课程资料讨论和补充建议。" },
];
```

- [ ] **Step 2: Refresh courses view language**

In `CoursesView.vue`, change page title to `课程维护`. Change description to:

```vue
<p class="muted">课程是资料库的一级入口。当前只服务软件学院，不把院校筛选作为前台主路径。</p>
```

Keep school/college/major form fields because backend requires IDs. Add helper copy near the form:

```vue
<p class="muted">组织字段用于入库关联，学生端展示时会弱化院校筛选。</p>
```

- [ ] **Step 3: Refresh materials view language**

In `MaterialsView.vue`, change page title to `PDF 资料`. Change description to:

```vue
<p class="muted">上传课程 PDF 资料，并保持资料类型、权限和预览内容清晰。下载仍由服务端完成权限检查和轻水印处理。</p>
```

Change upload card title to `上传 PDF 资料`. Keep accepted file types unchanged.

- [ ] **Step 4: Refresh admin login language**

In `LoginView.vue`, change header to `资料运营工作台登录`. Change description to:

```vue
<p class="muted">管理员登录后可以维护课程、上传 PDF 资料，并管理资料保障相关状态。</p>
```

Preserve auth logic and admin role check.

- [ ] **Step 5: Verify admin build**

Run:

```bash
npm --workspace @final-review/admin run build
```

Expected: exits 0. A chunk size warning is acceptable if the build exits 0.

- [ ] **Step 6: Commit**

```bash
git add apps/admin/src/views/DashboardView.vue apps/admin/src/views/CoursesView.vue apps/admin/src/views/MaterialsView.vue apps/admin/src/views/LoginView.vue
git commit -m "feat(admin): align operations views with material library"
```

---

## Task 8: Full Verification and Visual QA

**Files:**
- Read: `docs/superpowers/specs/2026-06-22-course-material-library-ui-design.md`
- Verify all modified frontend files.

- [ ] **Step 1: Run full lint**

```bash
npm run lint
```

Expected: web `tsc --noEmit` and admin `vue-tsc --noEmit` both exit 0.

- [ ] **Step 2: Run full test/build**

```bash
npm test
```

Expected: web Next build exits 0 and admin Vite build exits 0. Admin chunk warning is acceptable.

- [ ] **Step 3: Run git diff check**

```bash
git diff --check
```

Expected: no whitespace errors.

- [ ] **Step 4: Manual UI checklist**

Start the student app in one terminal:

```bash
npm --workspace @final-review/web run dev
```

Expected: Next dev server serves `http://localhost:3000`.

Start the admin app in a second terminal:

```bash
npm --workspace @final-review/admin run dev
```

Expected: Vite dev server serves `http://localhost:5173`.

If the Go API is not running, API-backed pages may show API unavailable states; still verify layout, hierarchy, copy, and empty/error states. If full data QA is required, run:

```bash
docker compose -f docker-compose.dev.yml up --build
```

Verify:

```txt
Student home:
- First viewport shows "软件学院课程资料库".
- First viewport exposes course/material entry.
- Main CTA is about course materials, not AI or quiz.

Student course pages:
- Course cards and material cards emphasize PDF, update, type, and guarantee.
- Quiz link is secondary if present.
- No copy promises 必考命中, 老师原版, AI 精准总结, or 全国高校覆盖.

Admin:
- Sidebar says 资料运营工作台.
- Dashboard communicates courses, PDF materials, and guarantee.
- Courses and materials forms remain usable.
- Login still enforces admin/super_admin through existing store logic.
```

- [ ] **Step 5: Final commit if visual QA fixes were needed**

If Step 4 required fixes:

```bash
git add apps/web apps/admin
git commit -m "fix: polish course material UI QA issues"
```

If no fixes were needed, do not create an empty commit.

---

## Self-Review Notes

- Spec coverage: The plan covers shared design language, student homepage, course catalog, course detail, material detail, login, admin shell, dashboard, course management, material management, dependency boundaries, and verification.
- Scope boundaries: The plan does not modify backend services, payment integration, real AI behavior, or storage/watermark implementation.
- Type consistency: The plan reuses existing `Course` and `Material` types and adds presentation helpers only for labels, formatting, and derived display summaries.
- Placeholder scan: No task depends on TBD fields or unchosen blocks. shadcn blocks are references only; React blocks are not used in Vue admin.
