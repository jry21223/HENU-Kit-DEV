/**
 * 开发环境用的资料库静态元数据。没有资料正文、幻灯片或在线预览；
 * 生产环境必须从 Library owner 加载目录并通过 owner 下载入口走 OSS。
 * 不保存任何用户拥有、收藏或购买状态。
 */

// ---------------------------------------------------------------- 类型

import type { MaterialType } from "./material-types";

export { MATERIAL_TYPES } from "./material-types";
export type { MaterialType } from "./material-types";

/** 遗留类型占位；OSS-only 资料永远不填充在线幻灯片。 */
export interface Slide {
  title: string;
  blocks?: string[];
}

export interface Material {
  id: string;
  type: MaterialType;
  subject: string;
  title: string;
  author: string; // 上传学长
  intro: string;
  toc: string[];
  pages: string[][]; // OSS-only contract: always empty
  pageCount?: number; // 原文件历史页数元数据，不代表可在线阅读
  price: number; // 0 = 免费（积分）
  previewPages: number; // OSS-only contract: always 0
  rating?: number; // 0-5；owner 未提供时不展示伪造评分
  downloads: number;
  favs?: number;
  /** 是否展示 owner 下载入口；静态 mock 不构造下载地址或授权 */
  downloadAvailable: boolean;
  fileSize?: number;
  /** OSS-only contract: absent/empty */
  slides?: Slide[];
}

// ---------------------------------------------------------------- mock 资料

const MATERIALS: Material[] = [
  // ---- 免费 ----
  {
    id: "free-limit-note", type: "handout", subject: "高等数学A",
    title: "极限与连续 · 学霸笔记", author: "21 级-李学长",
    intro: "期末 92 分学长的一轮复习笔记：两个重要极限、等价无穷小替换表、连续与间断点判定的全部套路，附 12 道易错例题。",
    toc: ["极限的定义与运算法则", "两个重要极限", "等价无穷小替换表", "连续性判定", "间断点分类", "易错例题 12 则"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.8, downloads: 2103, favs: 486, downloadAvailable: false,
  },
  {
    id: "free-ds-exam24", type: "exam", subject: "数据结构",
    title: "数据结构 · 2024 期末试卷 A 卷", author: "王助教",
    intro: "2024 秋季学期真题（含参考答案要点）：选择 20 分、填空 20 分、应用题 40 分、算法设计 20 分。树与图占比最高。",
    toc: ["一、选择题", "二、填空题", "三、应用题", "四、算法设计题", "参考答案要点"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.9, downloads: 3567, favs: 812, downloadAvailable: false,
  },
  {
    id: "free-la-cards", type: "note", subject: "线性代数",
    title: "矩阵运算公式手卡", author: "22 级-赵同学",
    intro: "A4 单页可打印：矩阵乘法、转置、逆、伴随、秩的全部高频公式与 6 个经典反例。",
    toc: ["乘法规则", "转置与逆", "伴随矩阵", "秩的不等式", "经典反例"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.6, downloads: 1588, favs: 302, downloadAvailable: false,
  },
  {
    id: "free-cet4-path", type: "note", subject: "大学英语",
    title: "四级 30 天备考路径", author: "20 级-陈学姐",
    intro: "四级 612 分学姐的 30 天冲刺日程表：每天 2 小时的单词/听力/阅读/写译分配，含各题型提分技巧与资料清单。",
    toc: ["第 1-10 天：单词+听力筑基", "第 11-20 天：阅读专项", "第 21-26 天：写译模板", "第 27-30 天：整卷模考", "资料清单"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.7, downloads: 1921, favs: 445, downloadAvailable: false,
  },
  {
    id: "free-phy-pendulum", type: "note", subject: "大学物理",
    title: "力学实验报告：单摆测重力加速度", author: "23 级-周同学",
    intro: "得分 95 的实验报告模板：实验原理、数据表格、不确定度计算完整过程，附教师批注过的易扣分点。",
    toc: ["实验目的", "实验原理", "数据记录", "数据处理", "误差分析", "易扣分点"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.5, downloads: 986, favs: 158, downloadAvailable: false,
  },
  {
    id: "free-ds-graph-note", type: "note", subject: "数据结构",
    title: "树与图知识图谱笔记", author: "21 级-李学长",
    intro: "把树和图揉成一张网：二叉树 5 个性质、三种遍历互推、图的两种存储、DFS/BFS、最小生成树双算法、最短路径。",
    toc: ["二叉树性质", "遍历互推", "图的存储", "DFS/BFS", "最小生成树", "最短路径"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.8, downloads: 1754, favs: 398, downloadAvailable: false,
  },
  {
    id: "free-math-mid-mock", type: "exercise", subject: "高等数学A",
    title: "期中模拟卷（一）", author: "数学学习小组",
    intro: "按近三年期中真题风格命制：极限 30%、导数与微分 40%、微分中值定理 30%，附评分标准。",
    toc: ["一、填空题", "二、计算题", "三、证明题", "评分标准"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.4, downloads: 1245, favs: 211, downloadAvailable: false,
  },
  {
    id: "free-ds-path", type: "note", subject: "数据结构",
    title: "数据结构：从入门到期末", author: "21 级-李学长",
    intro: "12 周学习路线：每周章节+刷题量+自测点，配套本库真题，适合期初收藏期末救命。",
    toc: ["第 1-2 周 线性表", "第 3-4 周 栈与队列", "第 5-7 周 树", "第 8-9 周 图", "第 10-11 周 查找与排序", "第 12 周 总复习"],
    pages: [],
    price: 0, previewPages: 0, rating: 4.7, downloads: 1432, favs: 366, downloadAvailable: false,
  },

  // ---- 收费 ----
  // Legacy paid metadata is non-downloadable and contains no preview body.
  {
    id: "paid-math-exam25", type: "exam", subject: "高等数学A",
    title: "高等数学A · 2025 期末试卷 + 逐题详解", author: "刘助教",
    intro: "2025 春季期末真题，每题附完整解题过程与评分点标注：积分应用与级数是本轮重点，压轴题为旋转体体积。",
    toc: ["一、填空题", "二、计算题", "三、应用题", "四、证明题", "逐题详解"],
    pages: [],
    pageCount: 8, price: 60, previewPages: 0, rating: 4.9, downloads: 2876, favs: 924, downloadAvailable: false,
  },
  {
    id: "paid-ds-3years", type: "exam", subject: "数据结构",
    title: "数据结构 · 2023-2025 三年真题合集", author: "王助教",
    intro: "三年 6 套真题 + 答案要点，树与图应用题逐年对比分析：2025 年图占比首次超过树，AVL 旋转连续三年出现。",
    toc: ["2023 A/B 卷", "2024 A/B 卷", "2025 A/B 卷", "三年考点对比", "答案要点"],
    pages: [],
    pageCount: 7, price: 80, previewPages: 0, rating: 4.9, downloads: 3122, favs: 1105, downloadAvailable: false,
  },
  {
    id: "paid-la-eigen", type: "note", subject: "线性代数",
    title: "特征值专题突破笔记", author: "22 级-赵同学",
    intro: "线代最难一章的 20 页攻坚：特征值/特征向量求法、相似对角化条件、实对称矩阵三大性质，含 8 道阶梯训练。",
    toc: ["定义与求法", "相似与对角化", "实对称矩阵", "阶梯训练 8 题"],
    pages: [],
    pageCount: 6, price: 45, previewPages: 0, rating: 4.7, downloads: 1654, favs: 431, downloadAvailable: false,
  },
  {
    id: "paid-phy-em-labs", type: "note", subject: "大学物理",
    title: "电磁学实验报告合集（4 篇）", author: "23 级-周同学",
    intro: "霍尔效应、示波器使用、螺线管磁场测定、RLC 稳态四篇 90+ 报告，数据表格与不确定度计算齐全。",
    toc: ["霍尔效应测磁场", "示波器的使用", "螺线管磁场分布", "RLC 稳态特性"],
    pages: [],
    pageCount: 5, price: 50, previewPages: 0, rating: 4.6, downloads: 987, favs: 203, downloadAvailable: false,
  },
  {
    id: "paid-math-5mocks", type: "exercise", subject: "高等数学A",
    title: "期末冲刺模拟卷（五套）", author: "数学学习小组",
    intro: "五套全真模拟：覆盖极限、微分、积分、级数全部题型，每套附答案与难度标注，适合考前两周每天一套。",
    toc: ["卷一（基础）", "卷二（基础+）", "卷三（中等）", "卷四（中等+）", "卷五（拔高）", "答案速查"],
    pages: [],
    pageCount: 6, price: 70, previewPages: 0, rating: 4.8, downloads: 2210, favs: 687, downloadAvailable: false,
  },
  {
    id: "paid-la-sprint", type: "note", subject: "线性代数",
    title: "线代一周速成路线", author: "22 级-赵同学",
    intro: "给只剩 7 天的人：每天一章 + 对应真题，舍小保大——行列式与矩阵计算必须全对，证明题背 4 个模板。",
    toc: ["D1 行列式", "D2 矩阵", "D3 向量组", "D4 方程组", "D5 特征值", "D6 二次型", "D7 模考"],
    pages: [],
    pageCount: 6, price: 30, previewPages: 0, rating: 4.5, downloads: 1308, favs: 296, downloadAvailable: false,
  },
  {
    id: "paid-cet6-writing", type: "note", subject: "大学英语",
    title: "六级写作模板 50 句", author: "20 级-陈学姐",
    intro: "六级 588 分学姐的写作弹药库：开头 10 句、论证 20 句、结尾 10 句、闪光替换词 10 组，覆盖近五年全部题型。",
    toc: ["开头段 10 句", "主体论证 20 句", "结尾段 10 句", "闪光替换 10 组"],
    pages: [],
    pageCount: 5, price: 40, previewPages: 0, rating: 4.6, downloads: 1517, favs: 354, downloadAvailable: false,
  },
  {
    id: "paid-ds-lab", type: "note", subject: "数据结构",
    title: "上机实验报告：链表与栈", author: "21 级-吴学长",
    intro: "数据结构前两次上机的满分报告：单链表基本操作集 + 顺序栈与表达式求值，代码、测试用例、复杂度分析齐全。",
    toc: ["实验一 单链表", "代码与注释", "实验二 顺序栈", "表达式求值", "测试与结论"],
    pages: [],
    pageCount: 5, price: 35, previewPages: 0, rating: 4.7, downloads: 876, favs: 167, downloadAvailable: false,
  },
];

/** 静态预生成用 */
export const STATIC_MATERIALS = MATERIALS;

export function getMaterial(id: string) {
  return MATERIALS.find((m) => m.id === id);
}
