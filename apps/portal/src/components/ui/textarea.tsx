import * as React from "react";
import { cn } from "@/lib/cn";

/**
 * HENU Kit 工程图纸风格多行文本域基础原语。
 */
const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<"textarea">
>(({ className, ...props }, ref) => {
  return (
    <textarea
      ref={ref}
      className={cn(
        "w-full resize-y border border-ink/30 bg-transparent p-3 text-sm leading-6 text-ink outline-none transition-colors placeholder:text-ink/60 focus:border-ink disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      {...props}
    />
  );
});
Textarea.displayName = "Textarea";

export { Textarea };
