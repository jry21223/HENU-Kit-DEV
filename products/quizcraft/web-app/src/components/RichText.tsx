import {
  getLanguageLabel,
  highlightCodeParts,
  parseRichText,
  shouldRenderRichText,
  type RichSegment,
  type HighlightPart,
  type TextSegment,
  type BoldSegment,
  type ItalicSegment,
} from "./codeSyntaxRenderer";

type RichTextProps = {
  text: string;
};

export function RichText({ text }: RichTextProps) {
  if (!shouldRenderRichText(text)) {
    return <>{text}</>;
  }

  let offset = 0;
  const segments = parseRichText(text).map((segment) => {
    const key = `${segment.kind}-${offset}-${segment.value.length}`;
    offset += segment.value.length;
    return <RichTextSegment key={key} segment={segment} />;
  });

  return <span className="qc-rich-text">{segments}</span>;
}

function TextSegmentView({ segment }: { segment: TextSegment }) {
  let offset = 0;
  const lines = segment.value.split(/\r?\n/);

  return (
    <>
      {lines.flatMap((line) => {
        const key = `line-${offset}`;
        offset += line.length + 1;
        if (offset === line.length + 1) return [line];
        return [<br key={`${key}-br`} />, line];
      })}
    </>
  );
}

function CodeParts({ parts }: { parts: HighlightPart[] }) {
  let offset = 0;

  return (
    <>
      {parts.map((part) => {
        const key = `token-${offset}-${part.kind}`;
        offset += part.value.length || 1;
        if (part.kind === "text") {
          return part.value;
        }

        return (
          <span
            key={key}
            className={`qc-code-token qc-code-token--${part.kind}`}
          >
            {part.value}
          </span>
        );
      })}
    </>
  );
}

function RichTextSegment({ segment }: { segment: RichSegment }) {
  if (segment.kind === "text") {
    return <TextSegmentView segment={segment} />;
  }
  if (segment.kind === "bold") {
    return <strong className="font-semibold text-gray-900 dark:text-slate-100">{(segment as BoldSegment).value}</strong>;
  }
  if (segment.kind === "italic") {
    return <em className="italic text-gray-700 dark:text-slate-300">{(segment as ItalicSegment).value}</em>;
  }

  const highlighted = highlightCodeParts(segment.value, segment.language);

  if (!segment.block) {
    return (
      <span className="qc-inline-code">
        <CodeParts parts={highlighted} />
      </span>
    );
  }

  return (
    <span
      className="qc-code-block"
      data-language={getLanguageLabel(segment.language)}
      data-swipe-ignore="true"
    >
      <code>
        <CodeParts parts={highlighted} />
      </code>
    </span>
  );
}
