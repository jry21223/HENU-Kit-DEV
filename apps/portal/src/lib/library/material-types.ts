/**
 * Public Library owner types projected one-to-one from the publishable
 * HENU-Final-Review canonical roles. "slides" classifies a source courseware
 * file only; it does not enable online preview.
 */
export const MATERIAL_TYPES = {
  handout: { name: "复习讲义", code: "HANDOUT" },
  exam: { name: "往年真题", code: "EXAM" },
  slides: { name: "课件", code: "COURSEWARE" },
  exercise: { name: "题库练习", code: "EXERCISE" },
  answer: { name: "答案解析", code: "ANSWER" },
  note: { name: "笔记总结", code: "NOTE" },
  textbook: { name: "电子版教材", code: "TEXTBOOK" },
} as const;

export type MaterialType = keyof typeof MATERIAL_TYPES;
