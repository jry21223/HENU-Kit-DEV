import { cn } from "@/lib/cn";

export default function SectionHeading({
  index,
  en,
  title,
  dark = false,
  className,
}: {
  index: string;
  en: string;
  title: string;
  dark?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-4", className)}>
      <p
        className={cn(
          "font-mono text-xs tracking-[0.3em] uppercase",
          dark ? "text-paper/60" : "text-ink/60"
        )}
      >
        <span className="text-accent">{index}</span>
        <span className="mx-2">/</span>
        {en}
      </p>
      <h2
        className={cn(
          "font-display text-4xl md:text-6xl font-bold tracking-tight",
          dark ? "text-paper" : "text-ink"
        )}
      >
        {title}
      </h2>
    </div>
  );
}
