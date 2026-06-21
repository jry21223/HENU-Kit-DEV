import { AiJobStatus, MaterialType } from "@prisma/client";

export type AiOutputType =
  | "knowledge_note"
  | "mock_paper"
  | "answer"
  | "quick_review"
  | "past_exam"
  | "other";

export const aiOutputTypeOptions: Array<{ value: AiOutputType; label: string }> = [
  { value: "knowledge_note", label: "知识点讲义" },
  { value: "mock_paper", label: "模拟试卷" },
  { value: "answer", label: "答案解析" },
  { value: "quick_review", label: "考前速背版" },
  { value: "past_exam", label: "历年真题整理" },
  { value: "other", label: "其他资料" },
];

const outputTypeMap: Record<AiOutputType, MaterialType> = {
  knowledge_note: MaterialType.KNOWLEDGE_NOTE,
  mock_paper: MaterialType.MOCK_PAPER,
  answer: MaterialType.ANSWER,
  quick_review: MaterialType.QUICK_REVIEW,
  past_exam: MaterialType.PAST_EXAM,
  other: MaterialType.OTHER,
};

const outputTypeReverseMap: Record<MaterialType, AiOutputType> = {
  KNOWLEDGE_NOTE: "knowledge_note",
  MOCK_PAPER: "mock_paper",
  ANSWER: "answer",
  QUICK_REVIEW: "quick_review",
  PAST_EXAM: "past_exam",
  OTHER: "other",
};

const statusMap: Record<string, AiJobStatus> = {
  queued: AiJobStatus.QUEUED,
  running: AiJobStatus.RUNNING,
  succeeded: AiJobStatus.SUCCEEDED,
  failed: AiJobStatus.FAILED,
  cancelled: AiJobStatus.CANCELLED,
};

const statusReverseMap: Record<AiJobStatus, "queued" | "running" | "succeeded" | "failed" | "cancelled"> = {
  QUEUED: "queued",
  RUNNING: "running",
  SUCCEEDED: "succeeded",
  FAILED: "failed",
  CANCELLED: "cancelled",
};

export function parseAiOutputType(value?: string | null): MaterialType | null {
  if (!value) {
    return null;
  }
  return outputTypeMap[value as AiOutputType] ?? null;
}

export function mapAiOutputType(type: MaterialType): AiOutputType {
  return outputTypeReverseMap[type];
}

export function parseAiJobStatus(value?: string | null): AiJobStatus | undefined {
  return value ? statusMap[value] : undefined;
}

export function mapAiJobStatus(status: AiJobStatus) {
  return statusReverseMap[status];
}

export function getAiOutputTypeLabel(type: MaterialType) {
  const appType = mapAiOutputType(type);
  return aiOutputTypeOptions.find((option) => option.value === appType)?.label ?? "其他资料";
}

export function buildAiDraftContent(input: {
  courseName: string;
  outputType: MaterialType;
  sourceTitles: string[];
}) {
  const label = getAiOutputTypeLabel(input.outputType);
  const sources = input.sourceTitles.length > 0 ? input.sourceTitles.join("、") : "暂无指定来源资料";

  return {
    title: `${input.courseName}${label}草稿`,
    description: `AI 辅助生成的${label}草稿，需人工审核后才能发布。`,
    previewContent: [
      `# ${input.courseName}${label}草稿`,
      "",
      "本内容由本地 AI 任务流程生成，用于模拟后续真实 AI 接入。",
      `来源资料：${sources}`,
      "",
      "## 人工审核要求",
      "",
      "- 核对知识点覆盖范围。",
      "- 核对公式、答案和推导过程。",
      "- 确认无误后再由管理员发布。",
    ].join("\n"),
  };
}
