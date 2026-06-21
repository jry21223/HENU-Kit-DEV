import type { College, Course, Major, Material, School } from "@/types";

export const schools: School[] = [
  {
    id: "henu",
    name: "河南大学",
    slug: "henan-university",
    emailDomains: ["henu.edu.cn", "stu.henu.edu.cn"],
    status: "published",
  },
];

export const colleges: College[] = [
  {
    id: "software-college",
    schoolId: "henu",
    name: "软件学院",
  },
];

export const majors: Major[] = [
  {
    id: "network-engineering",
    schoolId: "henu",
    collegeId: "software-college",
    name: "网络工程",
    slug: "network-engineering",
  },
  {
    id: "software-engineering",
    schoolId: "henu",
    collegeId: "software-college",
    name: "软件工程",
    slug: "software-engineering",
  },
];

export const grades = ["2023级", "2024级"];

export const courses: Course[] = [
  {
    id: "discrete-math",
    schoolId: "henu",
    collegeId: "software-college",
    majorIds: ["network-engineering", "software-engineering"],
    grades: ["2023级", "2024级"],
    name: "离散数学",
    slug: "discrete-math",
    description: "覆盖集合、关系、图论、树、命题逻辑和谓词逻辑等期末重点。",
    examScope: "集合与关系、函数、图与树、命题逻辑、谓词逻辑、组合计数基础。",
    status: "published",
    teacher: "按课程考试范围整理",
    supportsPractice: true,
  },
  {
    id: "probability-statistics-a",
    schoolId: "henu",
    collegeId: "software-college",
    majorIds: ["network-engineering", "software-engineering"],
    grades: ["2023级"],
    name: "概率论与数理统计A",
    slug: "probability-statistics-a",
    description: "整理随机变量、分布、数字特征、参数估计和假设检验基础。",
    examScope: "随机事件、随机变量及分布、多维随机变量、数字特征、参数估计、假设检验。",
    status: "published",
    supportsPractice: true,
  },
  {
    id: "college-physics",
    schoolId: "henu",
    collegeId: "software-college",
    majorIds: ["network-engineering"],
    grades: ["2023级"],
    name: "大学物理",
    slug: "college-physics",
    description: "面向软件学院工科基础课，突出电磁学常考模型和公式应用。",
    examScope: "静电场、稳恒磁场、电磁感应、振动与波动基础。",
    status: "published",
    supportsPractice: false,
  },
  {
    id: "advanced-math-a",
    schoolId: "henu",
    collegeId: "software-college",
    majorIds: ["network-engineering", "software-engineering"],
    grades: ["2024级"],
    name: "高等数学A",
    slug: "advanced-math-a",
    description: "覆盖极限、导数、积分、级数和多元函数基础题型。",
    examScope: "函数极限与连续、一元微积分、常微分方程初步、多元函数微分法。",
    status: "published",
    supportsPractice: false,
  },
  {
    id: "software-engineering",
    schoolId: "henu",
    collegeId: "software-college",
    majorIds: ["software-engineering"],
    grades: ["2023级"],
    name: "软件工程",
    slug: "software-engineering",
    description: "围绕需求、设计、测试、项目管理和软件过程模型整理。",
    examScope: "软件过程、需求工程、概要设计、详细设计、测试、维护与项目管理。",
    status: "published",
    supportsPractice: false,
  },
  {
    id: "mobile-development",
    schoolId: "henu",
    collegeId: "software-college",
    majorIds: ["software-engineering", "network-engineering"],
    grades: ["2023级"],
    name: "移动开发",
    slug: "mobile-development",
    description: "整理移动端页面、组件、网络请求、状态管理和常见期末代码题。",
    examScope: "移动端基础组件、页面导航、网络请求、本地存储、项目结构与调试。",
    status: "published",
    supportsPractice: false,
  },
];

export const materials: Material[] = [
  {
    id: "discrete-note",
    courseId: "discrete-math",
    title: "离散数学重点知识点讲义",
    type: "knowledge_note",
    description: "按章节汇总定义、定理、例题和易错点。",
    fileName: "discrete-math-note.pdf",
    fileSize: "2.4 MB",
    previewContent: "集合运算、等价关系、偏序关系、图的连通性、树的性质与逻辑推理题型。",
    accessLevel: "login_required",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "discrete-sample",
    courseId: "discrete-math",
    title: "离散数学样例资料",
    type: "other",
    description: "用于 Phase 1 展示免费资料权限状态。",
    fileName: "discrete-math-sample.pdf",
    fileSize: "480 KB",
    previewContent: "本样例展示资料简介和权限标签，真实下载将在 Phase 5 实现。",
    accessLevel: "free",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "discrete-mock-1",
    courseId: "discrete-math",
    title: "离散数学模拟卷一",
    type: "mock_paper",
    description: "贴近期末题型的模拟试卷。",
    fileName: "discrete-math-mock-1.pdf",
    fileSize: "1.1 MB",
    previewContent: "选择、填空、证明和图论综合题。",
    accessLevel: "paid",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "discrete-mock-2",
    courseId: "discrete-math",
    title: "离散数学模拟卷二",
    type: "mock_paper",
    description: "覆盖高频考点的第二套模拟卷。",
    fileName: "discrete-math-mock-2.pdf",
    fileSize: "1.2 MB",
    previewContent: "关系性质、图的遍历、命题逻辑等综合训练。",
    accessLevel: "paid",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "discrete-answer",
    courseId: "discrete-math",
    title: "离散数学答案解析",
    type: "answer",
    description: "模拟卷和重点题型的步骤解析。",
    fileName: "discrete-math-answer.pdf",
    fileSize: "1.6 MB",
    previewContent: "按题型说明关键步骤、证明思路和常见扣分点。",
    accessLevel: "paid",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "discrete-quick-review",
    courseId: "discrete-math",
    title: "离散数学考前速背版",
    type: "quick_review",
    description: "考前一晚快速回顾公式、定义和定理。",
    fileName: "discrete-math-quick-review.pdf",
    fileSize: "860 KB",
    previewContent: "核心定义、图论结论、逻辑等价式和证明模板。",
    accessLevel: "login_required",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "probability-note",
    courseId: "probability-statistics-a",
    title: "概率论重点知识点讲义",
    type: "knowledge_note",
    description: "常见分布、数字特征和统计推断重点。",
    fileName: "probability-note.pdf",
    fileSize: "2.1 MB",
    previewContent: "二项分布、泊松分布、正态分布、期望方差、参数估计。",
    accessLevel: "login_required",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "probability-mock-1",
    courseId: "probability-statistics-a",
    title: "概率论模拟卷一",
    type: "mock_paper",
    description: "覆盖概率统计A期末高频题型。",
    fileName: "probability-mock-1.pdf",
    fileSize: "1.0 MB",
    previewContent: "随机变量分布、数字特征、区间估计和假设检验。",
    accessLevel: "paid",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "probability-answer",
    courseId: "probability-statistics-a",
    title: "概率论答案解析",
    type: "answer",
    description: "模拟卷答案和关键公式推导。",
    fileName: "probability-answer.pdf",
    fileSize: "1.4 MB",
    previewContent: "按步骤展示分布计算、估计量和检验统计量选择。",
    accessLevel: "paid",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "physics-em-note",
    courseId: "college-physics",
    title: "大学物理电磁学重点整理",
    type: "knowledge_note",
    description: "电磁学公式、模型和典型例题。",
    fileName: "physics-em-note.pdf",
    fileSize: "1.9 MB",
    previewContent: "高斯定理、安培环路定理、电磁感应和简单电路模型。",
    accessLevel: "login_required",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "physics-mock-1",
    courseId: "college-physics",
    title: "大学物理模拟卷一",
    type: "mock_paper",
    description: "电磁学重点题型模拟。",
    fileName: "physics-mock-1.pdf",
    fileSize: "1.3 MB",
    previewContent: "场强、磁感应强度、电磁感应和能量计算。",
    accessLevel: "paid",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "physics-answer",
    courseId: "college-physics",
    title: "大学物理答案解析",
    type: "answer",
    description: "大学物理模拟题解析。",
    fileName: "physics-answer.pdf",
    fileSize: "1.5 MB",
    previewContent: "列式思路、单位检查和高频错误提醒。",
    accessLevel: "paid",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "advanced-math-note",
    courseId: "advanced-math-a",
    title: "高等数学A重点知识点讲义",
    type: "knowledge_note",
    description: "高数A期末核心公式和题型整理。",
    fileName: "advanced-math-note.pdf",
    fileSize: "2.7 MB",
    previewContent: "极限、导数、积分、微分方程和多元函数微分。",
    accessLevel: "login_required",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "software-engineering-note",
    courseId: "software-engineering",
    title: "软件工程知识点讲义",
    type: "knowledge_note",
    description: "软件工程概念、过程模型和设计题答题框架。",
    fileName: "software-engineering-note.pdf",
    fileSize: "1.8 MB",
    previewContent: "瀑布模型、敏捷、需求分析、设计原则、测试策略。",
    accessLevel: "login_required",
    status: "published",
    updatedAt: "2026-06-20",
  },
  {
    id: "mobile-dev-note",
    courseId: "mobile-development",
    title: "移动开发重点讲义",
    type: "knowledge_note",
    description: "移动开发期末代码题和概念题整理。",
    fileName: "mobile-dev-note.pdf",
    fileSize: "2.0 MB",
    previewContent: "页面生命周期、组件通信、网络请求和本地存储。",
    accessLevel: "login_required",
    status: "published",
    updatedAt: "2026-06-20",
  },
];

export function getPublishedCourses() {
  return courses.filter((course) => course.status === "published");
}

export function getPublishedMaterialsByCourse(courseId: string) {
  return materials.filter(
    (material) => material.courseId === courseId && material.status === "published",
  );
}

export function getMaterialCount(courseId: string) {
  return getPublishedMaterialsByCourse(courseId).length;
}

export function findCourse(courseId: string) {
  return getPublishedCourses().find((course) => course.id === courseId);
}

export function findMaterial(materialId: string) {
  return materials.find(
    (material) => material.id === materialId && material.status === "published",
  );
}

export function getCourseMajors(course: Course) {
  return majors.filter((major) => course.majorIds.includes(major.id));
}

export function getSchoolName(schoolId: string) {
  return schools.find((school) => school.id === schoolId)?.name ?? "未知学校";
}

export function getCollegeName(collegeId: string) {
  return colleges.find((college) => college.id === collegeId)?.name ?? "未知学院";
}
