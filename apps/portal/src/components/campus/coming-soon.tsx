import Link from "next/link";

/**
 * 互助平台的旧二级入口页（发布单子、我的交易）：承接旧链接与书签，
 * 清晰说明对应操作已在互助平台广场与单子详情页中提供，并引导前往 /campus。
 */
export default function CampusComingSoon({
  index,
  en,
  title,
  badge = "入口说明",
  description,
  note,
  ctaLabel = "前往互助平台",
  ctaHref = "/campus",
}: {
  index: string;
  en: string;
  title: string;
  badge?: string;
  description: string;
  note?: string;
  ctaLabel?: string;
  ctaHref?: string;
}) {
  return (
    <main className="mx-auto max-w-site px-5 py-10 md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent-text">{index}</span>
        <span className="mx-2">/</span>
        {en}
      </p>
      <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">{title}</h1>

      <div className="mt-6 border border-ink/25 bg-paper-raised px-6 py-12 text-center sm:px-10">
        <p className="font-mono text-xs text-ink/60">{badge}</p>
        <p className="mx-auto mt-3 max-w-lg text-sm leading-7 text-ink/80">{description}</p>
        {note ? (
          <p className="mx-auto mt-3 max-w-lg text-xs leading-6 text-ink/60">{note}</p>
        ) : null}
        <Link
          href={ctaHref}
          className="mt-6 inline-flex min-h-11 items-center border border-ink bg-ink px-6 font-mono text-xs text-paper transition-colors hover:border-accent hover:bg-accent hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
        >
          {ctaLabel}
        </Link>
      </div>
    </main>
  );
}
