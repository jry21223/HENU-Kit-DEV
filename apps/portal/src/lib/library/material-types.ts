/**
 * Public Library owner types projected one-to-one from the publishable
 * HENU-Final-Review canonical roles. "slides" classifies a source courseware
 * file only; it does not enable online preview.
 */
export const MATERIAL_TYPES = {
  handout: { name: "复习讲义" },
  exam: { name: "往年真题" },
  slides: { name: "课件" },
  exercise: { name: "题库练习" },
  answer: { name: "答案解析" },
  note: { name: "笔记总结" },
  textbook: { name: "电子版教材" },
} as const;

export type MaterialType = keyof typeof MATERIAL_TYPES;
