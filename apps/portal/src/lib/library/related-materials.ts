import type { Material } from "./mock";

const RELATED_LIMIT = 3;

type Anchor = Pick<Material, "id" | "subject" | "type">;

/** 详情页“相关资料”：同科目优先，不足再用同类型补齐，始终排除资料本身。 */
export function relatedMaterials(catalog: readonly Material[], current: Anchor, limit = RELATED_LIMIT): Material[] {
  const others = catalog.filter((m) => m.id !== current.id);
  const sameSubject = others.filter((m) => m.subject === current.subject);
  const sameType = others.filter((m) => m.subject !== current.subject && m.type === current.type);
  return [...sameSubject, ...sameType].slice(0, limit);
}
