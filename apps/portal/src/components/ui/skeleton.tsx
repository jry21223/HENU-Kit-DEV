import type { ComponentProps } from "react";
import { cn } from "@/lib/cn";

export function Skeleton({ className, ...props }: ComponentProps<"div">) {
  return <div {...props} aria-hidden="true" className={cn("border border-line bg-ink/5 motion-safe:animate-pulse", className)} />;
}
