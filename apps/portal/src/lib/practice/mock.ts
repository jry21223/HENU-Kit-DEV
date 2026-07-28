/**
 * 刷题子站 v1 mock 数据层。
 * 所有随机性使用种子化伪随机（mulberry32），保证 SSR 与客户端输出一致。
 */

// ---------------------------------------------------------------- 基础

function mulberry32(seed: number) {
  return function () {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** 两位小数定点（SVG 坐标/数值确定性输出，防水合不匹配） */
export const r2 = (v: number) => Number(v.toFixed(2));

// ---------------------------------------------------------------- 类型

export interface Question {
  id: string;
  subject: string;
  chapter: string;
  difficulty: number; // 1.0 - 10.0，一位小数
  stem: string;
  options: [string, string, string, string];
  answer: number; // 0-3
  explanation: string;
  accuracy: number; // 全网正确率 %
}

export interface QuizListMeta {
  id: string;
  name: string;
  creator: string;
  tags: string[];
  poolKey: PoolKey;
  count: number; // 取题库前 N 题
  completion: number; // 完成度 %
}

export interface Subject {
  id: string;
  name: string;
  lists: QuizListMeta[];
}

export interface Major {
  id: string;
  name: string;
  subjects: Subject[];
}

export interface School {
  id: string;
  name: string;
  majors: Major[];
}

// ---------------------------------------------------------------- 题库（题目内容）

type PoolKey = "ds" | "math" | "la" | "os";

const rand = mulberry32(20260719);

/** 由难度分推算全网正确率（确定性） */
function accuracyOf(difficulty: number) {
  return Math.round(Math.min(96, Math.max(18, 94 - difficulty * 8 + rand() * 8)));
}

function q(
  pool: PoolKey,
  seq: number,
  subject: string,
  chapter: string,
  difficulty: number,
  stem: string,
  options: [string, string, string, string],
  answer: number,
  explanation: string
): Question {
  return {
    id: `${pool}-${String(seq).padStart(2, "0")}`,
    subject,
    chapter,
    difficulty,
    stem,
    options,
    answer,
    explanation,
    accuracy: accuracyOf(difficulty),
  };
}

const DS: Question[] = [
  q("ds", 1, "数据结构", "排序", 2.5,
    "下列排序算法中，最坏情况下时间复杂度为 O(n²) 的是？",
    ["归并排序", "快速排序", "堆排序", "基数排序"], 1,
    "快速排序在序列已有序时划分极不平衡，退化为 O(n²)；归并与堆排序任何情况下都是 O(n log n)。"),
  q("ds", 2, "数据结构", "链表", 3.0,
    "在带头结点的单链表中，删除首元结点的时间复杂度是？",
    ["O(1)", "O(log n)", "O(n)", "O(n log n)"], 0,
    "只需把头结点的 next 指向第二个结点即可，与表长无关，故为 O(1)。"),
  q("ds", 3, "数据结构", "栈与队列", 3.5,
    "对后缀表达式（逆波兰式）求值，最适合使用的数据结构是？",
    ["队列", "栈", "单链表", "哈希表"], 1,
    "扫描到操作数入栈，遇到运算符弹出栈顶两个操作数运算后压回，是栈的经典应用。"),
  q("ds", 4, "数据结构", "图", 4.0,
    "对图进行广度优先搜索（BFS）时，通常需要借助哪种数据结构？",
    ["栈", "队列", "二叉树", "并查集"], 1,
    "BFS 按层次访问，先访问的顶点先扩展其邻接点，先进先出，故用队列。"),
  q("ds", 5, "数据结构", "树", 5.0,
    "含 n 个结点的二叉链表中，空指针域的个数为？",
    ["n - 1", "n", "n + 1", "2n"], 2,
    "指针域共 2n 个，非空指针等于分支数 n - 1，故空指针域 = 2n - (n - 1) = n + 1。"),
  q("ds", 6, "数据结构", "树", 5.5,
    "具有 n 个结点的完全二叉树，其深度为？",
    ["⌊log₂n⌋", "⌊log₂n⌋ + 1", "⌈log₂n⌉", "n / 2"], 1,
    "完全二叉树深度 k 满足 2^(k-1) ≤ n < 2^k，解得 k = ⌊log₂n⌋ + 1。"),
  q("ds", 7, "数据结构", "查找", 6.0,
    "用开放定址法处理哈希冲突时，线性探测容易产生的问题是？",
    ["二次聚集", "一次聚集（堆积）", "链表过长", "无法删除元素"], 1,
    "线性探测使冲突记录连续占用相邻单元，形成一次聚集，显著拉长平均查找长度。"),
  q("ds", 8, "数据结构", "排序", 6.5,
    "快速排序的平均时间复杂度是？",
    ["O(n)", "O(n log n)", "O(n²)", "O(log n)"], 1,
    "快排平均划分较均衡，递归深度 O(log n)，每层划分 O(n)，平均 O(n log n)。"),
  q("ds", 9, "数据结构", "树", 7.5,
    "平衡二叉树（AVL 树）中任一结点的平衡因子取值范围是？",
    ["{0, 1}", "{-1, 0, 1}", "{-2, -1, 0, 1, 2}", "任意整数"], 1,
    "AVL 要求左右子树高度差绝对值不超过 1，故平衡因子只能取 -1、0、1。"),
  q("ds", 10, "数据结构", "图", 8.0,
    "拓扑排序适用于下列哪种图？",
    ["无向连通图", "有向无环图（DAG）", "无向完全图", "带权无向图"], 1,
    "拓扑排序针对 AOV 网，前提是不存在回路，即有向无环图。"),
];

const MATH: Question[] = [
  q("math", 1, "高等数学A", "极限", 2.0,
    "极限 lim(x→0) (sin x) / x 的值为？",
    ["0", "1", "e", "不存在"], 1,
    "重要极限之一：x→0 时 sin x 与 x 为等价无穷小，极限为 1（注意 x 以弧度计）。"),
  q("math", 2, "高等数学A", "极限", 3.0,
    "极限 lim(x→∞) (1 + 1/x)^x 的值为？",
    ["1", "1/e", "e", "+∞"], 2,
    "第二个重要极限，其结果为自然对数的底 e，是 1^∞ 型极限的标准结论。"),
  q("math", 3, "高等数学A", "连续", 3.5,
    "函数 f(x) 在点 x₀ 处连续的充要条件是？",
    ["f(x) 在 x₀ 处有定义", "f(x) 在 x₀ 处左右极限存在",
     "lim(x→x₀) f(x) = f(x₀)", "f(x) 在 x₀ 处可导"], 2,
    "连续的定义即极限值等于函数值，它同时蕴含了有定义与极限存在。"),
  q("math", 4, "高等数学A", "导数", 4.0,
    "设 f(x) 在 x₀ 处可导，则 f′(x₀) 等于？",
    ["lim(h→0) [f(x₀+h) − f(x₀)] / h", "lim(h→0) [f(x₀) − f(x₀−h)] / (2h)",
     "lim(h→∞) [f(x₀+h) − f(x₀)] / h", "lim(h→0) [f(x₀+2h) − f(x₀)] / h"], 0,
    "导数定义即差商在增量趋于 0 时的极限；D 项结果应为 2f′(x₀)。"),
  q("math", 5, "高等数学A", "导数", 4.5,
    "关于可导与连续的关系，下列说法正确的是？",
    ["连续必可导", "可导必连续，连续不一定可导",
     "可导与连续互不相干", "不连续也可能可导"], 1,
    "可导 ⇒ 连续；反例 f(x) = |x| 在 x = 0 处连续但不可导。"),
  q("math", 6, "高等数学A", "微分中值定理", 5.0,
    "洛必达法则可直接用于下列哪种未定式？",
    ["0/0 型", "1^∞ 型", "0·∞ 型", "∞ − ∞ 型"], 0,
    "洛必达法则只直接适用于 0/0 与 ∞/∞；其余类型须先变形转化。"),
  q("math", 7, "高等数学A", "微分中值定理", 6.0,
    "拉格朗日中值定理成立的条件是 f(x) 满足？",
    ["在 (a,b) 内连续即可", "在 [a,b] 上连续，在 (a,b) 内可导",
     "在 [a,b] 上可导", "在 (a,b) 内有界"], 1,
    "闭区间连续 + 开区间可导是拉格朗日中值定理的标准条件。"),
  q("math", 8, "高等数学A", "积分", 5.5,
    "不定积分 ∫ (1/x) dx 等于？",
    ["ln x + C", "ln|x| + C", "−1/x² + C", "x·ln x + C"], 1,
    "被积函数在 x<0 也有定义，必须写成 ln|x| + C；A 项丢失了负数区间的原函数。"),
  q("math", 9, "高等数学A", "积分", 6.5,
    "定积分 ∫[a,b] f(x) dx 的几何意义是？",
    ["曲线与 x 轴围成的面积（恒为正）", "曲边梯形面积的代数和",
     "曲线上点的纵坐标之和", "原函数在 [a,b] 上的增量"], 1,
    "f 在 x 轴下方时对应部分取负，故为面积的代数和；D 是牛顿-莱布尼茨公式的表述。"),
  q("math", 10, "高等数学A", "级数", 8.0,
    "e^x 的麦克劳林展开式为？",
    ["Σ (−1)ⁿ xⁿ / n!", "Σ xⁿ / n!", "Σ x²ⁿ / (2n)!", "Σ (−1)ⁿ x²ⁿ⁺¹ / (2n+1)!"], 1,
    "e^x 的各阶导数均为 e^x，在 0 处取值 1，故展开为 Σ xⁿ/n!，收敛域为全体实数。"),
];

const LA: Question[] = [
  q("la", 1, "线性代数", "行列式", 3.0,
    "互换行列式的两行（列），行列式的值？",
    ["不变", "变号", "变为 0", "变为原来的 2 倍"], 1,
    "行列式的基本性质：互换两行（列），行列式变号。"),
  q("la", 2, "线性代数", "行列式", 4.5,
    "设 A、B 均为 n 阶方阵，则 |AB| 等于？",
    ["|A| + |B|", "|A| · |B|", "|A| − |B|", "|A| / |B|"], 1,
    "行列式乘法定理：同阶方阵乘积的行列式等于行列式的乘积。"),
  q("la", 3, "线性代数", "矩阵", 3.5,
    "关于矩阵乘法，下列说法正确的是？",
    ["满足交换律", "一般不满足交换律",
     "AB = 0 则必有 A = 0 或 B = 0", "满足消去律"], 1,
    "矩阵乘法一般 AB ≠ BA，且存在非零矩阵乘积为零矩阵的情况。"),
  q("la", 4, "线性代数", "矩阵", 5.0,
    "n 阶方阵 A 可逆的充要条件是？",
    ["A 的元素均不为 0", "|A| ≠ 0", "A 为对角矩阵", "A 的各行不成比例"], 1,
    "A 可逆 ⟺ |A| ≠ 0（此时 r(A) = n，A 非奇异）。"),
  q("la", 5, "线性代数", "秩", 6.5,
    "设 A 为 m×n 矩阵，B 为 n×s 矩阵，则秩 r(AB) 满足？",
    ["r(AB) = r(A) + r(B)", "r(AB) ≥ r(A) + r(B)",
     "r(AB) ≤ min{ r(A), r(B) }", "r(AB) = min{ r(A), r(B) }"], 2,
    "乘积的秩不超过各因子秩的最小值，等号一般不成立。"),
  q("la", 6, "线性代数", "线性方程组", 7.0,
    "n 元齐次线性方程组 Ax = 0 有非零解的充要条件是？",
    ["r(A) = n", "r(A) < n", "|A| ≠ 0", "A 的行数大于列数"], 1,
    "有非零解 ⟺ 存在自由未知量 ⟺ r(A) < n（方阵情形等价于 |A| = 0）。"),
  q("la", 7, "线性代数", "特征值", 7.5,
    "n 阶方阵 A 的全部特征值之积等于？",
    ["A 的迹 tr(A)", "|A|", "A 的主对角线元素之和", "r(A)"], 1,
    "特征值之积 = |A|，特征值之和 = tr(A)，注意区分。"),
  q("la", 8, "线性代数", "特征值", 8.0,
    "关于实对称矩阵，下列结论正确的是？",
    ["特征值可能为复数", "必可相似对角化",
     "不同特征值的特征向量必平行", "一定可逆"], 1,
    "实对称矩阵特征值全为实数，不同特征值的特征向量正交，必可（正交）相似对角化。"),
];

const OS: Question[] = [
  q("os", 1, "操作系统", "进程管理", 2.5,
    "进程与程序的本质区别是？",
    ["进程占用内存，程序不占内存", "进程是动态的，程序是静态的",
     "进程可以并发，程序不能并发", "二者没有区别"], 1,
    "程序是指令的静态集合，进程是程序的一次动态执行过程，具有生命周期。"),
  q("os", 2, "操作系统", "进程管理", 3.5,
    "进程的基本三态模型不包括下列哪个状态？",
    ["就绪态", "运行态", "阻塞态", "挂起态"], 3,
    "基本三态为就绪、运行、阻塞；挂起态属于扩展的五态模型。"),
  q("os", 3, "操作系统", "进程同步", 5.0,
    "P 操作（wait 原语）对信号量 s 的影响是？",
    ["s 加 1，若 s ≤ 0 则唤醒进程", "s 减 1，若 s < 0 则进程阻塞",
     "s 减 1，若 s ≤ 0 则进程阻塞", "s 置为 0"], 1,
    "P 操作：s = s − 1；若 s < 0 说明资源不足，进程进入阻塞队列。"),
  q("os", 4, "操作系统", "死锁", 6.0,
    "产生死锁的四个必要条件不包括？",
    ["互斥条件", "占有且等待", "不可剥夺条件", "先来先服务"], 3,
    "四要素为互斥、占有且等待、不可剥夺、循环等待；先来先服务是调度策略。"),
  q("os", 5, "操作系统", "死锁", 6.5,
    "银行家算法用于处理死锁问题的策略是？",
    ["死锁预防", "死锁避免", "死锁检测", "死锁解除"], 1,
    "银行家算法在分配资源前检查系统是否处于安全状态，属于死锁避免。"),
  q("os", 6, "操作系统", "存储管理", 7.0,
    "关于分页与分段存储管理，下列说法正确的是？",
    ["段长固定，页长可变", "页是信息的物理单位，大小固定",
     "分页会产生外部碎片", "分段对用户不可见"], 1,
    "页是信息的物理单位、大小固定，消除了外部碎片；段是信息的逻辑单位，段长可变。"),
  q("os", 7, "操作系统", "存储管理", 7.5,
    "LRU（最近最久未使用）页面置换算法淘汰的页面是？",
    ["最先进入内存的页面", "最近最久未被访问的页面",
     "访问次数最少的页面", "以后不再使用的页面"], 1,
    "LRU 依据局部性原理，淘汰最近最长时间未被引用的页面；D 是理想 OPT。"),
  q("os", 8, "操作系统", "存储管理", 8.0,
    "虚拟存储器技术的理论基础是？",
    ["程序的局部性原理", "中断技术", "通道技术", "SPOOLing 技术"], 0,
    "程序执行呈现时间与空间局部性，故可只装入部分页面运行，即虚拟存储。"),
];

const POOLS: Record<PoolKey, Question[]> = { ds: DS, math: MATH, la: LA, os: OS };

// ---------------------------------------------------------------- 层级数据：学院 → 专业 → 科目 → 题单

export const SCHOOLS: School[] = [
  {
    id: "cs",
    name: "计算机与信息工程学院",
    majors: [
      {
        id: "se",
        name: "软件工程",
        subjects: [
          {
            id: "se-ds",
            name: "数据结构",
            lists: [
              { id: "ds-final", name: "数据结构 · 期末冲刺 50 题", creator: "王助教", tags: ["期末", "高频考点"], poolKey: "ds", count: 10, completion: 68 },
              { id: "ds-tree", name: "数据结构 · 树与图专题", creator: "21 级-李学长", tags: ["专题", "考研"], poolKey: "ds", count: 8, completion: 24 },
            ],
          },
          {
            id: "se-os",
            name: "操作系统",
            lists: [
              { id: "os-core", name: "操作系统 · 核心概念精选", creator: "王助教", tags: ["期末", "概念"], poolKey: "os", count: 8, completion: 41 },
            ],
          },
        ],
      },
      {
        id: "cst",
        name: "计算机科学与技术",
        subjects: [
          {
            id: "cst-ds",
            name: "数据结构",
            lists: [
              { id: "ds-kaoyan", name: "数据结构 · 考研基础题", creator: "20 级-陈学姐", tags: ["考研", "基础"], poolKey: "ds", count: 8, completion: 12 },
            ],
          },
        ],
      },
    ],
  },
  {
    id: "maths",
    name: "数学与统计学院",
    majors: [
      {
        id: "am",
        name: "数学与应用数学",
        subjects: [
          {
            id: "am-math",
            name: "高等数学A",
            lists: [
              { id: "math-limit", name: "高等数学A · 极限与连续", creator: "刘助教", tags: ["期中", "基础"], poolKey: "math", count: 10, completion: 83 },
              { id: "math-mvt", name: "高等数学A · 中值定理专题", creator: "22 级-赵同学", tags: ["专题", "难点"], poolKey: "math", count: 8, completion: 36 },
            ],
          },
          {
            id: "am-la",
            name: "线性代数",
            lists: [
              { id: "la-matrix", name: "线性代数 · 矩阵与行列式", creator: "刘助教", tags: ["期末", "计算"], poolKey: "la", count: 8, completion: 57 },
            ],
          },
        ],
      },
    ],
  },
  {
    id: "phy",
    name: "物理与电子学院",
    majors: [
      {
        id: "ee",
        name: "电子信息工程",
        subjects: [
          {
            id: "ee-math",
            name: "高等数学A",
            lists: [
              { id: "math-midterm", name: "高等数学A · 期中复习卷", creator: "23 级-周同学", tags: ["期中"], poolKey: "math", count: 8, completion: 9 },
            ],
          },
        ],
      },
    ],
  },
];

// ---------------------------------------------------------------- 查询辅助

export function getListById(id: string): QuizListMeta | undefined {
  for (const school of SCHOOLS)
    for (const major of school.majors)
      for (const subject of major.subjects) {
        const hit = subject.lists.find((l) => l.id === id);
        if (hit) return hit;
      }
  return undefined;
}

export function getAllLists(): QuizListMeta[] {
  return SCHOOLS.flatMap((s) =>
    s.majors.flatMap((m) => m.subjects.flatMap((sub) => sub.lists))
  );
}

/** 题单的题目：取对应题库前 count 题，按难度分升序 */
export function getListQuestions(list: QuizListMeta): Question[] {
  return POOLS[list.poolKey]
    .slice(0, list.count)
    .slice()
    .sort((a, b) => a.difficulty - b.difficulty);
}

/** 刷题界面固定使用数据结构整组（v1 不加载真实题单） */
export const QUIZ_SET: Question[] = DS;

/** 题目状态（确定性）：0 未做 / 1 已做 / 2 做错 */
export function questionStatus(listId: string, seq: number): 0 | 1 | 2 {
  const r = mulberry32(listId.length * 100 + seq * 7 + 3)();
  if (r < 0.45) return 1;
  if (r < 0.65) return 2;
  return 0;
}

// ---------------------------------------------------------------- 用户数据面板

export const USER_STATS = {
  totalQuestions: 486,
  accuracy: 76,
  streakDays: 12,
  beatPercent: 83,
  mastery: [
    { label: "数据结构", value: 81 },
    { label: "高等数学A", value: 78 },
    { label: "操作系统", value: 64 },
    { label: "线性代数", value: 52 },
  ],
  weakTop5: [
    { topic: "特征值与特征向量", subject: "线性代数", wrong: 18 },
    { topic: "泰勒展开", subject: "高等数学A", wrong: 15 },
    { topic: "页面置换算法", subject: "操作系统", wrong: 12 },
    { topic: "平衡二叉树", subject: "数据结构", wrong: 11 },
    { topic: "微分中值定理", subject: "高等数学A", wrong: 9 },
  ],
};

/** 能力分时间序列（0–100，确定性随机游走） */
function walk(seed: number, points: number, start: number, drift: number): number[] {
  const r = mulberry32(seed);
  const out: number[] = [];
  let v = start;
  for (let i = 0; i < points; i++) {
    v += drift + (r() - 0.5) * 6;
    v = Math.min(98, Math.max(20, v));
    out.push(r2(v));
  }
  return out;
}

export const ABILITY_SERIES: Record<"d30" | "d180" | "d520", number[]> = {
  d30: walk(101, 30, 52, 0.9),
  d180: walk(202, 60, 41, 0.7),
  d520: walk(303, 58, 30, 0.9), // 520 天按约 9 天采样，≤ 60 点
};

/** 科目雷达图（用户专业 mock 为软件工程） */
export const RADAR_DATA = [
  { subject: "数据结构", value: 82 },
  { subject: "操作系统", value: 74 },
  { subject: "计算机网络", value: 61 },
  { subject: "高等数学A", value: 78 },
  { subject: "线性代数", value: 55 },
  { subject: "概率论", value: 66 },
];

// ---------------------------------------------------------------- 日历热力图（近 26 周）

export interface HeatCell {
  date: string; // "M月D日"
  count: number; // 0-20
}

const HEAT_END_UTC = Date.UTC(2026, 6, 19); // 固定基准日，保证 SSR/CSR 一致

export const HEATMAP: HeatCell[] = (() => {
  const r = mulberry32(404);
  const cells: HeatCell[] = [];
  for (let i = 26 * 7 - 1; i >= 0; i--) {
    const d = new Date(HEAT_END_UTC - i * 86400000);
    const dow = d.getUTCDay();
    let base = r() * 6;
    if (dow === 0 || dow === 6) base *= 1.8; // 周末更多
    const week = Math.floor((26 * 7 - 1 - i) / 7);
    if (week === 19) base *= 2.6; // 考试周峰值
    if (r() < 0.18) base = 0; // 摸鱼日
    cells.push({
      date: `${d.getUTCMonth() + 1}月${d.getUTCDate()}日`,
      count: Math.round(Math.min(20, base)),
    });
  }
  return cells;
})();
