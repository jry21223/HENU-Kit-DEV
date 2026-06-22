import type { PracticeQuestionType } from "@/types";

export type MockQuestion = {
  id: string;
  courseId: string;
  knowledgePointId?: string;
  knowledgePointTitle?: string;
  type: PracticeQuestionType;
  stem: string;
  options: Array<{ id: string; text: string }>;
  answer: string;
  explanation: string;
  difficulty: number;
  status: "draft" | "published" | "archived";
};

export const mockQuestions: MockQuestion[] = [
  {
    id: "discrete-q-logic-1",
    courseId: "discrete-math",
    knowledgePointId: "kp-discrete-logic",
    knowledgePointTitle: "命题逻辑",
    type: "single_choice",
    stem: "命题公式 P -> Q 与下列哪个公式等价？",
    options: [
      { id: "A", text: "非 P 或 Q" },
      { id: "B", text: "P 且 Q" },
      { id: "C", text: "非 Q 或 P" },
      { id: "D", text: "P 或 Q" },
    ],
    answer: "A",
    explanation: "蕴含式 P -> Q 等价于非 P 或 Q，这是化简命题公式的基础等价式。",
    difficulty: 1,
    status: "published",
  },
  {
    id: "discrete-q-graph-1",
    courseId: "discrete-math",
    knowledgePointId: "kp-discrete-graph",
    knowledgePointTitle: "图论基础",
    type: "true_false",
    stem: "任意一棵含 n 个顶点的树都有 n - 1 条边。",
    options: [
      { id: "true", text: "正确" },
      { id: "false", text: "错误" },
    ],
    answer: "true",
    explanation: "树是连通无回路图，含 n 个顶点时边数恒为 n - 1。",
    difficulty: 1,
    status: "published",
  },
  {
    id: "probability-q-distribution-1",
    courseId: "probability-statistics-a",
    knowledgePointId: "kp-probability-distribution",
    knowledgePointTitle: "常见分布",
    type: "single_choice",
    stem: "若 X 服从参数为 lambda 的泊松分布，则 E(X) 等于多少？",
    options: [
      { id: "A", text: "lambda" },
      { id: "B", text: "lambda 的平方" },
      { id: "C", text: "1 / lambda" },
      { id: "D", text: "0" },
    ],
    answer: "A",
    explanation: "泊松分布的期望和方差都等于参数 lambda。",
    difficulty: 1,
    status: "published",
  },
];

export function getPublishedMockQuestionsByCourse(courseId: string) {
  return mockQuestions.filter(
    (question) => question.courseId === courseId && question.status === "published",
  );
}

export function findPublishedMockQuestion(questionId: string) {
  return mockQuestions.find(
    (question) => question.id === questionId && question.status === "published",
  );
}
