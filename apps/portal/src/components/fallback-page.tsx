import type { ReactNode } from "react";
import SiteShell from "@/components/site-shell";

/**
 * 404 与出错兜底页的共用版式：图纸网格上一行 mono 编号、标题、说明，下面是出路。
 * 兜底页替换掉了出事那一层的布局（子站导航和页脚都不在了），所以全站页脚由这里的
 * SiteShell 重新带上。
 */
export default function FallbackPage({
  code,
  label,
  title,
  description,
  children,
}: {
  code: string;
  label: string;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <SiteShell className="bg-blueprint">
      <main className="mx-auto max-w-site px-5 py-16 md:px-8 md:py-24">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">{code}</span>
          <span className="mx-2">/</span>
          {label}
        </p>
        <h1 className="mt-4 font-display text-4xl font-bold tracking-tight md:text-6xl">{title}</h1>
        <div aria-hidden className="mt-6 h-px w-24 bg-accent" />
        <p className="mt-5 max-w-xl text-base leading-7 text-ink/70">{description}</p>
        {children}
      </main>
    </SiteShell>
  );
}
