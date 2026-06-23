"use client";

import { useState } from "react";
import Link from "next/link";
import type { LucideIcon } from "lucide-react";
import {
  Archive,
  Bell,
  BookOpen,
  Boxes,
  ChevronsLeft,
  ChevronsRight,
  ChevronsUpDown,
  ChevronRight,
  CircleHelp,
  ClipboardList,
  Cpu,
  Database,
  Download,
  Droplets,
  FileArchive,
  FileText,
  Filter,
  FlaskConical,
  GraduationCap,
  Library,
  Menu,
  Network,
  PanelLeft,
  Presentation,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  Upload,
  UsersRound,
  Wrench,
} from "lucide-react";
import { Button } from "@/components/ui/button";

type NavItem = {
  label: string;
  href: string;
  icon: LucideIcon;
  active?: boolean;
  count?: number;
};

type NavSection = {
  title: string;
  items: NavItem[];
};

type TodayMaterial = {
  title: string;
  meta: string;
  icon: LucideIcon;
  tone: "pdf" | "archive" | "note";
};

type CourseModule = {
  title: string;
  subtitle: string;
  files: number;
  code: string;
  icon: LucideIcon;
};

type DocumentRow = {
  title: string;
  course: string;
  category: string;
  date: string;
  tags: Array<"轻水印" | "稳定供应" | "已归档" | "本周更新">;
};

const navSections: NavSection[] = [
  {
    title: "资料分类",
    items: [
      { label: "核心课程", href: "/courses", icon: BookOpen, active: true, count: 24 },
      { label: "选修课程", href: "/courses", icon: GraduationCap, count: 11 },
      { label: "实验资料", href: "/courses", icon: FlaskConical, count: 36 },
      { label: "历年真题", href: "/courses", icon: ClipboardList, count: 38 },
      { label: "复习笔记", href: "/courses", icon: FileText, count: 52 },
      { label: "课件讲义", href: "/courses", icon: Presentation, count: 63 },
    ],
  },
  {
    title: "核心课程",
    items: [
      { label: "数据结构", href: "/courses", icon: Database, active: true, count: 142 },
      { label: "操作系统", href: "/courses", icon: Cpu, count: 89 },
      { label: "计算机网络", href: "/courses", icon: Network, count: 115 },
      { label: "软件工程", href: "/courses", icon: Boxes, count: 76 },
    ],
  },
];

const todayMaterials: TodayMaterial[] = [
  {
    title: "数据结构期末真题解析.pdf",
    meta: "真题解析 / 2 小时前查看",
    icon: FileText,
    tone: "pdf",
  },
  {
    title: "OS_Process_Scheduling_Lab.zip",
    meta: "实验资料 / 昨日更新",
    icon: FileArchive,
    tone: "archive",
  },
  {
    title: "软件工程复习讲义.pdf",
    meta: "复习讲义 / 稳定供应",
    icon: FileText,
    tone: "note",
  },
  {
    title: "计算机网络协议分层速查.pdf",
    meta: "速查资料 / 本周新增",
    icon: Library,
    tone: "pdf",
  },
];

const statusPanels = [
  {
    title: "稳定供应",
    subtitle: "可下载资料优先维护",
    icon: Archive,
    marker: ShieldCheck,
  },
  {
    title: "轻水印",
    subtitle: "下载前统一加轻水印",
    icon: Droplets,
    marker: CircleHelp,
  },
  {
    title: "持续维护",
    subtitle: "资料缺口持续补齐",
    icon: RefreshCw,
    marker: Wrench,
  },
];

const courseModules: CourseModule[] = [
  {
    title: "数据结构",
    subtitle: "Data Structures",
    files: 142,
    code: "CS201",
    icon: Database,
  },
  {
    title: "操作系统",
    subtitle: "Operating Systems",
    files: 89,
    code: "CS301",
    icon: Cpu,
  },
  {
    title: "计算机网络",
    subtitle: "Computer Networks",
    files: 115,
    code: "CS302",
    icon: Network,
  },
];

const documentRows: DocumentRow[] = [
  {
    title: "数据结构期末真题解析",
    course: "数据结构",
    category: "Exam",
    date: "2023-12-01",
    tags: ["轻水印", "稳定供应"],
  },
  {
    title: "操作系统考前速背清单",
    course: "操作系统",
    category: "Note",
    date: "2023-11-28",
    tags: ["本周更新"],
  },
  {
    title: "计算机网络协议分层讲义",
    course: "计算机网络",
    category: "Slide",
    date: "2023-10-15",
    tags: ["稳定供应"],
  },
  {
    title: "软件工程复习讲义",
    course: "软件工程",
    category: "Note",
    date: "2023-09-02",
    tags: ["轻水印", "已归档"],
  },
];

const mobileCategories = navSections[0].items.slice(0, 4);

const sidebarQuickLinks = [
  { label: "资料补充建议", href: "/courses", icon: Upload, detail: "缺失资料反馈" },
  { label: "我的下载", href: "/me/downloads", icon: Download, detail: "下载记录与资料回看" },
  { label: "维护状态", href: "/courses", icon: ShieldCheck, detail: "稳定供应记录" },
  { label: "账号设置", href: "/login", icon: Settings, detail: "登录与通知" },
];

function IconBlock({ icon: Icon, className = "" }: { icon: LucideIcon; className?: string }) {
  return (
    <span
      className={`grid size-8 shrink-0 place-items-center rounded-md border border-border bg-secondary text-primary ${className}`}
    >
      <Icon className="size-4" aria-hidden="true" />
    </span>
  );
}

function Sidebar({
  collapsed,
  onToggle,
}: {
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <aside
      className={[
        "relative hidden min-h-[100dvh] border-r border-border bg-card/70 backdrop-blur-md transition-[width] duration-200 lg:flex lg:flex-col",
        collapsed ? "w-[76px]" : "w-64",
      ].join(" ")}
    >
      <div className="border-b border-border p-3">
        <div
          className={[
            "archive-panel-soft flex items-center rounded-xl p-1.5",
            collapsed ? "justify-center" : "gap-2",
          ].join(" ")}
        >
          <Link
            className={[
              "flex min-w-0 items-center gap-2 rounded-md transition hover:bg-card",
              collapsed ? "justify-center p-1" : "flex-1 px-1.5 py-1",
            ].join(" ")}
            href="/"
            title="软件学院资料库"
          >
            <span className="grid size-8 shrink-0 place-items-center rounded-md bg-primary text-primary-foreground">
              <BookOpen className="size-4" aria-hidden="true" />
            </span>
            {!collapsed ? (
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold leading-none">软件学院资料库</span>
                <span className="mt-1 block truncate font-mono text-[10px] text-muted-foreground">SE Material Library</span>
              </span>
            ) : null}
          </Link>
          {!collapsed ? (
            <button
              aria-label="切换资料库暂未开放"
              className="grid size-8 cursor-not-allowed place-items-center rounded-md text-muted-foreground opacity-60"
              disabled
              type="button"
            >
              <ChevronsUpDown className="size-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </div>

      <button
        aria-label={collapsed ? "展开侧栏" : "收起侧栏"}
        className="archive-panel absolute -right-3 top-[74px] z-10 grid size-6 place-items-center rounded-full text-muted-foreground transition hover:border-primary hover:text-primary"
        onClick={onToggle}
        type="button"
      >
        {collapsed ? <ChevronsRight className="size-3.5" aria-hidden="true" /> : <ChevronsLeft className="size-3.5" aria-hidden="true" />}
      </button>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 py-4">
        {navSections.map((section) => (
          <section key={section.title} className={collapsed ? "mb-5" : "mb-6"}>
            {!collapsed ? (
              <h2 className="mb-2 px-2 text-xs font-semibold text-muted-foreground">
                {section.title}
              </h2>
            ) : null}
            <div className="grid gap-1">
              {section.items.map((item) => {
                const Icon = item.icon;

                return (
                  <Link
                    key={`${section.title}-${item.label}`}
                    className={[
                      "group relative flex min-h-9 items-center rounded px-2.5 py-2 text-sm transition",
                      collapsed ? "justify-center px-0" : "gap-3",
                      item.active
                        ? "bg-primary/10 font-semibold text-primary"
                        : "text-muted-foreground hover:bg-secondary hover:text-foreground",
                    ].join(" ")}
                    href={item.href}
                    title={collapsed ? item.label : undefined}
                  >
                    {item.active ? (
                      <span
                        className={[
                          "absolute rounded-full bg-primary",
                          collapsed ? "left-1 top-2 h-5 w-0.5" : "right-0 top-2 h-5 w-0.5",
                        ].join(" ")}
                      />
                    ) : null}
                    <Icon className="size-4 shrink-0" aria-hidden="true" />
                    {!collapsed ? (
                      <>
                        <span className="min-w-0 flex-1 truncate">{item.label}</span>
                        {item.count ? (
                          <span className="font-mono text-[11px] text-muted-foreground group-hover:text-foreground">
                            {item.count}
                          </span>
                        ) : null}
                      </>
                    ) : null}
                  </Link>
                );
              })}
            </div>
          </section>
        ))}

        {!collapsed ? (
          <section className="mb-2">
            <h2 className="mb-2 px-2 text-xs font-semibold text-muted-foreground">常用入口</h2>
            <div className="grid gap-1">
              {sidebarQuickLinks.map((item) => {
                const Icon = item.icon;

                return (
                  <Link
                    key={item.label}
                    className="group flex min-h-10 items-center gap-3 rounded px-2.5 py-2 text-sm text-muted-foreground transition hover:bg-secondary hover:text-foreground"
                    href={item.href}
                  >
                    <Icon className="size-4 shrink-0" aria-hidden="true" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{item.label}</span>
                      <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{item.detail}</span>
                    </span>
                    <ChevronRight className="size-3.5 opacity-0 transition group-hover:translate-x-0.5 group-hover:opacity-100" aria-hidden="true" />
                  </Link>
                );
              })}
            </div>
          </section>
        ) : null}
      </nav>

      <div className="border-t border-border p-2">
        {!collapsed ? (
          <div className="archive-panel-soft rounded-xl p-2">
            <div className="mb-2 flex items-center justify-between gap-2">
              <span className="text-xs font-semibold">资料保障</span>
              <span className="rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[10px] text-primary">online</span>
            </div>
            <p className="text-xs leading-5 text-muted-foreground">下载入口、轻水印和资料补齐状态统一维护。</p>
          </div>
        ) : null}
        <Link
          className={[
            "archive-panel-soft mt-2 flex items-center rounded-xl text-muted-foreground transition hover:border-primary hover:text-primary",
            collapsed ? "justify-center p-2" : "gap-3 p-2",
          ].join(" ")}
          href="/login"
          title="学生账号"
        >
          <span className="grid size-8 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">N</span>
          {!collapsed ? (
            <>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium text-foreground">学生账号</span>
                <span className="mt-0.5 block truncate font-mono text-[10px]">登录后同步下载记录</span>
              </span>
              <ChevronsUpDown className="size-4 shrink-0" aria-hidden="true" />
            </>
          ) : null}
        </Link>
      </div>
    </aside>
  );
}

function DesktopTopBar() {
  return (
    <header className="sticky top-0 z-20 hidden h-[64px] items-center justify-between border-b border-border bg-card/70 px-6 backdrop-blur-md lg:flex">
      <div className="relative w-full max-w-md">
        <label className="sr-only" htmlFor="workspace-desktop-search">
          搜索课程、讲义、真题、实验资料
        </label>
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
        <input
          className="h-9 w-full rounded-lg border border-input bg-card/70 px-9 font-mono text-[12px] text-foreground shadow-[0_10px_30px_hsl(58_12%_44%/0.06)] outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
          id="workspace-desktop-search"
          name="q"
          placeholder="搜索课程、讲义、真题、实验资料..."
          type="search"
        />
      </div>

      <nav className="mx-6 flex items-center gap-6 font-mono text-[12px]">
        <Link className="border-b-2 border-primary pb-1 font-semibold text-primary" href="/">
          资料库
        </Link>
        <Link className="pb-1 text-muted-foreground hover:text-primary" href="/courses">
          课程
        </Link>
        <Link className="pb-1 text-muted-foreground hover:text-primary" href="#documents">
          资料
        </Link>
        <Link className="pb-1 text-muted-foreground hover:text-primary" href="/me/downloads">
          我的下载
        </Link>
      </nav>

      <div className="flex items-center gap-2">
        <button
          className="grid size-8 cursor-not-allowed place-items-center rounded text-muted-foreground opacity-60"
          aria-label="通知暂未开放"
          disabled
          type="button"
        >
          <Bell className="size-4" aria-hidden="true" />
        </button>
        <button
          className="grid size-8 cursor-not-allowed place-items-center rounded text-muted-foreground opacity-60"
          aria-label="帮助暂未开放"
          disabled
          type="button"
        >
          <CircleHelp className="size-4" aria-hidden="true" />
        </button>
        <Link
          className="grid size-8 place-items-center rounded text-muted-foreground hover:bg-secondary hover:text-primary"
          href="/me/downloads"
          aria-label="我的下载"
        >
          <Download className="size-4" aria-hidden="true" />
        </Link>
        <Button asChild size="sm" className="h-8 rounded px-3 text-xs">
          <Link href="/login">登录</Link>
        </Button>
      </div>
    </header>
  );
}

function MobileHeader() {
  return (
    <header className="sticky top-0 z-30 border-b border-border bg-card/75 px-4 py-3 backdrop-blur-md lg:hidden">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <button
            className="grid size-9 cursor-not-allowed place-items-center rounded border border-border text-muted-foreground opacity-60"
            aria-label="打开菜单暂未开放"
            disabled
            type="button"
          >
            <Menu className="size-4" aria-hidden="true" />
          </button>
          <div className="min-w-0">
            <p className="truncate font-heading text-lg font-semibold leading-none">软件学院资料库</p>
            <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">SE Material Library</p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Link
            className="grid size-8 place-items-center rounded border border-border text-muted-foreground hover:bg-secondary hover:text-primary"
            href="/me/downloads"
            aria-label="我的下载"
          >
            <Download className="size-4" aria-hidden="true" />
          </Link>
          <Button asChild size="sm" className="h-8 rounded px-3 text-xs">
            <Link href="/login">登录</Link>
          </Button>
        </div>
      </div>
      <div className="relative mt-3">
        <label className="sr-only" htmlFor="workspace-mobile-search">
          搜索课程、讲义、真题
        </label>
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
        <input
          className="h-10 w-full rounded-lg border border-input bg-card/70 px-9 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
          id="workspace-mobile-search"
          name="q"
          placeholder="搜索课程、讲义、真题"
          type="search"
        />
      </div>
    </header>
  );
}

function Breadcrumb() {
  return (
    <div className="hidden items-center gap-1.5 font-mono text-[12px] text-muted-foreground lg:flex">
      <PanelLeft className="size-4" aria-hidden="true" />
      <span>课程资料库</span>
      <ChevronRight className="size-3" aria-hidden="true" />
      <span>核心课程</span>
      <ChevronRight className="size-3" aria-hidden="true" />
      <span className="font-medium text-foreground">数据结构</span>
    </div>
  );
}

function TodayMaterials() {
  return (
    <section className="archive-panel overflow-hidden rounded-xl lg:col-span-8">
      <div className="flex items-center justify-between border-b border-border bg-secondary/70 px-4 py-3">
        <h1 className="flex items-center gap-2 font-heading text-2xl font-semibold tracking-tight sm:text-[28px]">
          <span className="grid size-7 place-items-center rounded bg-primary/10 text-primary">
            <Library className="size-4" aria-hidden="true" />
          </span>
          今日可用资料
        </h1>
        <span className="hidden rounded-lg border border-border bg-card/80 px-2 py-1 text-xs text-muted-foreground sm:inline-flex">
          最近访问
        </span>
      </div>
      <div className="grid gap-2 p-3 sm:grid-cols-2">
        {todayMaterials.map((material) => {
          const Icon = material.icon;
          const iconTone =
            material.tone === "archive"
              ? "text-[#9a7351]"
              : material.tone === "note"
                ? "text-primary"
                : "text-[#a85f55]";

          return (
            <Link
              key={material.title}
              className="group flex min-w-0 items-start gap-3 rounded border border-transparent p-2 transition hover:border-border hover:bg-secondary"
              href="/courses"
            >
              <Icon className={`mt-0.5 size-4 shrink-0 ${iconTone}`} aria-hidden="true" />
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium group-hover:text-primary">{material.title}</span>
                <span className="mt-1 block truncate font-mono text-[11px] text-muted-foreground">{material.meta}</span>
              </span>
            </Link>
          );
        })}
      </div>
    </section>
  );
}

function StatusPanels() {
  return (
    <section className="grid gap-2 lg:col-span-4">
      {statusPanels.map((panel) => {
        const Icon = panel.icon;
        const Marker = panel.marker;

        return (
          <div key={panel.title} className="archive-panel flex items-center justify-between rounded-xl p-3">
            <div className="flex min-w-0 items-center gap-3">
              <IconBlock icon={Icon} />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{panel.title}</p>
                <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">{panel.subtitle}</p>
              </div>
            </div>
            <Marker className="size-4 shrink-0 text-primary" aria-hidden="true" />
          </div>
        );
      })}
    </section>
  );
}

function CourseGrid() {
  return (
    <section className="mt-2 lg:col-span-12">
      <div className="mb-2 flex items-center justify-between px-1">
        <h2 className="text-xs font-semibold text-muted-foreground">核心课程</h2>
        <Link className="text-xs font-medium text-primary hover:underline" href="/courses">
          查看全部
        </Link>
      </div>
      <div className="grid gap-3 md:grid-cols-3">
        {courseModules.map((course) => {
          const Icon = course.icon;

          return (
            <Link
              key={course.title}
              className="archive-panel group overflow-hidden rounded-xl transition hover:-translate-y-px hover:border-primary"
              href="/courses"
            >
              <div className="flex items-start justify-between border-b border-border bg-secondary/70 p-4">
                <div>
                  <h3 className="font-heading text-[22px] font-semibold leading-tight group-hover:text-primary">{course.title}</h3>
                  <p className="mt-2 font-mono text-[11px] text-muted-foreground">{course.subtitle}</p>
                </div>
                <Icon className="size-5 text-muted-foreground group-hover:text-primary" aria-hidden="true" />
              </div>
              <div className="flex items-center justify-between p-3">
                <span className="rounded-lg border border-border bg-secondary/70 px-2 py-1 font-mono text-[11px] text-muted-foreground">
                  {course.code}
                </span>
                <span className="rounded-lg border border-border bg-secondary/70 px-2 py-1 font-mono text-[11px] text-foreground">
                  {course.files} Files
                </span>
              </div>
            </Link>
          );
        })}
      </div>
    </section>
  );
}

function Tag({ value }: { value: DocumentRow["tags"][number] }) {
  const className = {
    轻水印: "border-border bg-secondary text-muted-foreground",
    稳定供应: "border-primary/30 bg-primary/10 text-primary",
    已归档: "border-border bg-secondary text-muted-foreground",
    本周更新: "border-[#b79a64]/40 bg-[#efe5d1] text-[#83663a]",
  }[value];

  return <span className={`rounded px-1.5 py-0.5 font-mono text-[10px] ring-1 ring-inset ${className}`}>{value}</span>;
}

function DocumentRepository() {
  return (
    <section id="documents" className="archive-panel overflow-hidden rounded-xl lg:col-span-12">
      <div className="flex items-center justify-between border-b border-border bg-secondary/70 px-4 py-3">
        <h2 className="text-xs font-semibold text-foreground">资料归档</h2>
        <button
          className="inline-flex h-8 cursor-not-allowed items-center gap-1.5 rounded-lg border border-border bg-card/80 px-2 text-xs text-muted-foreground opacity-60"
          aria-label="筛选暂未开放"
          disabled
          type="button"
        >
          <Filter className="size-3.5" aria-hidden="true" />
          筛选
        </button>
      </div>

      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[820px] border-collapse text-left text-sm">
          <thead>
            <tr className="border-b border-border bg-secondary/70 text-xs text-muted-foreground">
              <th className="w-12 px-4 py-2 font-medium">类型</th>
              <th className="px-4 py-2 font-medium">资料标题</th>
              <th className="px-4 py-2 font-medium">课程</th>
              <th className="px-4 py-2 font-medium">分类</th>
              <th className="px-4 py-2 font-medium">日期</th>
              <th className="px-4 py-2 font-medium">标签</th>
              <th className="px-4 py-2 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {documentRows.map((document) => (
              <tr key={`${document.course}-${document.title}`} className="group transition hover:bg-secondary">
                <td className="px-4 py-3">
                  <FileText className="size-4 text-[#a85f55]" aria-hidden="true" />
                </td>
                <td className="px-4 py-3 font-medium">{document.title}</td>
                <td className="px-4 py-3 font-mono text-[11px] text-muted-foreground">{document.course}</td>
                <td className="px-4 py-3 font-mono text-[11px] text-muted-foreground">{document.category}</td>
                <td className="px-4 py-3 font-mono text-[11px] text-muted-foreground">{document.date}</td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-1">
                    {document.tags.map((tag) => (
                      <Tag key={`${document.title}-${tag}`} value={tag} />
                    ))}
                  </div>
                </td>
                <td className="px-4 py-3 text-right">
                  <Link className="inline-grid size-7 place-items-center rounded text-muted-foreground hover:bg-card hover:text-primary" href="/courses" aria-label={`下载 ${document.title}`}>
                    <Download className="size-4" aria-hidden="true" />
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="grid gap-1 p-2 md:hidden">
        {documentRows.map((document) => (
          <Link key={`${document.course}-${document.title}`} className="rounded border border-transparent p-2 transition hover:border-border hover:bg-secondary" href="/courses">
            <div className="flex min-w-0 items-start justify-between gap-3">
              <div className="flex min-w-0 gap-3">
                <FileText className="mt-1 size-4 shrink-0 text-[#a85f55]" aria-hidden="true" />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{document.title}</p>
                  <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">
                    {document.course} / {document.category} / {document.date}
                  </p>
                </div>
              </div>
              <Download className="mt-1 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            </div>
            <div className="mt-2 flex flex-wrap gap-1 pl-7">
              {document.tags.map((tag) => (
                <Tag key={`${document.title}-mobile-${tag}`} value={tag} />
              ))}
            </div>
          </Link>
        ))}
      </div>
    </section>
  );
}

function MobileCategories() {
  return (
    <section className="min-w-0 lg:hidden">
      <h2 className="mb-2 text-xs font-semibold text-muted-foreground">资料分类</h2>
      <div className="flex w-full max-w-full gap-2 overflow-x-auto pb-2">
        {mobileCategories.map((category) => {
          const Icon = category.icon;

          return (
            <Link
              key={category.label}
              className={[
                "flex h-10 shrink-0 items-center gap-2 rounded-full border px-3 text-sm",
                category.active
                  ? "border-primary bg-primary/10 text-primary"
                  : "border-border bg-card/70 text-muted-foreground",
              ].join(" ")}
              href={category.href}
            >
              <Icon className="size-4" aria-hidden="true" />
              {category.label}
            </Link>
          );
        })}
      </div>
    </section>
  );
}

export default function HomePage() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  return (
    <main className="min-h-[100dvh] overflow-x-hidden bg-background text-foreground">
      <MobileHeader />

      <div
        className={[
          "lg:grid lg:min-h-[100dvh] lg:transition-[grid-template-columns] lg:duration-200",
          sidebarCollapsed ? "lg:grid-cols-[76px_minmax(0,1fr)]" : "lg:grid-cols-[256px_minmax(0,1fr)]",
        ].join(" ")}
      >
        <Sidebar collapsed={sidebarCollapsed} onToggle={() => setSidebarCollapsed((value) => !value)} />

        <section className="min-w-0">
          <DesktopTopBar />
          <div className="mx-auto flex w-full max-w-[1440px] flex-col gap-5 px-4 py-5 lg:px-6 lg:py-7">
            <Breadcrumb />
            <MobileCategories />

            <section className="grid grid-cols-1 gap-5 lg:grid-cols-12">
              <TodayMaterials />
              <StatusPanels />
              <CourseGrid />
              <DocumentRepository />
            </section>

            <section className="grid gap-4 pb-6 md:grid-cols-3">
              <div className="archive-panel rounded-xl p-4">
                <IconBlock icon={ShieldCheck} />
                <h2 className="mt-4 font-heading text-xl font-semibold">资料保障在线</h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">PDF 入口保持清晰，下载前统一加轻水印，重点资料优先维护。</p>
              </div>
              <div className="archive-panel rounded-xl p-4">
                <IconBlock icon={BookOpen} />
                <h2 className="mt-4 font-heading text-xl font-semibold">按课程组织</h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">首页只做入口和更新概览，课程页承接讲义、真题、答案解析和实验包。</p>
              </div>
              <div className="archive-panel rounded-xl p-4">
                <IconBlock icon={UsersRound} />
                <h2 className="mt-4 font-heading text-xl font-semibold">社区功能预留</h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">后续讨论围绕资料补充、勘误和课程经验展开，不抢占当前下载主流程。</p>
              </div>
            </section>
          </div>
        </section>
      </div>
    </main>
  );
}
