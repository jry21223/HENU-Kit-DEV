import type { PostBlock } from "@/lib/food/mock";
import Img from "@/components/ui/img";

/** 结构化富文本块渲染（h2/p/quote/list/img，不渲染原始 HTML） */
export default function PostBlocks({
  blocks,
  attachments = [],
}: {
  blocks: PostBlock[];
  /** 编辑器附件（img 块 ref 序号对应的 src），存储后的块自带 src 则不需要 */
  attachments?: string[];
}) {
  return (
    <div className="space-y-4">
      {blocks.map((b, i) => {
        switch (b.type) {
          case "h2":
            return (
              <h2 key={i} className="mt-8 flex items-center gap-3 font-display text-xl font-bold">
                <span aria-hidden className="h-px w-6 bg-accent" />
                {b.text}
              </h2>
            );
          case "quote":
            return (
              <blockquote key={i} className="border-l-2 border-accent py-1 pl-4 text-sm leading-7 text-ink/70">
                {b.text}
              </blockquote>
            );
          case "list":
            return (
              <ul key={i} className="space-y-1.5">
                {b.items?.map((item, j) => (
                  <li key={j} className="text-sm leading-7 text-ink/80">
                    <span className="mr-2 font-mono text-accent">+</span>
                    {item}
                  </li>
                ))}
              </ul>
            );
          case "img": {
            const src = b.src ?? attachments[(b.ref ?? 1) - 1];
            return (
              <Img
                key={i}
                src={src}
                alt="正文插图"
                label={`FIG.${String(i + 1).padStart(2, "0")}`}
                className="max-h-96 w-full"
              />
            );
          }
          default:
            return (
              <p key={i} className="text-sm leading-7 text-ink/80 md:text-base md:leading-8">
                {b.text}
              </p>
            );
        }
      })}
    </div>
  );
}
