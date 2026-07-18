import { cva, type VariantProps } from "class-variance-authority";
export { default as Badge } from "./Badge.vue";
export const badgeVariants = cva(
  "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring",
  { variants: { variant: {
    default: "border-transparent bg-primary text-primary-foreground",
    secondary: "border-transparent bg-secondary text-secondary-foreground",
    destructive: "border-transparent bg-destructive text-destructive-foreground",
    outline: "text-foreground",
    success: "border-emerald-200 bg-emerald-50 text-emerald-700",
    warning: "border-amber-200 bg-amber-50 text-amber-700",
    muted: "border-slate-200 bg-slate-100 text-slate-600",
  } }, defaultVariants: { variant: "default" } },
);
export type BadgeVariants = VariantProps<typeof badgeVariants>;
