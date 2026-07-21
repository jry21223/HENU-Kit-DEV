/**
 * 认证 store（支持真实 Gateway 和 mock 两种模式）。
 *
 * 当 NEXT_PUBLIC_PORTAL_GATEWAY_URL 环境变量存在时，
 * 使用 Portal Gateway 的 OAuth 认证流程；
 * 否则回退到 mock 模式（任意用户名 + 6 位密码）。
 *
 * 模块级单例 + useSyncExternalStore：server snapshot 恒为未登录常量，
 * 首次订阅时从 localStorage 恢复会话或从 Gateway 获取 session，
 * SSR/水合无差异。所有写操作只发生在事件回调里。
 */

import { hasGateway, fetchSession, redirectToLogin, logout as apiLogout } from "@/lib/api/client";

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

  if (hasGateway) {
    // 真实 Gateway 模式：从 Gateway 获取 session
    fetchSession().then((session) => {
      if (session) {
        state = {
          user: { name: session.user_id, uid: session.user_id, email: "" },
          ready: true,
        };
      } else {
        state = { user: null, ready: true };
      }
      emit();
    });
    return;
  }

  // Mock 模式：从 localStorage 恢复
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

  /** 登录 */
  login(name: string) {
    if (hasGateway) {
      // 真实模式：重定向到 Gateway OAuth
      redirectToLogin();
      return;
    }
    // Mock 模式
    setUser({ name, uid: uidOf(name), email: "" });
  },

  /** 注册（mock） */
  register(name: string, email: string) {
    if (hasGateway) {
      redirectToLogin();
      return;
    }
    setUser({ name, uid: uidOf(name), email });
  },

  /** 登出 */
  async logout() {
    if (hasGateway) {
      await apiLogout();
    }
    setUser(null);
  },
};
