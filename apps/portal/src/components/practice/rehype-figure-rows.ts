import type { Element, Root, RootContent } from "hast";
import { isPracticeFigureSource, PRACTICE_FIGURE_ROW_CLASS } from "./question-content";

function isWhitespace(node: RootContent, source: string) {
  if (node.type === "text") return node.value.trim() === "";
  // CommonMark turns two trailing spaces into a hard break. Only that
  // whitespace separator may join a row; an explicit backslash break survives.
  if (node.type !== "element" || node.tagName !== "br") return false;
  const start = node.position?.start.offset;
  const end = node.position?.end.offset;
  return start !== undefined && end !== undefined && end > start && source.slice(start, end).trim() === "";
}

/** Only image-only paragraphs or inline reviewed images can join a row. */
function figuresIn(node: RootContent, source: string): Element[] | null {
  if (node.type !== "element") return null;
  if (node.tagName === "img" && isPracticeFigureSource(node.properties.src)) return [node];
  if (node.tagName !== "p") return null;
  const figures: Element[] = [];
  for (const child of node.children) {
    if (isWhitespace(child, source)) continue;
    if (child.type !== "element" || child.tagName !== "img" || !isPracticeFigureSource(child.properties.src)) return null;
    figures.push(child);
  }
  return figures.length ? figures : null;
}

function groupFigures(parent: Root | Element, source: string) {
  for (let index = 0; index < parent.children.length; index++) {
    const child = parent.children[index];
    const figures = figuresIn(child, source);
    if (!figures) {
      if (child.type === "element") groupFigures(child, source);
      continue;
    }
    let end = index + 1;
    for (let next = end; next < parent.children.length; next++) {
      const candidate = parent.children[next];
      if (isWhitespace(candidate, source)) continue;
      const adjacent = figuresIn(candidate, source);
      if (!adjacent) break;
      figures.push(...adjacent);
      end = next + 1;
    }
    for (const figure of figures) figure.properties["data-practice-grouped"] = true;
    const row: Element = {
      type: "element",
      tagName: "span",
      properties: { className: PRACTICE_FIGURE_ROW_CLASS.split(" ") },
      children: figures,
    };
    // Run before descending, so image-only paragraphs can join across blank
    // Markdown lines. Real text, fenced code and paragraph boundaries survive.
    parent.children.splice(index, end - index, row);
    const before = parent.children[index - 1];
    if (before?.type === "text") before.value = before.value.replace(/\r?\n[ \t\r\n]*$/, "");
    const after = parent.children[index + 1];
    if (after?.type === "text") after.value = after.value.replace(/^(?:[ \t]*\r?\n)+/, "");
  }
}

function preserveParagraphIndentation(tree: Root, source: string) {
  // CommonMark drops 1–3 leading spaces in plain paragraphs. Restore only
  // whitespace from exact source positions, never infer or rewrite code. Root
  // paragraphs exclude structural indentation belonging to lists/blockquotes.
  for (const paragraph of tree.children) {
    if (paragraph.type !== "element" || paragraph.tagName !== "p") continue;
    for (const text of paragraph.children) {
      if (text.type !== "text") continue;
      const start = text.position?.start.offset;
      const end = text.position?.end.offset;
      if (start === undefined || end === undefined) continue;
      const original = source.slice(start, end).split(/\r?\n/);
      const rendered = text.value.split("\n");
      if (original.length !== rendered.length) continue;
      const prefix = source.slice(source.lastIndexOf("\n", start - 1) + 1, start);
      text.value = rendered.map((line, index) => {
        if (index === 0) return /^[ \t]*$/.test(prefix) ? prefix + line : line;
        return (original[index].match(/^[ \t]*/)?.[0] ?? "") + line.replace(/^[ \t]*/, "");
      }).join("\n");
    }
  }
}

export default function rehypeFigureRows() {
  return (tree: Root, file: { value: unknown }) => {
    const source = typeof file.value === "string" ? file.value : "";
    preserveParagraphIndentation(tree, source);
    groupFigures(tree, source);
  };
}
