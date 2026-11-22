import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const badgeVariants = cva(
  "inline-flex items-center border px-2 py-0.5 font-mono text-xs leading-5",
  {
    variants: {
      variant: {
        default: "border-ink/30 text-ink/70",
        accent: "border-accent text-accent-text",
        ink: "border-ink bg-ink text-paper",
        muted: "border-line text-ink/60",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}
