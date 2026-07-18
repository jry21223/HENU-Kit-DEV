import { cva, type VariantProps } from "class-variance-authority";
export { default as Alert } from "./Alert.vue";
export const alertVariants = cva(
  "relative w-full rounded-lg border px-4 py-3 text-sm [&>svg+div]:translate-y-[-3px] [&>svg]:absolute [&>svg]:left-4 [&>svg]:top-4 [&>svg]:text-foreground [&>svg~*]:pl-7",
  { variants: { variant: {
    default: "bg-background text-foreground",
    destructive: "border-destructive/50 text-destructive dark:border-destructive [&>svg]:text-destructive",
    warning: "border-amber-200 bg-amber-50 text-amber-800",
  } }, defaultVariants: { variant: "default" } },
);
export type AlertVariants = VariantProps<typeof alertVariants>;
