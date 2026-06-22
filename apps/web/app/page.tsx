import { ArrowRight, FileDown, Library, RefreshCw, ShieldCheck, Sparkles } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";

const guarantees = [
  { label: "按课程整理", icon: Library },
  { label: "PDF 稳定下载", icon: FileDown },
  { label: "轻水印", icon: Sparkles },
  { label: "持续维护", icon: RefreshCw },
];

const recentMaterials = ["软件工程复习讲义", "数据结构真题解析", "操作系统考前速背"];

export default function HomePage() {
  return (
    <SiteShell>
      <section className="grid gap-5 rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6 lg:grid-cols-[1.05fr_0.95fr]">
        <div className="flex flex-col justify-center gap-5">
          <Badge tone="success">资料保障</Badge>
          <div>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">软件学院课程资料库</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
              按课程整理 PDF 资料，稳定供应，下载带轻水印。
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <ButtonLink href="/courses">
              进入课程资料库
              <ArrowRight className="ml-2 size-4" aria-hidden="true" />
            </ButtonLink>
            <ButtonLink href="#guarantee" variant="secondary">
              查看资料保障
            </ButtonLink>
          </div>
        </div>

        <div className="rounded-2xl border border-border bg-background p-4">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-base font-semibold">最近更新资料</h2>
            <Badge tone="muted">PDF</Badge>
          </div>
          <div className="mt-4 grid gap-3">
            {recentMaterials.map((material) => (
              <div key={material} className="flex items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-3">
                <span className="text-sm font-medium">{material}</span>
                <span className="text-xs text-muted-foreground">轻水印下载</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section id="guarantee" className="grid gap-4 md:grid-cols-[0.75fr_1.25fr]">
        <div>
          <p className="text-sm font-medium text-primary">资料保障</p>
          <h2 className="mt-2 text-2xl font-semibold tracking-tight">课程材料入口保持清晰、克制、可维护。</h2>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          {guarantees.map((item) => {
            const Icon = item.icon;
            return (
              <div key={item.label} className="rounded-2xl border border-border bg-card p-4">
                <Icon className="size-5 text-primary" aria-hidden="true" />
                <p className="mt-3 text-sm font-medium">{item.label}</p>
              </div>
            );
          })}
        </div>
      </section>

      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-medium text-primary">课程入口</p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight">按课程进入资料卡片</h2>
          </div>
          <ButtonLink href="/courses" variant="secondary">
            查看课程
            <ShieldCheck className="ml-2 size-4" aria-hidden="true" />
          </ButtonLink>
        </div>
      </section>
    </SiteShell>
  );
}
