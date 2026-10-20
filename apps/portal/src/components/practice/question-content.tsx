import Img from "@/components/ui/img";

/** Only reviewed, content-addressed Practice figures become network requests. */
const FIGURE = /!\[([^\]\n]*)\]\((\/practice-assets\/h3c\/[a-f0-9]{64}\.png)\)/g;

export default function QuestionContent({ content, openImages = true }: { content: string; openImages?: boolean }) {
  const parts: React.ReactNode[] = [];
  let end = 0;

  for (const match of content.matchAll(FIGURE)) {
    if (match.index > end) {
      parts.push(<span key={`text-${end}`}>{content.slice(end, match.index)}</span>);
    }
    const label = match[1].trim() || "题图";
    const src = match[2];
    const figure = (
      <Img
        src={src}
        alt={label}
        loading="eager"
        className="block h-auto max-h-[70vh] w-full bg-white object-contain p-2"
      />
    );
    parts.push(openImages ? (
      <a
        key={`figure-${src}-${match.index}`}
        href={src}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={`打开${label}原图`}
        className="my-4 block max-w-full rounded-sm focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus-ring"
      >
        {figure}
      </a>
    ) : <span key={`figure-${src}-${match.index}`} className="my-4 block max-w-full">{figure}</span>);
    end = match.index + match[0].length;
  }

  if (end < content.length) {
    parts.push(<span key={`text-${end}`}>{content.slice(end)}</span>);
  }

  return <span className="block min-w-0 whitespace-pre-wrap break-words">{parts}</span>;
}
