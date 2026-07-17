export type CodeLanguage = "java" | "html" | "css" | "javascript" | "json" | "text";
export type TextSegment = { kind: "text"; value: string };
export type CodeSegment = { kind: "code"; value: string; language: CodeLanguage; block: boolean };
export type BoldSegment = { kind: "bold"; value: string };
export type ItalicSegment = { kind: "italic"; value: string };
export type RichSegment = TextSegment | CodeSegment | BoldSegment | ItalicSegment;
type TokenKind = "text" | "comment" | "string";
export type HighlightPartKind =
  | TokenKind
  | "attr"
  | "keyword"
  | "number"
  | "tag";
export type HighlightPart = {
  kind: HighlightPartKind;
  value: string;
};

type Token = {
  kind: TokenKind;
  value: string;
};

const LANGUAGE_ALIASES: Record<string, CodeLanguage> = {
  htm: "html",
  html: "html",
  markup: "html",
  xml: "html",
  web: "html",
  css: "css",
  java: "java",
  js: "javascript",
  javascript: "javascript",
  jsx: "javascript",
  ts: "javascript",
  tsx: "javascript",
  typescript: "javascript",
  json: "json",
  text: "text",
  plaintext: "text",
  plain: "text",
};

const KEYWORDS: Record<CodeLanguage, string[]> = {
  java: [
    "abstract",
    "assert",
    "boolean",
    "break",
    "byte",
    "case",
    "catch",
    "char",
    "class",
    "const",
    "continue",
    "default",
    "do",
    "double",
    "else",
    "enum",
    "extends",
    "final",
    "finally",
    "float",
    "for",
    "if",
    "implements",
    "import",
    "instanceof",
    "int",
    "interface",
    "long",
    "new",
    "package",
    "private",
    "protected",
    "public",
    "return",
    "short",
    "static",
    "strictfp",
    "super",
    "switch",
    "synchronized",
    "this",
    "throw",
    "throws",
    "transient",
    "try",
    "void",
    "volatile",
    "while",
    "true",
    "false",
    "null",
  ],
  javascript: [
    "async",
    "await",
    "break",
    "case",
    "catch",
    "class",
    "const",
    "continue",
    "debugger",
    "default",
    "delete",
    "do",
    "else",
    "export",
    "extends",
    "false",
    "finally",
    "for",
    "from",
    "function",
    "if",
    "import",
    "in",
    "instanceof",
    "let",
    "new",
    "null",
    "return",
    "super",
    "switch",
    "this",
    "throw",
    "true",
    "try",
    "typeof",
    "undefined",
    "var",
    "void",
    "while",
    "yield",
  ],
  css: [
    "align-items",
    "background",
    "border",
    "box-sizing",
    "color",
    "display",
    "flex",
    "font-size",
    "font-weight",
    "gap",
    "grid",
    "height",
    "justify-content",
    "line-height",
    "margin",
    "padding",
    "position",
    "text-align",
    "width",
  ],
  html: [],
  json: ["true", "false", "null"],
  text: [],
};

const CODE_FENCE_PATTERN = /```[ \t]*([A-Za-z0-9_-]+)?[ \t]*\r?\n?([\s\S]*?)```/g;
const INLINE_CODE_PATTERN = /`([^`\n]+)`/g;
const BARE_HTML_TAG_PATTERN = /<\/?([A-Za-z][\w:-]*)(?:\s+[^<>\n]*?)?\s*\/?>/g;
const HTML_TAG_NAMES = new Set([
  "a",
  "audio",
  "body",
  "br",
  "button",
  "dd",
  "div",
  "dl",
  "dt",
  "form",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "head",
  "heading",
  "html",
  "image",
  "img",
  "import",
  "input",
  "label",
  "lb",
  "li",
  "link",
  "ol",
  "optgroup",
  "p",
  "script",
  "select",
  "span",
  "style",
  "table",
  "td",
  "textarea",
  "text",
  "thead",
  "title",
  "tl",
  "tr",
  "ul",
  "video",
]);

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function testInlineCode(text: string) {
  INLINE_CODE_PATTERN.lastIndex = 0;
  return INLINE_CODE_PATTERN.test(text);
}

function testBareHtmlTag(text: string) {
  BARE_HTML_TAG_PATTERN.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = BARE_HTML_TAG_PATTERN.exec(text))) {
    if (HTML_TAG_NAMES.has(match[1].toLowerCase())) return true;
  }
  return false;
}

function normalizeLanguage(language: string | undefined, code = ""): CodeLanguage {
  const normalized = language?.trim().toLowerCase();

  if (normalized && LANGUAGE_ALIASES[normalized]) {
    return LANGUAGE_ALIASES[normalized];
  }

  if (/\b(public|private|protected|class|static|void|String|int|boolean|System\.out)\b/.test(code)) {
    return "java";
  }

  if (/<\/?[A-Za-z][\w:-]*(\s|>|\/)/.test(code)) {
    return "html";
  }

  if (/\b(const|let|var|function|document|console)\b|=>/.test(code)) {
    return "javascript";
  }

  if (/[.#][\w-]+\s*\{|\b(color|display|margin|padding|font-size)\s*:/.test(code)) {
    return "css";
  }

  if (/^\s*(?:\[|\{)/.test(code)) {
    return "json";
  }

  return "text";
}

function looksLikeCodeLine(line: string) {
  const trimmed = line.trim();
  if (!trimmed) return false;

  return (
    /\b(public|private|protected|class|static|void|String|int|boolean|System\.out)\b/.test(trimmed) ||
    /<\/?[A-Za-z][\w:-]*(\s|>|\/)/.test(trimmed) ||
    /^\s*(const|let|var|function|if|for|while|return)\b/.test(trimmed) ||
    /=>|[{};]|[.#][\w-]+\s*\{/.test(trimmed) ||
    /\b(color|display|margin|padding|font-size|background)\s*:/.test(trimmed)
  );
}

function splitLooseCode(text: string): RichSegment[] | null {
  const lines = text.split(/\r?\n/);
  if (lines.length < 2) return null;

  const firstCodeLine = lines.findIndex(looksLikeCodeLine);
  if (firstCodeLine < 0) return null;

  const intro = lines.slice(0, firstCodeLine).join("\n").trimEnd();
  const code = lines.slice(firstCodeLine).join("\n").trimEnd();

  if (!code) return null;

  return [
    ...(intro ? [{ kind: "text" as const, value: intro }] : []),
    { kind: "code" as const, value: code, language: normalizeLanguage(undefined, code), block: true },
  ];
}

export function parseRichText(text: string): RichSegment[] {
  CODE_FENCE_PATTERN.lastIndex = 0;

  const segments: RichSegment[] = [];
  let cursor = 0;
  let match: RegExpExecArray | null;

  while ((match = CODE_FENCE_PATTERN.exec(text))) {
    if (match.index > cursor) {
      segments.push({ kind: "text", value: text.slice(cursor, match.index) });
    }

    const code = match[2].replace(/^\r?\n/, "").replace(/\r?\n$/, "");
    segments.push({
      kind: "code",
      value: code,
      language: normalizeLanguage(match[1], code),
      block: true,
    });
    cursor = match.index + match[0].length;
  }

  if (segments.length > 0) {
    if (cursor < text.length) {
      segments.push({ kind: "text", value: text.slice(cursor) });
    }
    return parseInlineMarkdown(segments);
  }

  const looseCode = splitLooseCode(text);
  if (looseCode) return parseInlineMarkdown(looseCode);

  INLINE_CODE_PATTERN.lastIndex = 0;
  cursor = 0;
  while ((match = INLINE_CODE_PATTERN.exec(text))) {
    if (match.index > cursor) {
      segments.push({ kind: "text", value: text.slice(cursor, match.index) });
    }
    segments.push({
      kind: "code",
      value: match[1],
      language: normalizeLanguage(undefined, match[1]),
      block: false,
    });
    cursor = match.index + match[0].length;
  }

  if (segments.length > 0) {
    if (cursor < text.length) {
      segments.push({ kind: "text", value: text.slice(cursor) });
    }
    return parseInlineMarkdown(segments);
  }

  BARE_HTML_TAG_PATTERN.lastIndex = 0;
  cursor = 0;
  while ((match = BARE_HTML_TAG_PATTERN.exec(text))) {
    if (!HTML_TAG_NAMES.has(match[1].toLowerCase())) continue;

    if (match.index > cursor) {
      segments.push({ kind: "text", value: text.slice(cursor, match.index) });
    }
    segments.push({
      kind: "code",
      value: match[0],
      language: "html",
      block: false,
    });
    cursor = match.index + match[0].length;
  }

  if (segments.length > 0) {
    if (cursor < text.length) {
      segments.push({ kind: "text", value: text.slice(cursor) });
    }
    return parseInlineMarkdown(segments);
  }

  return parseInlineMarkdown([{ kind: "text", value: text }]);
}

function tokenize(code: string): Token[] {
  const patterns: Array<{ kind: Exclude<TokenKind, "text">; pattern: RegExp }> = [
    { kind: "comment", pattern: /\/\*[\s\S]*?\*\/|\/\/[^\n\r]*/y },
    { kind: "comment", pattern: /<!--[\s\S]*?-->/y },
    { kind: "string", pattern: /"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`/y },
  ];

  const tokens: Token[] = [];
  let cursor = 0;
  let textBuffer = "";

  const flushText = () => {
    if (textBuffer) {
      tokens.push({ kind: "text", value: textBuffer });
      textBuffer = "";
    }
  };

  while (cursor < code.length) {
    let matched = false;

    for (const { kind, pattern } of patterns) {
      pattern.lastIndex = cursor;
      const match = pattern.exec(code);
      if (!match) continue;

      flushText();
      tokens.push({ kind, value: match[0] });
      cursor += match[0].length;
      matched = true;
      break;
    }

    if (!matched) {
      textBuffer += code[cursor];
      cursor += 1;
    }
  }

  flushText();
  return tokens;
}

function splitTextParts(
  parts: HighlightPart[],
  pattern: RegExp,
  kind: Exclude<HighlightPartKind, "text" | "comment" | "string">,
) {
  const next: HighlightPart[] = [];

  for (const part of parts) {
    if (part.kind !== "text") {
      next.push(part);
      continue;
    }

    let cursor = 0;
    let match: RegExpExecArray | null;
    pattern.lastIndex = 0;

    while ((match = pattern.exec(part.value))) {
      if (match.index > cursor) {
        next.push({ kind: "text", value: part.value.slice(cursor, match.index) });
      }
      next.push({ kind, value: match[1] });
      cursor = match.index + match[0].length;

      if (match[0].length === 0) {
        pattern.lastIndex += 1;
      }
    }

    if (cursor < part.value.length) {
      next.push({ kind: "text", value: part.value.slice(cursor) });
    }
  }

  return next;
}

function highlightHtmlTextParts(value: string): HighlightPart[] {
  const parts: HighlightPart[] = [];
  const tagPattern = /(<\/?)([A-Za-z][\w:-]*)([^>]*>)/g;
  const attrPattern = /([A-Za-z_:][-A-Za-z0-9_:.]*)(=)/g;
  let cursor = 0;
  let tagMatch: RegExpExecArray | null;

  while ((tagMatch = tagPattern.exec(value))) {
    if (tagMatch.index > cursor) {
      parts.push({ kind: "text", value: value.slice(cursor, tagMatch.index) });
    }

    parts.push({ kind: "text", value: tagMatch[1] });
    parts.push({ kind: "tag", value: tagMatch[2] });

    const rest = tagMatch[3];
    let restCursor = 0;
    let attrMatch: RegExpExecArray | null;
    attrPattern.lastIndex = 0;

    while ((attrMatch = attrPattern.exec(rest))) {
      if (attrMatch.index > restCursor) {
        parts.push({ kind: "text", value: rest.slice(restCursor, attrMatch.index) });
      }
      parts.push({ kind: "attr", value: attrMatch[1] });
      parts.push({ kind: "text", value: attrMatch[2] });
      restCursor = attrMatch.index + attrMatch[0].length;
    }

    if (restCursor < rest.length) {
      parts.push({ kind: "text", value: rest.slice(restCursor) });
    }

    cursor = tagMatch.index + tagMatch[0].length;
  }

  if (cursor < value.length) {
    parts.push({ kind: "text", value: value.slice(cursor) });
  }

  return parts;
}

function highlightTextParts(value: string, language: CodeLanguage): HighlightPart[] {
  let parts =
    language === "html"
      ? highlightHtmlTextParts(value)
      : [{ kind: "text" as const, value }];

  const keywords = KEYWORDS[language];
  if (keywords.length > 0) {
    parts = splitTextParts(
      parts,
      new RegExp(`\\b(${keywords.map(escapeRegExp).join("|")})\\b`, "g"),
      "keyword",
    );
  }

  return splitTextParts(parts, /\b(\d+(?:\.\d+)?)\b/g, "number");
}

export function highlightCodeParts(code: string, language: CodeLanguage) {
  return tokenize(code).flatMap((token): HighlightPart[] => {
    if (token.kind === "text") {
      return highlightTextParts(token.value, language);
    }

    return [{ kind: token.kind, value: token.value }];
  });
}

export function getLanguageLabel(language: CodeLanguage) {
  return language === "javascript" ? "JS" : language.toUpperCase();
}

// ── Parse inline code within a text string ──

function parseInlineCodeInText(text: string): RichSegment[] {
  INLINE_CODE_PATTERN.lastIndex = 0;
  const result: RichSegment[] = [];
  let cursor = 0;
  let match: RegExpExecArray | null;
  while ((match = INLINE_CODE_PATTERN.exec(text))) {
    if (match.index > cursor) {
      result.push({ kind: "text", value: text.slice(cursor, match.index) });
    }
    result.push({ kind: "code", value: match[1], language: normalizeLanguage(undefined, match[1]), block: false });
    cursor = match.index + match[0].length;
  }
  if (cursor < text.length) {
    result.push({ kind: "text", value: text.slice(cursor) });
  }
  return result.length > 0 ? result : [{ kind: "text", value: text }];
}

// ── Inline Markdown: bold / italic ──

const BOLD_PATTERN = /\*\*(.+?)\*\*|__(.+?)__/g;
const ITALIC_PATTERN = /\*(.+?)\*/g;

function parseInlineMarkdown(segments: RichSegment[]): RichSegment[] {
  // First pass: parse inline code within text segments
  const withCode: RichSegment[] = [];
  for (const seg of segments) {
    if (seg.kind !== "text") {
      withCode.push(seg);
      continue;
    }
    for (const sub of parseInlineCodeInText(seg.value)) {
      withCode.push(sub);
    }
  }

  // Second pass: parse bold/italic within text segments
  const result: RichSegment[] = [];
  for (const seg of withCode) {
    if (seg.kind !== "text") {
      result.push(seg);
      continue;
    }
    let parts: Array<{ kind: "text" | "bold" | "italic"; value: string }> = [
      { kind: "text", value: seg.value },
    ];

    // Parse bold first
    const boldParts: typeof parts = [];
    for (const p of parts) {
      if (p.kind !== "text") { boldParts.push(p); continue; }
      BOLD_PATTERN.lastIndex = 0;
      let cursor = 0;
      let match: RegExpExecArray | null;
      while ((match = BOLD_PATTERN.exec(p.value))) {
        if (match.index > cursor) {
          boldParts.push({ kind: "text", value: p.value.slice(cursor, match.index) });
        }
        boldParts.push({ kind: "bold", value: match[1] || match[2] });
        cursor = match.index + match[0].length;
      }
      if (cursor < p.value.length) {
        boldParts.push({ kind: "text", value: p.value.slice(cursor) });
      }
    }
    parts = boldParts.length > 0 ? boldParts : parts;

    // Parse italic (skip if already bold)
    const italicParts: typeof parts = [];
    for (const p of parts) {
      if (p.kind !== "text") { italicParts.push(p); continue; }
      ITALIC_PATTERN.lastIndex = 0;
      let cursor = 0;
      let match: RegExpExecArray | null;
      while ((match = ITALIC_PATTERN.exec(p.value))) {
        if (match.index > cursor) {
          italicParts.push({ kind: "text", value: p.value.slice(cursor, match.index) });
        }
        italicParts.push({ kind: "italic", value: match[1] });
        cursor = match.index + match[0].length;
      }
      if (cursor < p.value.length) {
        italicParts.push({ kind: "text", value: p.value.slice(cursor) });
      }
    }

    for (const p of (italicParts.length > 0 ? italicParts : parts)) {
      result.push(p as RichSegment);
    }
  }
  return result;
}

export function shouldRenderRichText(text: string) {
  return (
    /[\r\n]/.test(text) ||
    text.includes("```") ||
    testInlineCode(text) ||
    testBareHtmlTag(text) ||
    splitLooseCode(text) !== null ||
    /\*\*/.test(text) ||
    /\*[^*]+\*/.test(text) ||
    /__/.test(text)
  );
}
