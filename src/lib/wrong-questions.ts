import type { WeakPointItem } from "@/types";

export type WeakPointSource = {
  course: {
    id: string;
    name: string;
  };
  knowledgePointId?: string | null;
  knowledgePointTitle?: string | null;
};

export function buildWeakPointStats(items: WeakPointSource[]): WeakPointItem[] {
  const stats = new Map<string, WeakPointItem>();

  for (const item of items) {
    const knowledgePointId = item.knowledgePointId ?? undefined;
    const knowledgePointTitle = item.knowledgePointTitle ?? "未关联知识点";
    const key = `${item.course.id}:${knowledgePointId ?? "unknown"}`;
    const current = stats.get(key);

    if (current) {
      current.wrongCount += 1;
      continue;
    }

    stats.set(key, {
      course: item.course,
      knowledgePointId,
      knowledgePointTitle,
      wrongCount: 1,
    });
  }

  return Array.from(stats.values()).sort((left, right) => {
    if (right.wrongCount !== left.wrongCount) {
      return right.wrongCount - left.wrongCount;
    }
    return left.knowledgePointTitle.localeCompare(right.knowledgePointTitle, "zh-CN");
  });
}
