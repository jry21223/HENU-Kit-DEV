/**
 * 账号控制台 mock 数据（字面量，SSR/客户端一致）+ 会话内 client store。
 * 设备下线、签到、开通会员、新建工单、标记已读等操作仅当前会话内存态，
 * 通过 useSyncExternalStore 跨页面同步（如概览未读数）。
 */

// ---------------------------------------------------------------- 字面量数据

export interface Device {
  id: string;
  name: string;
  place: string;
  ip: string;
  active: string;
  current?: boolean;
}

export interface Txn {
  id: string;
  time: string;
  item: string;
  amount: number; // 正获得 / 负消耗
}

export interface Membership {
  plan: string;
  expire: string;
  daysLeft: number;
  totalDays: number;
}

export interface TicketMsg {
  from: "我" | "客服";
  text: string;
  time: string;
}

export interface Ticket {
  id: string;
  title: string;
  type: string;
  status: "待处理" | "处理中" | "已解决";
  time: string;
  msgs: TicketMsg[];
}

export interface Notice {
  id: string;
  title: string;
  body: string;
  time: string;
  read: boolean;
}

const INITIAL_DEVICES: Device[] = [
  { id: "d1", name: "Windows · Chrome 139", place: "河南 郑州", ip: "171.12.34.56", active: "当前在线", current: true },
  { id: "d2", name: "iPhone 14 · henukit App", place: "河南 洛阳", ip: "223.88.91.20", active: "2 小时前" },
  { id: "d3", name: "Redmi K40 · Chrome", place: "河南 开封", ip: "117.159.12.8", active: "2026-07-02 21:40" },
];

const INITIAL_TXNS: Txn[] = [
  { id: "t1", time: "07-19 08:12", item: "每日签到", amount: 10 },
  { id: "t2", time: "07-18 22:31", item: "答题奖励 · 数据结构 10 连对", amount: 25 },
  { id: "t3", time: "07-18 15:02", item: "兑换 · 打印店 5 元代金券", amount: -80 },
  { id: "t4", time: "07-17 20:44", item: "每日签到", amount: 10 },
  { id: "t5", time: "07-17 16:20", item: "上传题单审核通过奖励", amount: 50 },
  { id: "t6", time: "07-16 09:05", item: "每日签到", amount: 10 },
  { id: "t7", time: "07-15 21:37", item: "答题奖励 · 高等数学A 周挑战", amount: 30 },
  { id: "t8", time: "07-14 12:10", item: "兑换 · 咖啡店第二杯半价券", amount: -100 },
  { id: "t9", time: "07-13 08:59", item: "新人任务 · 完善资料", amount: 20 },
];

const INITIAL_MEMBERSHIP: Membership = {
  plan: "月度会员",
  expire: "2026-08-06",
  daysLeft: 18,
  totalDays: 30,
};

export const MEMBERSHIP_PLANS = [
  { id: "m", name: "月度会员", price: 12, days: 30, note: "轻量体验" },
  { id: "q", name: "季度会员", price: 30, days: 90, note: "折合 ¥10/月" },
  { id: "y", name: "年度会员", price: 98, days: 365, note: "折合 ¥8.2/月", recommended: true },
];

const INITIAL_TICKETS: Ticket[] = [
  {
    id: "T-2607", title: "数据结构题单 Q-06 答案疑似有误", type: "题目纠错", status: "处理中",
    time: "07-18 19:24",
    msgs: [
      { from: "我", text: "完全二叉树深度那道题，参考答案给的是 ⌊log₂n⌋，我认为应为 ⌊log₂n⌋+1。", time: "07-18 19:24" },
      { from: "客服", text: "已转交教研组复核，预计 1 个工作日内回复。", time: "07-18 20:02" },
    ],
  },
  {
    id: "T-2606", title: "排行榜数据两天没更新", type: "功能异常", status: "待处理",
    time: "07-17 10:11",
    msgs: [{ from: "我", text: "总榜从 7 月 15 日起没有变化，麻烦看一下。", time: "07-17 10:11" }],
  },
  {
    id: "T-2604", title: "建议增加线性代数向量空间专题", type: "功能建议", status: "已解决",
    time: "07-12 16:45",
    msgs: [
      { from: "我", text: "线代第四章题库有点少，能不能加一个向量空间专题？", time: "07-12 16:45" },
      { from: "客服", text: "建议已收录，专题预计下月上线，感谢反馈。", time: "07-13 09:30" },
    ],
  },
  {
    id: "T-2601", title: "账号异地登录提醒是否误报", type: "账号问题", status: "已解决",
    time: "07-05 08:52",
    msgs: [
      { from: "我", text: "收到开封的登录提醒，但那是我的旧手机，能否标记为常用设备？", time: "07-05 08:52" },
      { from: "客服", text: "已为您将该设备加入信任列表，后续不再提醒。", time: "07-05 11:20" },
    ],
  },
];

const INITIAL_NOTICES: Notice[] = [
  { id: "n1", title: "系统维护通知", body: "7 月 21 日 02:00–04:00 平台升级维护，期间刷题记录可能延迟同步。", time: "07-19 09:00", read: false },
  { id: "n2", title: "会员将于 18 天后到期", body: "您的月度会员将在 2026-08-06 到期，续费可享连续包月优惠。", time: "07-19 08:00", read: false },
  { id: "n3", title: "您上传的题单已通过审核", body: "《高等数学A · 期中复习卷》已上架题库，奖励 50 积分已到账。", time: "07-17 16:20", read: false },
  { id: "n4", title: "暑期刷题挑战赛上线", body: "7 月 20 日起连续 21 天打卡可瓜分 100,000 积分池。", time: "07-15 12:00", read: true },
  { id: "n5", title: "账号在开封登录", body: "检测到新设备登录（Redmi K40），如非本人操作请立即修改密码。", time: "07-02 21:40", read: true },
  { id: "n6", title: "v0.9 版本更新说明", body: "新增数据面板雷达图与 26 周刷题热力图，修复若干已知问题。", time: "06-28 10:00", read: true },
];

// ---------------------------------------------------------------- 会话内 store

export interface AccountData {
  balance: number;
  signedToday: boolean;
  txns: Txn[];
  devices: Device[];
  membership: Membership;
  tickets: Ticket[];
  notices: Notice[];
}

const INITIAL: AccountData = {
  balance: 345,
  signedToday: false,
  txns: INITIAL_TXNS,
  devices: INITIAL_DEVICES,
  membership: INITIAL_MEMBERSHIP,
  tickets: INITIAL_TICKETS,
  notices: INITIAL_NOTICES,
};

let state: AccountData = INITIAL;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

function set(patch: Partial<AccountData>) {
  state = { ...state, ...patch };
  emit();
}

export const accountStore = {
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
  get: (): AccountData => state,
  getServer: (): AccountData => INITIAL,

  signIn() {
    if (state.signedToday) return;
    set({
      balance: state.balance + 10,
      signedToday: true,
      txns: [{ id: `t-sign-${state.txns.length}`, time: "刚刚", item: "每日签到", amount: 10 }, ...state.txns],
    });
  },

  /** 积分消费（资料购买等）：余额不足返回 false，成功追加流水 */
  spendPoints(amount: number, item: string): boolean {
    if (state.balance < amount) return false;
    set({
      balance: state.balance - amount,
      txns: [{ id: `t-spend-${state.txns.length}`, time: "刚刚", item, amount: -amount }, ...state.txns],
    });
    return true;
  },

  removeDevice(id: string) {
    set({ devices: state.devices.filter((d) => d.id !== id) });
  },

  openMembership(plan: string, days: number, expire: string) {
    set({ membership: { plan, expire, daysLeft: days, totalDays: days } });
  },

  addTicket(title: string, type: string, desc: string) {
    const seq = state.tickets.length + 1;
    const ticket: Ticket = {
      id: `T-${2607 + seq}`,
      title,
      type,
      status: "待处理",
      time: "刚刚",
      msgs: [{ from: "我", text: desc, time: "刚刚" }],
    };
    set({ tickets: [ticket, ...state.tickets] });
  },

  markAllNoticesRead() {
    set({ notices: state.notices.map((n) => ({ ...n, read: true })) });
  },

  markNoticeRead(id: string) {
    set({ notices: state.notices.map((n) => (n.id === id ? { ...n, read: true } : n)) });
  },
};

export const unreadNotices = (d: AccountData) => d.notices.filter((n) => !n.read).length;
export const openTickets = (d: AccountData) => d.tickets.filter((t) => t.status !== "已解决").length;

// ---------------------------------------------------------------- 邮箱验证码（mock）

/** 演示验证码（v1 不发送真实邮件，页面直接提示） */
export const EMAIL_DEMO_CODE = "427819";

/** 发送邮箱验证码（mock）：仅做格式校验，返回是否已"发送" */
export function sendEmailCode(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}
