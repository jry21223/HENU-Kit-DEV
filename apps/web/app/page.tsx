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
      <section className="grid w-full max-w-full min-w-0 gap-5 rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6 lg:grid-cols-[1.05fr_0.95fr]">
        <div className="flex min-w-0 flex-col justify-center gap-5">
          <Badge tone="success">资料保障</Badge>
          <div className="min-w-0">
            <h1 className="max-w-full break-words text-3xl font-semibold tracking-tight sm:text-4xl">软件学院课程资料库</h1>
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

        <div className="w-full max-w-full min-w-0 rounded-2xl border border-border bg-background p-4">
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
            <h2 className="min-w-0 break-words text-base font-semibold">最近更新资料</h2>
            <Badge tone="muted">PDF</Badge>
          </div>
          <div className="mt-4 grid gap-3">
            {recentMaterials.map((material) => (
              <div
                key={material}
                className="flex w-full max-w-full min-w-0 flex-col items-start gap-1 rounded-2xl border border-border bg-card px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-3"
              >
                <span className="min-w-0 break-words text-sm font-medium sm:flex-1">{material}</span>
                <span className="min-w-0 break-words text-xs text-muted-foreground sm:text-right">轻水印下载</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section id="guarantee" className="grid w-full max-w-full min-w-0 gap-4 md:grid-cols-[0.75fr_1.25fr]">
        <div className="min-w-0">
          <p className="text-sm font-medium text-primary">资料保障</p>
          <h2 className="mt-2 max-w-full break-words text-2xl font-semibold tracking-tight">课程材料入口保持清晰、克制、可维护。</h2>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          {guarantees.map((item) => {
            const Icon = item.icon;
            return (
              <div key={item.label} className="min-w-0 rounded-2xl border border-border bg-card p-4">
                <Icon className="size-5 text-primary" aria-hidden="true" />
                <p className="mt-3 break-words text-sm font-medium">{item.label}</p>
              </div>
            );
          })}
        </div>
      </section>

      <section className="min-w-0 rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className="text-sm font-medium text-primary">课程入口</p>
            <h2 className="mt-2 break-words text-2xl font-semibold tracking-tight">按课程进入资料卡片</h2>
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
