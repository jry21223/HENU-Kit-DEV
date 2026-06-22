import type { Material } from "@/lib/api";

export const materialTypeLabel: Record<string, string> = {
  knowledge_note: "讲义",
  mock_paper: "模拟卷",
  answer: "答案解析",
  quick_review: "考前速背",
  past_exam: "历年真题",
  other: "资料",
};

export const accessLevelLabel: Record<string, string> = {
  free: "免费",
  login_required: "登录可下载",
  paid: "付费资料",
  member_only: "会员资料",
};

export function formatFileSize(bytes?: number) {
  if (!bytes || bytes <= 0) return "大小未知";
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function labelMaterialType(type: string) {
  return materialTypeLabel[type] ?? type;
}

export function labelAccessLevel(level: string) {
  return accessLevelLabel[level] ?? level;
}

export function summarizeMaterialTypes(materials: Material[]) {
  const counts = new Map<string, number>();
  for (const material of materials) {
    const label = labelMaterialType(material.type);
    counts.set(label, (counts.get(label) ?? 0) + 1);
  }
  return Array.from(counts.entries()).map(([label, count]) => ({ label, count }));
}

export function latestUpdatedAt(materials: Material[]) {
  const dates = materials
    .map((material) => material.updatedAt)
    .filter((value): value is string => Boolean(value))
    .map((value) => new Date(value))
    .filter((value) => !Number.isNaN(value.getTime()));
  if (!dates.length) return "持续维护";
  const latest = new Date(Math.max(...dates.map((date) => date.getTime())));
  return latest.toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" });
}
