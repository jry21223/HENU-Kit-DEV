import Img from "@/components/ui/img";

/** Only reviewed, content-addressed Practice figures become network requests. */
const FIGURE = /!\[([^\]\n]*)\]\((\/practice-assets\/h3c\/[a-f0-9]{64}\.png)\)/g;
const FIGURE_SOURCE = /^\/practice-assets\/h3c\/[a-f0-9]{64}\.png$/;
export const PRACTICE_FIGURE_ROW_CLASS = "my-4 flex min-w-0 items-start gap-2 whitespace-normal";

export function isPracticeFigureSource(src: unknown): src is string {
  return typeof src === "string" && FIGURE_SOURCE.test(src);
}

export function PracticeFigure({ src, alt, openImage = true, grouped = false }: { src: string; alt?: string; openImage?: boolean; grouped?: boolean }) {
  const label = alt?.trim() || "题图";
  if (!isPracticeFigureSource(src)) return <span>{label}</span>;
  const figure = (
    <Img
      key={src}
      src={src}
      alt={label}
      loading="eager"
      className="block h-auto max-h-[min(28svh,17.5rem)] w-auto max-w-full bg-white object-contain p-2"
    />
  );
  return openImage ? (
    <a
      href={src}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={`打开${label}原图`}
      className={`${grouped ? "block min-w-0 max-w-full shrink" : "my-4 block max-w-full"} rounded-sm focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus-ring`}
    >
      {figure}
    </a>
  ) : <span className={grouped ? "block min-w-0 max-w-full shrink" : "my-4 block max-w-full"}>{figure}</span>;
}

export default function QuestionContent({ content, openImages = true }: { content: string; openImages?: boolean }) {
  const parts: React.ReactNode[] = [];
  const figures = Array.from(content.matchAll(FIGURE));
  let end = 0;

  for (let index = 0; index < figures.length;) {
    const first = figures[index];
    // Remove only row separator lines, retaining text paragraphs and indentation.
    let text = content.slice(end, first.index);
    if (index > 0) text = text.replace(/^(?:[ \t]*\r?\n)+/, "");
    text = text.replace(/\r?\n[ \t\r\n]*$/, "");
    if (text) parts.push(<span key={`text-${end}`}>{text}</span>);

    const group = [first];
    end = first.index + first[0].length;
    index++;
    while (index < figures.length && content.slice(end, figures[index].index).trim() === "") {
      group.push(figures[index]);
      end = figures[index].index + figures[index][0].length;
      index++;
    }
    parts.push(
      <span key={`figures-${first.index}`} className={PRACTICE_FIGURE_ROW_CLASS}>
        {group.map((match) => <PracticeFigure key={`figure-${match[2]}-${match.index}`} src={match[2]} alt={match[1]} openImage={openImages} grouped />)}
      </span>
    );
  }

  if (end < content.length) {
    const remainder = content.slice(end);
    const text = figures.length ? remainder.replace(/^(?:[ \t]*\r?\n)+/, "") : remainder;
    if (text) parts.push(<span key={`text-${end}`}>{text}</span>);
  }

  return <span className="block min-w-0 whitespace-pre-wrap break-words">{parts}</span>;
}
