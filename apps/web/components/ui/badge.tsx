import type { ReactNode } from "react";

type BadgeTone = "default" | "success" | "muted";

const toneClass: Record<BadgeTone, string> = {
  default: "border-border bg-card text-foreground",
  success: "border-emerald-200 bg-emerald-50 text-emerald-700",
  muted: "border-border bg-muted text-muted-foreground",
};

export function Badge({ children, tone = "default" }: { children: ReactNode; tone?: BadgeTone }) {
  return (
    <span className={`inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-medium ${toneClass[tone]}`}>
      {children}
    </span>
  );
}
