import {
  MaterialAccessLevel,
  MaterialStatus,
  MaterialType,
  Prisma,
  PrismaClient,
  QuestionType,
  RecordStatus,
  UserRole,
} from "@prisma/client";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

const prisma = new PrismaClient();

const courseSeeds = [
  {
    id: "discrete-math",
    name: "离散数学",
    slug: "discrete-math",
    majorIds: ["network-engineering", "software-engineering"],
    grades: ["2023级", "2024级"],
    description: "覆盖集合、关系、图论、树、命题逻辑和谓词逻辑等期末重点。",
    examScope: "集合与关系、函数、图与树、命题逻辑、谓词逻辑、组合计数基础。",
    teacher: "按课程考试范围整理",
  },
  {
    id: "probability-statistics-a",
    name: "概率论与数理统计A",
    slug: "probability-statistics-a",
    majorIds: ["network-engineering", "software-engineering"],
    grades: ["2023级"],
    description: "整理随机变量、分布、数字特征、参数估计和假设检验基础。",
    examScope: "随机事件、随机变量及分布、多维随机变量、数字特征、参数估计、假设检验。",
  },
  {
    id: "college-physics",
    name: "大学物理",
    slug: "college-physics",
    majorIds: ["network-engineering"],
    grades: ["2023级"],
    description: "面向软件学院工科基础课，突出电磁学常考模型和公式应用。",
    examScope: "静电场、稳恒磁场、电磁感应、振动与波动基础。",
  },
  {
    id: "advanced-math-a",
    name: "高等数学A",
    slug: "advanced-math-a",
    majorIds: ["network-engineering", "software-engineering"],
    grades: ["2024级"],
    description: "覆盖极限、导数、积分、级数和多元函数基础题型。",
    examScope: "函数极限与连续、一元微积分、常微分方程初步、多元函数微分法。",
  },
  {
    id: "software-engineering",
    name: "软件工程",
    slug: "software-engineering",
    majorIds: ["software-engineering"],
    grades: ["2023级"],
    description: "围绕需求、设计、测试、项目管理和软件过程模型整理。",
    examScope: "软件过程、需求工程、概要设计、详细设计、测试、维护与项目管理。",
  },
  {
    id: "mobile-development",
    name: "移动开发",
    slug: "mobile-development",
    majorIds: ["network-engineering", "software-engineering"],
    grades: ["2023级"],
    description: "整理移动端页面、组件、网络请求、状态管理和常见期末代码题。",
    examScope: "移动端基础组件、页面导航、网络请求、本地存储、项目结构与调试。",
  },
] as const;

const materialSeeds = [
  {
    id: "discrete-note",
    courseId: "discrete-math",
    title: "离散数学重点知识点讲义",
    type: MaterialType.KNOWLEDGE_NOTE,
    description: "按章节汇总定义、定理、例题和易错点。",
    fileName: "discrete-math-note.pdf",
    fileSize: 2_400_000,
    previewContent: "集合运算、等价关系、偏序关系、图的连通性、树的性质与逻辑推理题型。",
    accessLevel: MaterialAccessLevel.LOGIN_REQUIRED,
  },
  {
    id: "discrete-sample",
    courseId: "discrete-math",
    title: "离散数学样例资料",
    type: MaterialType.OTHER,
    description: "用于未登录下载验收的免费样例资料。",
    fileName: "discrete-math-sample.pdf",
    fileSize: 480_000,
    previewContent: "本样例展示资料简介和权限标签，真实下载将在 Phase 5 实现。",
    accessLevel: MaterialAccessLevel.FREE,
  },
  {
    id: "discrete-mock-1",
    courseId: "discrete-math",
    title: "离散数学模拟卷一",
    type: MaterialType.MOCK_PAPER,
    description: "贴近期末题型的模拟试卷。",
    fileName: "discrete-math-mock-1.pdf",
    fileSize: 1_100_000,
    previewContent: "选择、填空、证明和图论综合题。",
    accessLevel: MaterialAccessLevel.PAID,
  },
  {
    id: "discrete-mock-2",
    courseId: "discrete-math",
    title: "离散数学模拟卷二",
    type: MaterialType.MOCK_PAPER,
    description: "覆盖高频考点的第二套模拟卷。",
    fileName: "discrete-math-mock-2.pdf",
    fileSize: 1_200_000,
    previewContent: "关系性质、图的遍历、命题逻辑等综合训练。",
    accessLevel: MaterialAccessLevel.PAID,
  },
  {
    id: "discrete-answer",
    courseId: "discrete-math",
    title: "离散数学答案解析",
    type: MaterialType.ANSWER,
    description: "模拟卷和重点题型的步骤解析。",
    fileName: "discrete-math-answer.pdf",
    fileSize: 1_600_000,
    previewContent: "按题型说明关键步骤、证明思路和常见扣分点。",
    accessLevel: MaterialAccessLevel.PAID,
  },
  {
    id: "discrete-quick-review",
    courseId: "discrete-math",
    title: "离散数学考前速背版",
    type: MaterialType.QUICK_REVIEW,
    description: "考前一晚快速回顾公式、定义和定理。",
    fileName: "discrete-math-quick-review.pdf",
    fileSize: 860_000,
    previewContent: "核心定义、图论结论、逻辑等价式和证明模板。",
    accessLevel: MaterialAccessLevel.LOGIN_REQUIRED,
  },
  {
    id: "probability-note",
    courseId: "probability-statistics-a",
    title: "概率论重点知识点讲义",
    type: MaterialType.KNOWLEDGE_NOTE,
    description: "常见分布、数字特征和统计推断重点。",
    fileName: "probability-note.pdf",
    fileSize: 2_100_000,
    previewContent: "二项分布、泊松分布、正态分布、期望方差、参数估计。",
    accessLevel: MaterialAccessLevel.LOGIN_REQUIRED,
  },
  {
    id: "probability-mock-1",
    courseId: "probability-statistics-a",
    title: "概率论模拟卷一",
    type: MaterialType.MOCK_PAPER,
    description: "覆盖概率统计A期末高频题型。",
    fileName: "probability-mock-1.pdf",
    fileSize: 1_000_000,
    previewContent: "随机变量分布、数字特征、区间估计和假设检验。",
    accessLevel: MaterialAccessLevel.PAID,
  },
  {
    id: "probability-answer",
    courseId: "probability-statistics-a",
    title: "概率论答案解析",
    type: MaterialType.ANSWER,
    description: "模拟卷答案和关键公式推导。",
    fileName: "probability-answer.pdf",
    fileSize: 1_400_000,
    previewContent: "按步骤展示分布计算、估计量和检验统计量选择。",
    accessLevel: MaterialAccessLevel.PAID,
  },
  {
    id: "physics-em-note",
    courseId: "college-physics",
    title: "大学物理电磁学重点整理",
    type: MaterialType.KNOWLEDGE_NOTE,
    description: "电磁学公式、模型和典型例题。",
    fileName: "physics-em-note.pdf",
    fileSize: 1_900_000,
    previewContent: "高斯定理、安培环路定理、电磁感应和简单电路模型。",
    accessLevel: MaterialAccessLevel.LOGIN_REQUIRED,
  },
  {
    id: "physics-mock-1",
    courseId: "college-physics",
    title: "大学物理模拟卷一",
    type: MaterialType.MOCK_PAPER,
    description: "电磁学重点题型模拟。",
    fileName: "physics-mock-1.pdf",
    fileSize: 1_300_000,
    previewContent: "场强、磁感应强度、电磁感应和能量计算。",
    accessLevel: MaterialAccessLevel.PAID,
  },
  {
    id: "physics-answer",
    courseId: "college-physics",
    title: "大学物理答案解析",
    type: MaterialType.ANSWER,
    description: "大学物理模拟题解析。",
    fileName: "physics-answer.pdf",
    fileSize: 1_500_000,
    previewContent: "列式思路、单位检查和高频错误提醒。",
    accessLevel: MaterialAccessLevel.PAID,
  },
] as const;

type KnowledgePointSeed = {
  id: string;
  courseId: string;
  title: string;
  description: string;
  sortOrder: number;
};

const knowledgePointSeeds: KnowledgePointSeed[] = [
  {
    id: "kp-discrete-logic",
    courseId: "discrete-math",
    title: "命题逻辑",
    description: "命题联结词、等价式和推理规则。",
    sortOrder: 1,
  },
  {
    id: "kp-discrete-graph",
    courseId: "discrete-math",
    title: "图论基础",
    description: "图、树、连通性和边数性质。",
    sortOrder: 2,
  },
  {
    id: "kp-probability-distribution",
    courseId: "probability-statistics-a",
    title: "常见分布",
    description: "二项分布、泊松分布和正态分布的常用结论。",
    sortOrder: 1,
  },
  {
    id: "kp-physics-em",
    courseId: "college-physics",
    title: "电磁学基础",
    description: "电场、磁场和电磁感应基础概念。",
    sortOrder: 1,
  },
];

type QuestionSeed = {
  id: string;
  courseId: string;
  knowledgePointId: string;
  type: QuestionType;
  stem: string;
  options: Prisma.InputJsonValue;
  answer: Prisma.InputJsonValue;
  explanation: string;
  difficulty: number;
};

const questionSeeds: QuestionSeed[] = [
  {
    id: "discrete-q-logic-1",
    courseId: "discrete-math",
    knowledgePointId: "kp-discrete-logic",
    type: QuestionType.SINGLE_CHOICE,
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
  },
  {
    id: "discrete-q-graph-1",
    courseId: "discrete-math",
    knowledgePointId: "kp-discrete-graph",
    type: QuestionType.TRUE_FALSE,
    stem: "任意一棵含 n 个顶点的树都有 n - 1 条边。",
    options: [
      { id: "true", text: "正确" },
      { id: "false", text: "错误" },
    ],
    answer: "true",
    explanation: "树是连通无回路图，含 n 个顶点时边数恒为 n - 1。",
    difficulty: 1,
  },
  {
    id: "probability-q-distribution-1",
    courseId: "probability-statistics-a",
    knowledgePointId: "kp-probability-distribution",
    type: QuestionType.SINGLE_CHOICE,
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
  },
  {
    id: "physics-q-em-1",
    courseId: "college-physics",
    knowledgePointId: "kp-physics-em",
    type: QuestionType.TRUE_FALSE,
    stem: "电场强度是描述电场对单位正电荷作用力的物理量。",
    options: [
      { id: "true", text: "正确" },
      { id: "false", text: "错误" },
    ],
    answer: "true",
    explanation: "电场强度定义为单位正电荷在电场中所受电场力。",
    difficulty: 1,
  },
];

const packageSeeds = [
  {
    id: "discrete-math-final-package",
    title: "离散数学期末复习包",
    description: "包含离散数学模拟卷、答案解析和考前重点资料的单科复习包。",
    schoolId: "henu",
    majorId: null,
    grade: "2023级",
    price: new Prisma.Decimal("19.90"),
    status: RecordStatus.PUBLISHED,
    items: [
      { resourceType: "material", resourceId: "discrete-mock-1" },
      { resourceType: "material", resourceId: "discrete-mock-2" },
      { resourceType: "material", resourceId: "discrete-answer" },
    ],
  },
] as const;

async function main() {
  await prisma.school.upsert({
    where: { id: "henu" },
    update: {
      name: "河南大学",
      slug: "henan-university",
      emailDomains: ["henu.edu.cn", "stu.henu.edu.cn"],
      status: RecordStatus.PUBLISHED,
    },
    create: {
      id: "henu",
      name: "河南大学",
      slug: "henan-university",
      emailDomains: ["henu.edu.cn", "stu.henu.edu.cn"],
      status: RecordStatus.PUBLISHED,
    },
  });

  await prisma.college.upsert({
    where: { id: "software-college" },
    update: {
      schoolId: "henu",
      name: "软件学院",
    },
    create: {
      id: "software-college",
      schoolId: "henu",
      name: "软件学院",
    },
  });

  for (const major of [
    { id: "network-engineering", name: "网络工程", slug: "network-engineering" },
    { id: "software-engineering", name: "软件工程", slug: "software-engineering" },
  ]) {
    await prisma.major.upsert({
      where: { id: major.id },
      update: {
        schoolId: "henu",
        collegeId: "software-college",
        name: major.name,
        slug: major.slug,
      },
      create: {
        id: major.id,
        schoolId: "henu",
        collegeId: "software-college",
        name: major.name,
        slug: major.slug,
      },
    });
  }

  await prisma.user.upsert({
    where: { email: "admin@example.com" },
    update: {
      name: "开发环境管理员",
      schoolId: "henu",
      role: UserRole.ADMIN,
      emailVerified: true,
    },
    create: {
      id: "dev-admin",
      email: "admin@example.com",
      name: "开发环境管理员",
      schoolId: "henu",
      role: UserRole.ADMIN,
      emailVerified: true,
    },
  });

  for (const course of courseSeeds) {
    const teacher = "teacher" in course ? course.teacher : null;

    await prisma.course.upsert({
      where: { id: course.id },
      update: {
        schoolId: "henu",
        collegeId: "software-college",
        grades: [...course.grades],
        name: course.name,
        slug: course.slug,
        description: course.description,
        examScope: course.examScope,
        teacher,
        status: RecordStatus.PUBLISHED,
      },
      create: {
        id: course.id,
        schoolId: "henu",
        collegeId: "software-college",
        grades: [...course.grades],
        name: course.name,
        slug: course.slug,
        description: course.description,
        examScope: course.examScope,
        teacher,
        status: RecordStatus.PUBLISHED,
      },
    });

    await prisma.courseMajor.deleteMany({ where: { courseId: course.id } });
    await prisma.courseMajor.createMany({
      data: course.majorIds.map((majorId) => ({
        courseId: course.id,
        majorId,
      })),
      skipDuplicates: true,
    });
  }

  for (const material of materialSeeds) {
    await prisma.material.upsert({
      where: { id: material.id },
      update: {
        courseId: material.courseId,
        title: material.title,
        type: material.type,
        description: material.description,
        fileUrl: `/uploads/mock/${material.fileName}`,
        fileName: material.fileName,
        fileSize: material.fileSize,
        previewContent: material.previewContent,
        accessLevel: material.accessLevel,
        status: MaterialStatus.PUBLISHED,
        createdById: "dev-admin",
      },
      create: {
        id: material.id,
        courseId: material.courseId,
        title: material.title,
        type: material.type,
        description: material.description,
        fileUrl: `/uploads/mock/${material.fileName}`,
        fileName: material.fileName,
        fileSize: material.fileSize,
        previewContent: material.previewContent,
        accessLevel: material.accessLevel,
        status: MaterialStatus.PUBLISHED,
        createdById: "dev-admin",
      },
    });
  }

  for (const knowledgePoint of knowledgePointSeeds) {
    await prisma.knowledgePoint.upsert({
      where: { id: knowledgePoint.id },
      update: {
        courseId: knowledgePoint.courseId,
        title: knowledgePoint.title,
        description: knowledgePoint.description,
        sortOrder: knowledgePoint.sortOrder,
      },
      create: {
        id: knowledgePoint.id,
        courseId: knowledgePoint.courseId,
        title: knowledgePoint.title,
        description: knowledgePoint.description,
        sortOrder: knowledgePoint.sortOrder,
      },
    });
  }

  for (const question of questionSeeds) {
    await prisma.question.upsert({
      where: { id: question.id },
      update: {
        courseId: question.courseId,
        knowledgePointId: question.knowledgePointId,
        type: question.type,
        stem: question.stem,
        options: question.options,
        answer: question.answer,
        explanation: question.explanation,
        difficulty: question.difficulty,
        status: RecordStatus.PUBLISHED,
      },
      create: {
        id: question.id,
        courseId: question.courseId,
        knowledgePointId: question.knowledgePointId,
        type: question.type,
        stem: question.stem,
        options: question.options,
        answer: question.answer,
        explanation: question.explanation,
        difficulty: question.difficulty,
        status: RecordStatus.PUBLISHED,
      },
    });
  }

  for (const coursePackage of packageSeeds) {
    await prisma.coursePackage.upsert({
      where: { id: coursePackage.id },
      update: {
        title: coursePackage.title,
        description: coursePackage.description,
        schoolId: coursePackage.schoolId,
        majorId: coursePackage.majorId,
        grade: coursePackage.grade,
        price: coursePackage.price,
        status: coursePackage.status,
      },
      create: {
        id: coursePackage.id,
        title: coursePackage.title,
        description: coursePackage.description,
        schoolId: coursePackage.schoolId,
        majorId: coursePackage.majorId,
        grade: coursePackage.grade,
        price: coursePackage.price,
        status: coursePackage.status,
      },
    });

    await prisma.packageItem.deleteMany({ where: { packageId: coursePackage.id } });
    await prisma.packageItem.createMany({
      data: coursePackage.items.map((item) => ({
        packageId: coursePackage.id,
        resourceType: item.resourceType,
        resourceId: item.resourceId,
      })),
      skipDuplicates: true,
    });
  }

  const uploadDir = path.resolve(process.cwd(), "uploads", "mock");
  await mkdir(uploadDir, { recursive: true });
  for (const material of materialSeeds) {
    const filePath = path.join(uploadDir, material.fileName);
    const placeholderPdf = [
      "%PDF-1.4",
      "1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj",
      "2 0 obj << /Type /Pages /Count 1 /Kids [3 0 R] >> endobj",
      "3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >> endobj",
      "4 0 obj << /Length 44 >> stream",
      `BT /F1 12 Tf 72 720 Td (${material.title}) Tj ET`,
      "endstream endobj",
      "xref",
      "0 5",
      "0000000000 65535 f ",
      "trailer << /Root 1 0 R >>",
      "%%EOF",
    ].join("\n");
    await writeFile(filePath, placeholderPdf, "utf8");
  }

  const [schoolCount, majorCount, courseCount, materialCount, questionCount, packageCount] =
    await Promise.all([
      prisma.school.count(),
      prisma.major.count(),
      prisma.course.count(),
      prisma.material.count(),
      prisma.question.count(),
      prisma.coursePackage.count(),
    ]);

  console.log(
    `Seed complete: ${schoolCount} schools, ${majorCount} majors, ${courseCount} courses, ${materialCount} materials, ${questionCount} questions, ${packageCount} packages.`,
  );
}

main()
  .catch((error) => {
    console.error(error);
    process.exit(1);
  })
  .finally(async () => {
    await prisma.$disconnect();
  });
