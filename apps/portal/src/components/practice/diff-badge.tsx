import { cn } from "@/lib/cn";

export function difficultyTier(d: number): "easy" | "mid" | "hard" {
  if (d < 4) return "easy";
  if (d < 7) return "mid";
  return "hard";
}

/** 难度分徽标：绿/黄/红实底方块 + 白字数值（全站统一三档） */
export default function DiffBadge({
  value,
  className,
}: {
  value: number;
  className?: string;
}) {
  const tier = difficultyTier(value);
  return (
    <span
      className={cn(
        "inline-flex min-w-10 items-center justify-center px-1.5 py-0.5 font-mono text-[11px] text-paper",
        tier === "easy" && "bg-easy",
        tier === "mid" && "bg-mid",
        tier === "hard" && "bg-hard",
        className
      )}
    >
      {value.toFixed(1)}
    </span>
  );
}
