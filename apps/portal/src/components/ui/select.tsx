import type { ComponentProps } from "react";
import { cn } from "@/lib/cn";

export function Select({ className, ...props }: ComponentProps<"select">) {
  return <select className={cn("min-h-11 min-w-11 border border-ink/40 bg-paper px-3 text-sm text-ink focus-visible:outline-2 focus-visible:outline-focus-ring disabled:opacity-50", className)} {...props} />;
}
