/**
 * 形变过渡的模块级外部 store。
 * 退出页（TransitionLink）写入 payload；进入页（usePageEnter）上报 landing；
 * TransitionProvider 监听后执行 overlay 形变并清空。
 * 模块单例，SSR 与客户端首次水合均为 EMPTY，不产生水合差异。
 */

export type MorphKind = "list" | "question";

export interface RectSnapshot {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface MorphPayload {
  kind: MorphKind;
  id?: string;
  title: string;
  sub?: string;
  rect: RectSnapshot;
}

export interface MorphState {
  payload: MorphPayload | null;
  landing: RectSnapshot | null;
}

const EMPTY: MorphState = { payload: null, landing: null };

let state: MorphState = EMPTY;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

export const morphStore = {
  /** 同步读取（事件/生命周期内使用） */
  peek: () => state,

  set(payload: MorphPayload) {
    state = { payload, landing: null };
    emit();
  },

  setLanding(landing: RectSnapshot) {
    if (!state.payload) return;
    state = { ...state, landing };
    emit();
  },

  clear() {
    state = EMPTY;
    emit();
  },

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },

  get: () => state,
  getServer: () => EMPTY,
};
