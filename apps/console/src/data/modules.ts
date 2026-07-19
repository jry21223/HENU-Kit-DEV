export type ModuleId = "portal" | "platform" | "notice" | "library" | "quizcraft" | "food";
export type ModuleStatus = "ok" | "loading" | "empty" | "partial" | "stale" | "unavailable" | "denied";

export interface ModuleMetric {
  label: string;
  value: string;
  hint?: string;
}

export interface TrendPoint {
  label: string;
  value: number;
}

export interface ModuleSummary {
  id: ModuleId;
  name: string;
  eyebrow: string;
  status: ModuleStatus;
  description: string;
  metrics: ModuleMetric[];
  statusMessage: string;
  asOf?: string;
  lastSuccessAt?: string;
  requestId?: string;
  trend?: TrendPoint[];
}

const presentation = [
  ["portal", "Portal", "Public experience", "公开主站版本、入口与关键页面探针，只读呈现。"],
  ["platform", "Platform Operations", "Shared platform", "账户、权限、Session、邮件、审计与 Operations Inbox。"],
  ["notice", "Notice", "Campus notices", "通知来源、版本、审核、受众与分发状态。"],
  ["library", "Library", "Learning materials", "课程、资料、下载、投稿、审核与纠错摘要。"],
  ["quizcraft", "QuizCraft", "Practice product", "题库、反馈与服务状态摘要；题库工坊仍在产品自有后台。"],
  ["food", "Food", "Campus food", "榜单投稿、异常票与调档确认。"],
] as const;

export const moduleSummaries: ModuleSummary[] = presentation.map(([id, name, eyebrow, description]) => ({
  id,
  name,
  eyebrow,
  description,
  status: "loading",
  metrics: [],
  statusMessage: "等待 Gateway 摘要",
}));
