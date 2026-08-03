import {
  BadgeCheck,
  BookMarked,
  Bot,
  BrainCircuit,
  ClipboardCheck,
  Coins,
  FileQuestion,
  FileText,
  Gift,
  GraduationCap,
  MessageSquareText,
  NotebookPen,
  PenLine,
  ShieldCheck,
  Sparkles,
  Tags,
  UsersRound,
} from "lucide-react";
import type {
  ArchiveDirectoryItem,
  CourseBook,
  GuaranteeItem,
  RecentUpdate,
  VisionFeature,
  VisionNote,
} from "./home-types";

export const archiveDirectory: ArchiveDirectoryItem[] = [
  { label: "课程资料", href: "/courses", description: "按课程进入讲义、真题、解析和实验资料。" },
  { label: "历年真题", href: "/courses", description: "围绕课程归档考前练习和解析入口。" },
  { label: "实验资料", href: "/courses", description: "实验报告、代码材料和环境说明集中维护。" },
  { label: "复习笔记", href: "/courses", description: "考前速记、重点整理和同学贡献内容。" },
  { label: "课程包", href: "/courses", description: "按课程打包解锁付费资料和复习组合。" },
  { label: "刷题练习", href: "/courses", description: "从课程题目进入练习、提交和错题沉淀。" },
  { label: "我的下载", href: "/me/downloads", description: "登录后回看自己的资料下载记录。" },
];

export const courseBooks: CourseBook[] = [
  { label: "数据结构", code: "CS201", subtitle: "Data Structures", meta: "讲义 / 真题 / 题目", href: "/courses", tone: "blue" },
  { label: "操作系统", code: "CS301", subtitle: "Operating Systems", meta: "实验 / 速背 / 错题", href: "/courses", tone: "orange" },
  { label: "计算机网络", code: "CS302", subtitle: "Computer Networks", meta: "协议 / 真题 / 讲义", href: "/courses", tone: "green" },
  { label: "软件工程", code: "SE301", subtitle: "Software Engineering", meta: "复习包 / 案例 / 笔记", href: "/courses", tone: "pink" },
  { label: "数据库系统", code: "DB202", subtitle: "Database Systems", meta: "SQL / 讲义 / 实验", href: "/courses", tone: "purple" },
  { label: "编译原理", code: "CS401", subtitle: "Compilers", meta: "语法 / 题目 / 资料", href: "/courses", tone: "cyan" },
];

export const communityNotes: VisionNote[] = [
  { title: "Wiki 共创", body: "一起整理知识点、资料说明和课程复习脉络。", tone: "yellow", tilt: "left" },
  { title: "博客经验", body: "沉淀复习路线、考前经验和避坑提醒。", tone: "pink", tilt: "right" },
  { title: "课程帖子", body: "围绕资料补充、勘误、提问和讨论展开。", tone: "blue", tilt: "none" },
  { title: "学习动态", body: "记录资料更新、同学贡献和课程相关活动。", tone: "green", tilt: "right" },
];

export const practiceFeatures: VisionFeature[] = [
  { title: "在线刷题", body: "从课程题目进入练习，提交后得到即时反馈。", icon: ClipboardCheck },
  { title: "错题本", body: "把做错的题目沉淀下来，复习时回到具体课程。", icon: NotebookPen },
  { title: "薄弱点统计", body: "按课程和知识点统计薄弱环节，减少盲目复习。", icon: BrainCircuit },
  { title: "AI 针对性强化", body: "围绕错题和资料给出复习提示，生成内容先进入审核边界。", icon: Bot },
];

export const membershipFeatures: VisionFeature[] = [
  { title: "创作者积分", body: "资料补充、内容共创和课程经验沉淀都能进入激励体系。", icon: Coins },
  { title: "会员权益", body: "会员和积分共同控制资料权益、课程包权益和 AI 使用额度。", icon: BadgeCheck },
  { title: "课程包", body: "以课程为单位组织付费资料，解锁仍由服务端校验。", icon: Gift },
  { title: "成本控制", body: "AI 能力按积分或会员额度使用，避免无边界消耗。", icon: Tags },
];

export const guaranteeItems: GuaranteeItem[] = [
  { title: "资料稳定供应", body: "课程 PDF 按资料库规则维护，避免临时网盘式失效。", icon: BookMarked },
  { title: "轻水印", body: "下载资料带有不影响阅读的来源标识。", icon: Sparkles },
  { title: "权限校验", body: "资料按公开、登录、付费规则由服务端校验。", icon: ShieldCheck },
  { title: "审核边界", body: "AI 草稿和贡献内容先进入审核流程，再成为正式内容。", icon: PenLine },
];

export const salesFeatures: VisionFeature[] = [
  { title: "群内咨询", body: "面向 QQ/微信群访客解释课程包和平台权益。", icon: MessageSquareText },
  { title: "购买引导", body: "把咨询转成注册、课程包购买和售后分流。", icon: UsersRound },
];

export const recentUpdates: RecentUpdate[] = [
  { title: "数据结构期末真题解析", course: "数据结构", href: "/courses", label: "真题解析" },
  { title: "操作系统考前速背清单", course: "操作系统", href: "/courses", label: "复习笔记" },
  { title: "计算机网络协议分层讲义", course: "计算机网络", href: "/courses", label: "讲义资料" },
  { title: "软件工程复习包说明", course: "软件工程", href: "/courses", label: "课程包" },
];

export const heroLinks = {
  primary: { label: "进入工作区", href: "/workspace" },
  secondary: { label: "浏览课程资料", href: "/courses" },
};

export const directoryIcons = {
  materials: FileText,
  quiz: FileQuestion,
  course: GraduationCap,
};
