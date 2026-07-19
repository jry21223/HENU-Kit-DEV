/**
 * 互助平台 mock 数据层 + campusStore（useSyncExternalStore 单例，同 accountStore/foodStore 模式）。
 * 字面量数据，SSR/客户端一致；想要/留言/发布/接单/确认完成/管理操作仅会话内存态，刷新还原。
 */

import { seedImg } from "@/lib/image";

// ---------------------------------------------------------------- 类型与分类

export type ItemType = "help" | "sell";
export type ItemStatus = "open" | "ongoing" | "done" | "hidden";

export interface Category {
  key: string;
  name: string;
  code: string; // 卡片编号代号
}

export const CATEGORIES: Category[] = [
  { key: "errand", name: "跑腿代办", code: "RUN" },
  { key: "express", name: "代取快递", code: "EXP" },
  { key: "luggage", name: "搬运行李", code: "LUG" },
  { key: "seat", name: "占座打卡", code: "SEA" },
  { key: "skill", name: "技能服务", code: "SKI" },
  { key: "flea", name: "闲置出售", code: "FLE" },
];

export const categoryOf = (key: string) =>
  CATEGORIES.find((c) => c.key === key) ?? CATEGORIES[0];

export interface Item {
  id: string;
  type: ItemType;
  category: string;
  title: string;
  desc: string;
  price: number;
  seller: string;
  credit: number; // 信用分 0-100
  dealsDone: number; // 成交数
  wants: number;
  place: string;
  deadline?: string; // 仅求助单
  status: ItemStatus;
  isMine?: boolean;
  wanted?: boolean;
  time: string;
  /** 1-3 张图片，首图为卡片缩略图 */
  images?: string[];
}

export interface DealMessage {
  id: string;
  itemId: string;
  author: string;
  time: string;
  text: string;
}

export interface Deal {
  id: string;
  itemId: string;
  title: string;
  price: number;
  other: string; // 对方（卖家/发单人）
  role: "taker" | "buyer"; // 接单 / 我买
  status: "ongoing" | "done";
  timeline: { label: string; time: string }[];
}

// ---------------------------------------------------------------- mock 单子

function item(
  id: string,
  type: ItemType,
  category: string,
  title: string,
  desc: string,
  price: number,
  seller: string,
  credit: number,
  dealsDone: number,
  wants: number,
  place: string,
  time: string,
  opts?: { deadline?: string; status?: ItemStatus; isMine?: boolean }
): Item {
  return {
    id, type, category, title, desc, price, seller, credit, dealsDone, wants,
    place, time,
    deadline: opts?.deadline,
    status: opts?.status ?? "open",
    isMine: opts?.isMine,
  };
}

const INITIAL_ITEMS: Item[] = [
  item("h-01", "help", "express", "代取中通快递 3 件到 6 号楼",
    "快递在明伦西门菜鸟驿站，三个小件，取件码私发。送到 6 号宿舍楼楼下即可，谢谢！",
    3, "取快递困难户", 86, 12, 45, "明伦校区 · 西门驿站", "07-19 10:20",
    { deadline: "今天 18:00 前" }),
  item("h-02", "help", "luggage", "开学搬行李上六楼（无电梯）",
    "两个 28 寸行李箱 + 一个编织袋，从校门口搬到桃李园 6 楼。需要一个力气大的同学，预计 20 分钟搞定。",
    15, "你", 91, 3, 28, "金明校区 · 桃李园", "07-19 09:05",
    { deadline: "本周五下午", isMine: true }),
  item("h-03", "help", "skill", "代做数据结构课程小项目（哈夫曼编码）",
    "课程小项目：实现哈夫曼编码/解码，要求有注释和测试用例，下周五前交付，代码查重率需低于 20%。",
    80, "DDL 战士", 79, 31, 67, "线上交付", "07-18 22:40",
    { deadline: "下周五 23:59 前", status: "ongoing" }),
  item("h-04", "help", "seat", "早八占座：图书馆三楼靠窗位",
    "每周二/四早 7:30 前帮我占三楼靠窗单人位，占到我 8:10 到为止。长期合作优先。",
    5, "早八在逃人员", 90, 46, 52, "明伦图书馆", "07-18 21:15",
    { deadline: "每周二/四 7:30" }),
  item("h-05", "help", "errand", "代拿外卖：南门到仁和学生公寓",
    "黄焖鸡米饭一份，外卖柜在南门岗亭旁，帮忙拿到仁和 3 号楼楼下。",
    2, "宿舍躺平学家", 75, 8, 19, "金明校区 · 南门", "07-18 12:30",
    { deadline: "今天 13:00 前" }),
  item("h-06", "help", "skill", "高数期末考前答疑 2 小时",
    "极限、微分中值定理、定积分三章串讲，自带往年卷。地点约在东综教学楼自习室。",
    40, "你", 91, 3, 33, "龙子湖校区 · 东综", "07-17 20:00",
    { deadline: "本周日 14:00", status: "ongoing", isMine: true }),
  item("h-07", "help", "seat", "代打卡：周四下午心理健康讲座",
    "周四 14:00 综合楼 301 讲座，需要签到+签退，中间可以离场，结束后把签到表照片发我。",
    6, "讲座绝缘体", 68, 5, 21, "明伦校区 · 综合楼", "07-17 16:44",
    { deadline: "周四 14:00" }),
  item("h-08", "help", "errand", "帮忙组装宜家书架（毕利系列）",
    "新买的毕利书架到了，宿舍没工具也不会装，求人带螺丝刀来组装，请喝奶茶。",
    25, "手残党本党", 82, 2, 16, "金明校区 · 南苑", "07-16 19:20",
    { deadline: "本周末均可" }),
  item("s-01", "sell", "flea", "九成新机械键盘 青轴 87 键",
    "用了三个月，无打油无暗病，箱说全。宿舍太吵换静音轴了，爽快可小刀。",
    120, "你", 91, 3, 58, "金明校区 · 可送到楼下", "07-18 15:30",
    { isMine: true }),
  item("s-02", "sell", "flea", "考研英语红宝书 全新未拆封",
    "25 版红宝书，买重复了，全新塑封未拆，原价 ¥59 转出。",
    25, "考研上岸ing", 95, 27, 41, "明伦校区", "07-18 11:10"),
  item("s-03", "sell", "flea", "宿舍小冰箱 46L 用了一年",
    "制冷正常，无异味，毕业出。需自提，六楼无电梯，建议两人来搬。",
    180, "毕业清仓中", 88, 19, 73, "明伦校区 · 自提", "07-17 18:25",
    { status: "done" }),
  item("s-04", "sell", "flea", "永久牌自行车 26 寸",
    "骑了一年半，刹车刚换过，送车锁和打气筒。车况看图不如来看实物，随时约看车。",
    150, "风一样的男子", 84, 11, 36, "龙子湖校区", "07-17 09:40"),
  item("s-05", "sell", "flea", "四六级听力耳机 只用过两次",
    "考四六级买的，就用过两次，电池仓干净。附送两节新电池。",
    20, "你", 91, 3, 24, "金明校区", "07-16 14:05",
    { isMine: true, status: "hidden" }),
  item("s-06", "sell", "flea", "素描石膏几何体套装（美术生用）",
    "石膏几何体 8 件套，结构素描课买的，课程结束出，无缺角。",
    30, "画不完的图", 92, 7, 15, "明伦校区 · 美术学院", "07-15 17:50"),
  item("s-07", "sell", "flea", "校园网千兆路由器",
    "TP-LINK 千兆双频，宿舍组网神器，毕业出，配件齐全。",
    45, "网线另一端", 89, 14, 22, "金明校区", "07-15 10:35"),
];

const INITIAL_MESSAGES: DealMessage[] = [
  { id: "m1", itemId: "h-01", author: "顺路侠", time: "07-19 10:35", text: "我中午正好要去驿站，可以接。" },
  { id: "m2", itemId: "h-01", author: "取快递困难户", time: "07-19 10:40", text: "好！我私你取件码。" },
  { id: "m3", itemId: "s-01", author: "键盘侠", time: "07-18 16:02", text: "什么牌子的？轴体是樱桃青吗？" },
  { id: "m4", itemId: "s-01", author: "你", time: "07-18 16:20", text: "国产轴，手感接近青轴，介意勿拍。" },
  { id: "m5", itemId: "h-03", author: "代码炼丹师", time: "07-18 23:10", text: "已接，今晚开工，周四给你初版。" },
  { id: "m6", itemId: "s-04", author: "早八不迟到", time: "07-17 11:00", text: "还在吗？明天中午能看车吗？" },
];

const INITIAL_DEALS: Deal[] = [
  {
    id: "D-260701", itemId: "h-03", title: "代做数据结构课程小项目（哈夫曼编码）", price: 80,
    other: "DDL 战士", role: "taker", status: "ongoing",
    timeline: [
      { label: "发布", time: "07-18 22:40" },
      { label: "赏金托管", time: "07-18 22:41" },
      { label: "接单服务", time: "07-18 23:05" },
    ],
  },
  {
    id: "D-260688", itemId: "s-03", title: "宿舍小冰箱 46L 用了一年", price: 180,
    other: "毕业清仓中", role: "buyer", status: "done",
    timeline: [
      { label: "发布", time: "07-17 18:25" },
      { label: "赏金托管", time: "07-17 18:30" },
      { label: "接单服务", time: "07-18 10:00" },
      { label: "确认完成", time: "07-18 19:20" },
      { label: "平台结算", time: "07-19 09:00" },
    ],
  },
];

// ---------------------------------------------------------------- campusStore

export interface CampusData {
  items: Item[];
  messages: DealMessage[];
  deals: Deal[];
}

const INITIAL: CampusData = {
  items: INITIAL_ITEMS,
  messages: INITIAL_MESSAGES,
  deals: INITIAL_DEALS,
};

/** 静态预生成用 */
export const STATIC_ITEMS = INITIAL_ITEMS;

// seed 图注入（确定性 picsum 外链；离线/失败时 Img 组件回退图纸占位块）
INITIAL_ITEMS.find((i) => i.id === "s-01")!.images = [
  seedImg("henu-keyboard", 800, 500),
  seedImg("henu-keyboard-keys", 800, 500),
];
INITIAL_ITEMS.find((i) => i.id === "s-03")!.images = [seedImg("henu-fridge", 800, 500)];
INITIAL_ITEMS.find((i) => i.id === "s-04")!.images = [
  seedImg("henu-bike", 800, 500),
  seedImg("henu-bike-lock", 800, 500),
  seedImg("henu-bike-bell", 800, 500),
];
INITIAL_ITEMS.find((i) => i.id === "s-06")!.images = [seedImg("henu-plaster-set", 800, 500)];
INITIAL_ITEMS.find((i) => i.id === "h-08")!.images = [seedImg("henu-bookshelf", 800, 500)];

let state: CampusData = INITIAL;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

function set(patch: Partial<CampusData>) {
  state = { ...state, ...patch };
  emit();
}

let seq = 100;

export const campusStore = {
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
  get: (): CampusData => state,
  getServer: (): CampusData => INITIAL,

  toggleWant(itemId: string) {
    set({
      items: state.items.map((it) =>
        it.id === itemId
          ? { ...it, wanted: !it.wanted, wants: it.wants + (it.wanted ? -1 : 1) }
          : it
      ),
    });
  },

  addMessage(itemId: string, author: string, text: string) {
    set({
      messages: [...state.messages, { id: `m-new-${seq++}`, itemId, author, time: "刚刚", text }],
    });
  },

  publish(input: Omit<Item, "id" | "status" | "wants" | "time" | "dealsDone" | "credit">) {
    const it: Item = {
      ...input,
      id: `i-new-${seq++}`,
      status: "open",
      wants: 0,
      time: "刚刚",
      credit: 91,
      dealsDone: 3,
      isMine: true,
    };
    set({ items: [it, ...state.items] });
    return it.id;
  },

  updateItem(id: string, patch: Partial<Item>) {
    set({ items: state.items.map((it) => (it.id === id ? { ...it, ...patch } : it)) });
  },

  /** 接单 / 我要买：生成订单，单子转进行中 */
  accept(itemId: string) {
    const it = state.items.find((i) => i.id === itemId);
    if (!it || it.status !== "open") return;
    const deal: Deal = {
      id: `D-${260700 + seq++}`,
      itemId,
      title: it.title,
      price: it.price,
      other: it.seller,
      role: it.type === "help" ? "taker" : "buyer",
      status: "ongoing",
      timeline: [
        { label: "发布", time: it.time },
        { label: "赏金托管", time: "刚刚" },
        { label: it.type === "help" ? "接单服务" : "下单购买", time: "刚刚" },
      ],
    };
    set({
      items: state.items.map((i) => (i.id === itemId ? { ...i, status: "ongoing" } : i)),
      deals: [deal, ...state.deals],
    });
  },

  /** 确认完成 → 平台结算：单子与订单转已完成 */
  confirmDone(itemId: string) {
    set({
      items: state.items.map((i) => (i.id === itemId ? { ...i, status: "done" } : i)),
      deals: state.deals.map((d) =>
        d.itemId === itemId
          ? {
              ...d,
              status: "done",
              timeline: [
                ...d.timeline,
                { label: "确认完成", time: "刚刚" },
                { label: "平台结算", time: "刚刚" },
              ],
            }
          : d
      ),
    });
  },

  /** 取消进行中的订单：单子回到待接单，订单移除 */
  cancelDeal(itemId: string) {
    set({
      items: state.items.map((i) => (i.id === itemId ? { ...i, status: "open" } : i)),
      deals: state.deals.filter((d) => d.itemId !== itemId),
    });
  },

  toggleHidden(id: string) {
    set({
      items: state.items.map((it) =>
        it.id === id
          ? { ...it, status: it.status === "hidden" ? "open" : "hidden" }
          : it
      ),
    });
  },

  removeItem(id: string) {
    set({
      items: state.items.filter((it) => it.id !== id),
      messages: state.messages.filter((m) => m.itemId !== id),
      deals: state.deals.filter((d) => d.itemId !== id),
    });
  },
};
