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
  unavailableReason?: "not_onboarded" | "operator_disabled";
  asOf?: string;
  lastSuccessAt?: string;
  requestId?: string;
  trend?: TrendPoint[];
}

const presentation = [
  ["portal", "Portal", "公共体验", "公开主站版本、入口与关键页面监测，只读呈现。"],
  ["platform", "Platform Operations", "共享平台", "账户、权限、登录会话、邮件、审计与运营收件箱。"],
  ["notice", "Notice", "校园通知", "通知来源、版本、审核、受众与分发状态。"],
  ["library", "Library", "学习资料", "课程、资料、下载、投稿、审核与纠错摘要。"],
  ["quizcraft", "QuizCraft", "刷题产品", "题库、反馈与服务状态摘要；题库工坊独立运营，后续接入。"],
  ["food", "Food", "校园美食", "榜单投稿、异常票与调档确认。"],
] as const;

export const moduleSummaries: ModuleSummary[] = presentation.map(([id, name, eyebrow, description]) => ({
  id,
  name,
  eyebrow,
  description,
  status: "loading",
  metrics: [],
  statusMessage: "等待数据更新",
}));
