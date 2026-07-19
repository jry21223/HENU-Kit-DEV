/**
 * mock 认证 store（v1 纯前端）。
 * 模块级单例 + useSyncExternalStore：server snapshot 恒为未登录常量，
 * 首次订阅时从 localStorage 恢复会话，SSR/水合无差异。
 * 所有写操作（含 localStorage）只发生在事件回调里。
 */

export interface AuthUser {
  name: string;
  uid: string;
  email: string;
}

export interface AuthState {
  user: AuthUser | null;
  /** 客户端会话是否已恢复（SSR 恒 false） */
  ready: boolean;
}

const STORAGE_KEY = "henukit.session";
const SERVER_STATE: AuthState = { user: null, ready: false };

let state: AuthState = SERVER_STATE;
let initialized = false;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

function init() {
  if (initialized || typeof window === "undefined") return;
  initialized = true;
  let user: AuthUser | null = null;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as AuthUser;
      if (parsed && typeof parsed.name === "string") user = parsed;
    }
  } catch {
    user = null;
  }
  state = { user, ready: true };
  emit();
}

function setUser(user: AuthUser | null) {
  state = { user, ready: true };
  try {
    if (user) localStorage.setItem(STORAGE_KEY, JSON.stringify(user));
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* 隐私模式等场景忽略持久化失败 */
  }
  emit();
}

/** 由用户名派生确定 UID（mock） */
function uidOf(name: string) {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) % 90000;
  return String(20260000 + h);
}

export const authStore = {
  subscribe(listener: () => void) {
    listeners.add(listener);
    init();
    return () => {
      listeners.delete(listener);
    };
  },
  get: (): AuthState => state,
  getServer: (): AuthState => SERVER_STATE,

  /** 登录（mock）：任意账号 + 密码 ≥6 即可 */
  login(name: string) {
    setUser({ name, uid: uidOf(name), email: "" });
  },
  /** 注册（mock） */
  register(name: string, email: string) {
    setUser({ name, uid: uidOf(name), email });
  },
  logout() {
    setUser(null);
  },
};
