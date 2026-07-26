/**
 * 认证 store（支持真实 Gateway 和 mock 两种模式）。
 *
 * - Gateway 已配置：仅使用真实 session（OAuth / cookie）
 * - NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1 或 production：禁用 mock 登录
 * - 本地 mock 仅当 NEXT_PUBLIC_PORTAL_ALLOW_MOCK=1 且未 require gateway
 */

import {
  fetchSession,
  hasGateway,
  logout as apiLogout,
  mockAllowed,
  redirectToLogin,
} from "@/lib/api/client";
import { requireGateway } from "@/lib/api/env";
import { initAllGateways } from "@/lib/gateway-init";

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

  // Always warm product gateways on client boot (independent of login).
  void initAllGateways();

  if (hasGateway || requireGateway()) {
    // 真实 Gateway 模式：只认 session；失败即未登录
    fetchSession()
      .then((session) => {
        if (session) {
          state = {
            user: { name: session.user_id, uid: session.user_id, email: "" },
            ready: true,
          };
        } else {
          state = { user: null, ready: true };
        }
        emit();
      })
      .catch(() => {
        state = { user: null, ready: true };
        emit();
      });
    return;
  }

  // Mock 模式（仅 allowMock）
  if (!mockAllowed) {
    state = { user: null, ready: true };
    emit();
    return;
  }

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

/** Mock 登录是否可用（UI 可据此隐藏演示码 / 本地登录表单） */
export function isMockAuthEnabled(): boolean {
  return mockAllowed && !hasGateway && !requireGateway();
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
    if (hasGateway || requireGateway()) {
      redirectToLogin();
      return;
    }
    if (!mockAllowed) {
      throw new Error("Mock login is disabled. Configure portal gateway.");
    }
    setUser({ name, uid: uidOf(name), email: "" });
  },

  /** 注册（mock only） */
  register(name: string, email: string) {
    if (hasGateway || requireGateway()) {
      redirectToLogin();
      return;
    }
    if (!mockAllowed) {
      throw new Error("Mock register is disabled. Configure portal gateway.");
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
