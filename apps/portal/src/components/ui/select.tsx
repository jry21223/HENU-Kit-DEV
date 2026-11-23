import type { ComponentProps } from "react";
import { cn } from "@/lib/cn";

export function Select({ className, ...props }: ComponentProps<"select">) {
  return (
    <span className="relative inline-flex items-center">
      <select
        className={cn(
          "min-h-11 min-w-11 w-full appearance-none rounded-none border border-ink/30 bg-paper-raised pl-3 pr-9 font-mono text-sm text-ink outline-none transition-colors hover:border-ink focus:border-ink focus-visible:outline-2 focus-visible:outline-focus-ring disabled:cursor-not-allowed disabled:opacity-50",
          className
        )}
        {...props}
      />
      <svg
        aria-hidden="true"
        viewBox="0 0 12 12"
        fill="none"
        className="pointer-events-none absolute right-3 size-3 text-ink/70"
      >
        <path
          d="M2.5 4.25L6 7.75L9.5 4.25"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="square"
          strokeLinejoin="miter"
        />
      </svg>
    </span>
  );
}
