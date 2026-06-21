import type {
  accessLevelLabels,
  courseStatusLabels,
  materialTypeLabels,
  questionTypeLabels,
} from "@/constants/enums";

export type School = {
  id: string;
  name: string;
  slug: string;
  emailDomains: string[];
  status: "published" | "archived";
};

export type College = {
  id: string;
  schoolId: string;
  name: string;
};

export type Major = {
  id: string;
  schoolId: string;
  collegeId: string;
  name: string;
  slug: string;
};

export type CourseStatus = keyof typeof courseStatusLabels;
export type MaterialType = keyof typeof materialTypeLabels;
export type AccessLevel = keyof typeof accessLevelLabels;
export type PracticeQuestionType = keyof typeof questionTypeLabels;

export type Course = {
  id: string;
  schoolId: string;
  collegeId: string;
  majorIds: string[];
  grades: string[];
  name: string;
  slug: string;
  description: string;
  examScope: string;
  status: CourseStatus;
  teacher?: string;
  supportsPractice: boolean;
  materialCount?: number;
  hasMockPaper?: boolean;
  hasAnswer?: boolean;
};

export type Material = {
  id: string;
  courseId: string;
  title: string;
  type: MaterialType;
  description: string;
  fileName: string;
  fileSize: string;
  previewContent: string;
  accessLevel: AccessLevel;
  status: "draft" | "pending_review" | "published" | "archived";
  updatedAt: string;
};

export type PracticeOption = {
  id: string;
  text: string;
};

export type PracticeQuestion = {
  id: string;
  courseId: string;
  knowledgePointId?: string;
  knowledgePointTitle?: string;
  type: PracticeQuestionType;
  stem: string;
  options: PracticeOption[];
  difficulty: number;
};

export type QuestionSubmitResult = {
  questionId: string;
  isCorrect: boolean;
  submittedAnswer: string[];
  correctAnswer: string[];
  correctAnswerLabel: string;
  explanation: string;
  wrongQuestionSaved: boolean;
};

export type WrongQuestionItem = {
  id: string;
  createdAt: string;
  course: {
    id: string;
    name: string;
  };
  knowledgePointTitle?: string;
  question: PracticeQuestion;
};

export type WrongQuestionFilters = {
  courseId?: string;
  knowledgePointId?: string;
};

export type WeakPointItem = {
  course: {
    id: string;
    name: string;
  };
  knowledgePointId?: string;
  knowledgePointTitle: string;
  wrongCount: number;
};

export type PackageStatus = "draft" | "published" | "archived";

export type CoursePackageItem = {
  id: string;
  resourceType: "material" | "course" | "question_set";
  resourceId: string;
  title: string;
  accessLevel?: AccessLevel;
};

export type CoursePackage = {
  id: string;
  title: string;
  description: string;
  schoolId: string;
  schoolName?: string;
  majorId?: string;
  majorName?: string;
  grade?: string;
  price: string;
  status: PackageStatus;
  itemCount: number;
  unlocked: boolean;
  items?: CoursePackageItem[];
};
