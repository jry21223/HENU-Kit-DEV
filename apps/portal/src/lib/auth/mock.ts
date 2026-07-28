/**
 * 账号控制台本地 store。
 *
 * 生产默认 **空诚实态**：免费会员、无通知/工单/设备列表。
 * 不再预置假会员天数、示例通知等演示数据。
 * 积分事实只由 Account Portfolio 的持久化账本提供；这里不保留签到
 * 加分、积分消费或本地流水状态。
 */

// ---------------------------------------------------------------- 类型

export interface Device {
  id: string;
  name: string;
  place: string;
  ip: string;
  active: string;
  current?: boolean;
}

export interface Membership {
  /** 展示名：免费 | 终身会员 */
  plan: string;
  /** 免费为 "—"；终身为 "永久" */
  expire: string;
  daysLeft: number;
  totalDays: number;
  lifetime: boolean;
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

export interface MembershipPlan {
  id: string;
  name: string;
  price: number;
  note: string;
  lifetime: boolean;
  recommended?: boolean;
}

/** 可购套餐仅一档：¥9.9 终身。免费是默认状态，不是 SKU。 */
export const MEMBERSHIP_PLANS: MembershipPlan[] = [
  {
    id: "life",
    name: "终身会员",
    price: 9.9,
    note: "一次开通 · 长期有效",
    lifetime: true,
    recommended: true,
  },
];

export const FREE_MEMBERSHIP: Membership = {
  plan: "免费",
  expire: "—",
  daysLeft: 0,
  totalDays: 0,
  lifetime: false,
};

// ---------------------------------------------------------------- 会话内 store（空初始）

export interface AccountData {
  devices: Device[];
  membership: Membership;
  tickets: Ticket[];
  notices: Notice[];
}

const EMPTY: AccountData = {
  devices: [],
  membership: FREE_MEMBERSHIP,
  tickets: [],
  notices: [],
};

let state: AccountData = EMPTY;
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
  /** SSR 与首屏一致：空态，避免 hydration 闪出假数据 */
  getServer: (): AccountData => EMPTY,

  removeDevice(id: string) {
    set({ devices: state.devices.filter((d) => d.id !== id) });
  },

  /** 开通终身会员（会话内预览；未接真实支付） */
  openLifetimeMembership() {
    set({
      membership: {
        plan: "终身会员",
        expire: "永久",
        daysLeft: 0,
        totalDays: 0,
        lifetime: true,
      },
    });
  },

  /** @deprecated 兼容旧调用；终身请用 openLifetimeMembership */
  openMembership(plan: string, _days: number, expire: string) {
    if (plan.includes("终身") || expire === "永久") {
      accountStore.openLifetimeMembership();
      return;
    }
    set({
      membership: {
        plan,
        expire,
        daysLeft: 0,
        totalDays: 0,
        lifetime: false,
      },
    });
  },

  addTicket(title: string, type: string, desc: string) {
    const seq = state.tickets.length + 1;
    const ticket: Ticket = {
      id: `T-${2600 + seq}`,
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
    set({
      notices: state.notices.map((n) =>
        n.id === id ? { ...n, read: true } : n
      ),
    });
  },
};

export const unreadNotices = (d: AccountData) =>
  d.notices.filter((n) => !n.read).length;
export const openTickets = (d: AccountData) =>
  d.tickets.filter((t) => t.status !== "已解决").length;

// ---------------------------------------------------------------- 邮箱验证码（仅显式 mock 认证模式）

/** 演示验证码：仅 isMockAuthEnabled 时展示 */
export const EMAIL_DEMO_CODE = "427819";

/** 发送邮箱验证码（本地 mock）：仅做格式校验 */
export function sendEmailCode(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}
