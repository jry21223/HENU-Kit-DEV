import Link from "next/link";
import { Fragment, type HTMLAttributes } from "react";
import { cn } from "@/lib/cn";
import { LEGAL_LINKS, SITE_OPERATOR_STATEMENT } from "@/lib/site-legal";

/**
 * 登录、注册与支付前的主体声明与同意告知（DESIGN_SYSTEM §16）。协议链接取自 LEGAL_LINKS，
 * 并在新标签页打开，读者填了一半的表单不会丢。
 */
export default function LegalConsent({
  action,
  className,
  ...rest
}: { action: string } & HTMLAttributes<HTMLParagraphElement>) {
  return (
    <p className={cn("text-xs leading-5 text-ink/65", className)} {...rest}>
      {SITE_OPERATOR_STATEMENT}
      {action}即表示你已阅读并同意
      {LEGAL_LINKS.map((link, index) => (
        <Fragment key={link.href}>
          {index > 0 ? "和" : null}
          <Link
            href={link.href}
            target="_blank"
            rel="noopener"
            className="text-ink underline underline-offset-4 hover:text-accent-text"
          >
            《{link.label}》
          </Link>
        </Fragment>
      ))}
      。
    </p>
  );
}
