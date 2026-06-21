export const materialTypeLabels = {
  knowledge_note: "知识点讲义",
  mock_paper: "模拟卷",
  answer: "答案解析",
  quick_review: "考前速背版",
  past_exam: "历年真题",
  other: "其他资料",
} as const;

export const accessLevelLabels = {
  free: "免费",
  login_required: "登录可用",
  paid: "需解锁",
} as const;

export const courseStatusLabels = {
  draft: "草稿",
  published: "已发布",
  archived: "已归档",
} as const;

export const questionTypeLabels = {
  single_choice: "单选题",
  multiple_choice: "多选题",
  true_false: "判断题",
  blank: "填空题",
  calculation: "计算题",
  proof: "证明题",
} as const;
