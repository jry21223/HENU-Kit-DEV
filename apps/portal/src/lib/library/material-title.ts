import { MATERIAL_TYPES, type MaterialType } from "./material-types";

const TYPE_ALIASES: Record<MaterialType, readonly string[]> = {
  handout: ["讲义"],
  exam: ["真题"],
  slides: [],
  exercise: [],
  answer: ["答案"],
  note: ["笔记"],
  textbook: ["教材"],
};

const IDENTIFIER_CHAR = /[A-Za-z0-9]/;

/**
 * Source titles separate segments with underscores (`2020级_第1章_扫描版`). Show
 * those separators as middle dots, but keep an underscore between two ASCII
 * letters or digits: it belongs to an identifier such as `primary_key`.
 */
function separateSegments(text: string): string {
  return text
    .replace(/\s*_+\s*/g, (separator: string, offset: number, whole: string) => {
      const before = whole[offset - 1];
      const after = whole[offset + separator.length];
      if (!before || !after) return "";
      return IDENTIFIER_CHAR.test(before) && IDENTIFIER_CHAR.test(after) ? separator : " · ";
    })
    .trim();
}

/**
 * Display title only: remove redundant, exact catalog prefixes and show segment
 * separators as middle dots. The source title itself stays unchanged.
 */
export function readableMaterialTitle(material: {
  title: string;
  subject: string;
  type: MaterialType;
}): string {
  const { title, subject, type } = material;
  const fallback = separateSegments(title) || title;
  if (!subject || !title.startsWith(`${subject}_`)) return fallback;

  let readable = title.slice(subject.length + 1);
  const typePrefix = [MATERIAL_TYPES[type].name, ...TYPE_ALIASES[type]]
    .find((alias) => readable.startsWith(`${alias}_`));
  if (!typePrefix) return fallback;
  readable = separateSegments(readable.slice(typePrefix.length + 1));

  return readable || fallback;
}
