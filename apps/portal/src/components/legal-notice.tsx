import Link from "next/link";
import { cn } from "@/lib/cn";
import {
  ICP_FILING,
  ICP_FILING_URL,
  LEGAL_LINKS,
  SITE_DISCLAIMER_SHORT,
} from "@/lib/site-legal";

const LINK_CLASS =
  "inline-flex min-h-11 items-center underline-offset-4 transition-colors hover:text-ink hover:underline";

/**
 * 主体声明、协议入口与备案号这一行。首页大页脚与子站页脚共用它，
 * 所以任何页面看到的都是同一份声明和同一组链接。
 */
export default function LegalNotice({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "flex flex-col gap-x-8 text-[13px] leading-6 text-ink/70 md:flex-row md:flex-wrap md:items-center md:justify-between",
        className
      )}
    >
      <p className="py-2 md:py-0">{SITE_DISCLAIMER_SHORT}</p>
      <nav aria-label="法律信息" className="flex flex-wrap items-center gap-x-6">
        {LEGAL_LINKS.map((link) => (
          <Link key={link.href} href={link.href} className={LINK_CLASS}>
            {link.label}
          </Link>
        ))}
        {ICP_FILING ? (
          <a href={ICP_FILING_URL} target="_blank" rel="noreferrer" className={LINK_CLASS}>
            {ICP_FILING}
          </a>
        ) : null}
      </nav>
    </div>
  );
}
