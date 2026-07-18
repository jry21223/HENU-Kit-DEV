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

export const moduleSummaries: ModuleSummary[] = [
  {
    id: "portal",
    name: "Portal",
    eyebrow: "Public experience",
    status: "ok",
    description: "公开主站版本、入口与关键页面探针，只读呈现。",
    metrics: [
      { label: "部署版本", value: "2026.07.18", hint: "commit 85154e1" },
      { label: "关键页面", value: "6/6", hint: "全部可访问" },
    ],
    statusMessage: "最近一次探针正常",
    asOf: "18:32",
    trend: [
      { label: "周一", value: 18 },
      { label: "周二", value: 26 },
      { label: "周三", value: 22 },
      { label: "周四", value: 31 },
      { label: "周五", value: 28 },
    ],
  },
  {
    id: "platform",
    name: "Platform Operations",
    eyebrow: "Shared platform",
    status: "partial",
    description: "账户、权限、Session、邮件、审计与 Operations Inbox。",
    metrics: [
      { label: "活跃 Session", value: "184", hint: "24h +12" },
      { label: "待办", value: "9", hint: "2 项接近 SLA" },
    ],
    statusMessage: "邮件供应商指标暂缺，其他 4 个来源可用",
    requestId: "req_mock_platform_02",
  },
  {
    id: "notice",
    name: "Notice",
    eyebrow: "Campus notices",
    status: "empty",
    description: "通知来源、版本、审核、受众与分发状态。",
    metrics: [],
    statusMessage: "当前没有待审核通知，这是正常空状态",
    asOf: "18:31",
  },
  {
    id: "library",
    name: "Library",
    eyebrow: "Learning materials",
    status: "stale",
    description: "课程、资料、下载、投稿、审核与纠错摘要。",
    metrics: [
      { label: "已发布资料", value: "1,284", hint: "本周 +36" },
      { label: "待审核", value: "17", hint: "最早 3h" },
    ],
    statusMessage: "当前展示最近一次成功快照",
    asOf: "18:26",
    lastSuccessAt: "18:26",
  },
  {
    id: "quizcraft",
    name: "QuizCraft",
    eyebrow: "Practice product",
    status: "unavailable",
    description: "题库、反馈与服务状态摘要；题库工坊仍在产品自有后台。",
    metrics: [],
    statusMessage: "摘要服务超过 2 秒模块超时",
    requestId: "req_mock_quiz_05",
  },
  {
    id: "food",
    name: "Food",
    eyebrow: "Campus food",
    status: "denied",
    description: "榜单投稿、异常票与调档确认。",
    metrics: [],
    statusMessage: "缺少 food.ranking.read 权限；未请求或展示业务数据",
    requestId: "req_mock_food_06",
  },
];
