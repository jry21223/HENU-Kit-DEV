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

/** Only remove redundant, exact catalog prefixes; keep the source title intact. */
export function readableMaterialTitle(material: {
  title: string;
  subject: string;
  type: MaterialType;
}): string {
  const { title, subject, type } = material;
  if (!subject || !title.startsWith(`${subject}_`)) return title;

  let readable = title.slice(subject.length + 1);
  const typePrefix = [MATERIAL_TYPES[type].name, ...TYPE_ALIASES[type]]
    .find((alias) => readable.startsWith(`${alias}_`));
  if (!typePrefix) return title;
  readable = readable.slice(typePrefix.length + 1);

  // Remaining underscores may belong to identifiers or edition information.
  return readable.trim() ? readable : title;
}
