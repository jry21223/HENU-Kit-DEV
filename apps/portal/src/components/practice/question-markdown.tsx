import { memo, type ReactNode } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import java from "highlight.js/lib/languages/java";
import javascript from "highlight.js/lib/languages/javascript";
import typescript from "highlight.js/lib/languages/typescript";
import python from "highlight.js/lib/languages/python";
import cpp from "highlight.js/lib/languages/cpp";
import sql from "highlight.js/lib/languages/sql";
import { cn } from "@/lib/cn";

const remarkPlugins = [remarkGfm];
const rehypePlugins: NonNullable<React.ComponentProps<typeof Markdown>["rehypePlugins"]> = [
  [rehypeHighlight, { languages: { java, javascript, typescript, python, cpp, sql }, detect: false, ignoreMissing: true }],
];

const components: Components = {
  pre: ({ children }) => (
    <pre data-no-question-swipe tabIndex={0} aria-label="代码，可横向滚动" className="question-code">
      {children}
    </pre>
  ),
  table: ({ children }) => (
    <div data-no-question-swipe tabIndex={0} role="region" aria-label="表格，可横向滚动" className="max-w-full overflow-x-auto">
      <table>{children}</table>
    </div>
  ),
};

// Options live inside native answer buttons: use only phrasing content and no
// nested links, focusable scrollers, inputs, images or other interactive nodes.
function OptionBlock({ children }: { children?: ReactNode }) {
  return <span className="block">{children}</span>;
}
const optionComponents: Components = {
  p: OptionBlock,
  pre: ({ children }) => <span data-no-question-swipe className="question-code block">{children}</span>,
  a: ({ children }) => <span>{children}</span>,
  img: ({ alt }) => <span>{alt}</span>,
  table: ({ children }) => <span data-no-question-swipe className="block max-w-full overflow-x-auto"><span role="table" className="table text-sm">{children}</span></span>,
  thead: ({ children }) => <span role="rowgroup" className="table-header-group">{children}</span>,
  tbody: ({ children }) => <span role="rowgroup" className="table-row-group">{children}</span>,
  tr: ({ children }) => <span role="row" className="table-row">{children}</span>,
  th: ({ children }) => <span role="columnheader" className="table-cell border border-line p-2">{children}</span>,
  td: ({ children }) => <span role="cell" className="table-cell border border-line p-2">{children}</span>,
  ul: ({ children }) => <span role="list" className="block list-disc pl-6">{children}</span>,
  ol: ({ children }) => <span role="list" className="block list-decimal pl-6">{children}</span>,
  li: ({ children }) => <span role="listitem" className="list-item">{children}</span>,
};

const optionElements = ["p", "pre", "code", "span", "strong", "em", "del", "br", "a", "img", "table", "thead", "tbody", "tr", "th", "td", "ul", "ol", "li"];

function QuestionMarkdown({ content, className, option = false }: {
  content: string;
  className?: string;
  option?: boolean;
}) {
  const rendered = (
    <Markdown
      remarkPlugins={remarkPlugins}
      rehypePlugins={rehypePlugins}
      skipHtml
      allowedElements={option ? optionElements : undefined}
      unwrapDisallowed={option}
      components={option ? optionComponents : components}
    >
      {content}
    </Markdown>
  );
  const classes = cn("question-markdown min-w-0 max-w-full", className);
  return option ? <span className={cn(classes, "block")}>{rendered}</span> : <div className={classes}>{rendered}</div>;
}

export default memo(QuestionMarkdown);
