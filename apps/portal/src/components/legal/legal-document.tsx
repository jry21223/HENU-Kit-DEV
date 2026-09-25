import type { ReactNode } from "react";
import { LEGAL_UPDATED_AT } from "@/lib/site-legal";

export type LegalSection = {
  id: string;
  title: string;
  content: ReactNode;
};

/** 正文排版：协议是要逐字读完的长文，字号、行宽与列表都按阅读而不是装饰来定。 */
const PROSE =
  "space-y-4 text-base leading-8 text-ink/85 [&_a]:underline [&_a]:underline-offset-4 [&_a:hover]:text-accent [&_h3]:mt-8 [&_h3]:text-lg [&_h3]:font-bold [&_h3]:text-ink [&_li+li]:mt-2 [&_ol]:list-decimal [&_ol]:pl-6 [&_strong]:font-semibold [&_strong]:text-ink [&_ul]:list-disc [&_ul]:pl-6";

/**
 * 隐私政策与用户协议共用的文档骨架：标题、更新日期、引言、目录与分节正文。
 * 目录由分节生成，标题与锚点只写一处。
 */
export default function LegalDocument({
  eyebrow,
  title,
  intro,
  sections,
}: {
  eyebrow: string;
  title: string;
  intro: ReactNode;
  sections: LegalSection[];
}) {
  return (
    <main className="mx-auto w-full max-w-3xl px-5 py-12 md:px-8 md:py-16">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/65">{eyebrow}</p>
      <h1 className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">{title}</h1>
      <p className="mt-4 text-sm text-ink/65">最近更新：{LEGAL_UPDATED_AT}</p>

      <div className={`mt-8 ${PROSE}`}>{intro}</div>

      <nav aria-label="目录" className="mt-10 border-y border-line py-5">
        <p className="text-sm font-bold">目录</p>
        <ol className="mt-2 grid text-[15px] sm:grid-cols-2 sm:gap-x-6">
          {sections.map((section) => (
            <li key={section.id}>
              <a
                href={`#${section.id}`}
                className="inline-flex min-h-11 items-center text-ink/80 transition-colors hover:text-accent"
              >
                {section.title}
              </a>
            </li>
          ))}
        </ol>
      </nav>

      {sections.map((section) => (
        <section
          key={section.id}
          id={section.id}
          aria-labelledby={`${section.id}-title`}
          className="mt-12 scroll-mt-20"
        >
          <h2 id={`${section.id}-title`} className="font-display text-2xl font-bold tracking-tight">
            {section.title}
          </h2>
          <div className={`mt-4 ${PROSE}`}>{section.content}</div>
        </section>
      ))}
    </main>
  );
}
