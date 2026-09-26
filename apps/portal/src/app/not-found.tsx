import type { Metadata } from "next";
import Link from "next/link";
import FallbackPage from "@/components/fallback-page";
import { buttonVariants } from "@/components/ui/button";

export const metadata: Metadata = {
  title: "页面不存在",
};

/** 编号与首页导航一致，同一个模块在哪儿都是同一个号。 */
const ENTRIES = [
  { index: "01", label: "资料库", href: "/library" },
  { index: "02", label: "智能刷题", href: "/practice" },
  { index: "03", label: "美食榜", href: "/food" },
];

/** 任何没有对应页面的地址都落在这里，状态码为 404。 */
export default function NotFound() {
  return (
    <FallbackPage
      code="404"
      label="NOT FOUND"
      title="页面不存在"
      description="这个地址没有对应的页面，可能是链接有误，或者页面已经移走。可以回首页，或从下面的常用入口继续。"
    >
      <div className="mt-8">
        <Link href="/" className={buttonVariants()}>
          回首页
        </Link>
      </div>
      <nav aria-labelledby="not-found-entries" className="mt-12 max-w-xl border-t border-line pt-5">
        <p id="not-found-entries" className="font-mono text-xs text-ink/60">
          常用入口
        </p>
        <ul className="mt-2 flex flex-wrap gap-x-8">
          {ENTRIES.map((entry) => (
            <li key={entry.href}>
              <Link
                href={entry.href}
                className="inline-flex min-h-11 items-center font-mono text-sm text-ink/70 transition-colors hover:text-accent-text"
              >
                <span className="mr-1.5 tracking-widest text-accent-text">{entry.index}</span>
                {entry.label}
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </FallbackPage>
  );
}
