import Link from "next/link";

/**
 * 互助平台还没开放的二级页（我的交易、发布）：直接访问时给出「即将开放」说明，
 * 而不是要求登录后再看到一个用不了的表单或空壳。入口已从二级导航撤下，这里只接住
 * 旧链接和书签，并把人带回仍可用的浏览页（#568）。
 */
export default function CampusComingSoon({
  index,
  en,
  title,
  description,
}: {
  index: string;
  en: string;
  title: string;
  description: string;
}) {
  return (
    <main className="mx-auto max-w-site px-5 py-10 md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent-text">{index}</span>
        <span className="mx-2">/</span>
        {en}
      </p>
      <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">{title}</h1>

      <div className="mt-6 border border-dashed border-ink/30 px-6 py-16 text-center">
        <p className="font-mono text-xs text-ink/60">即将开放</p>
        <p className="mx-auto mt-3 max-w-sm text-sm leading-7 text-ink/70">{description}</p>
        <Link
          href="/campus"
          className="mt-6 inline-flex min-h-11 items-center border border-ink px-6 font-mono text-xs text-ink transition-colors hover:border-accent hover:text-accent-text"
        >
          去浏览互助信息
        </Link>
      </div>
    </main>
  );
}
