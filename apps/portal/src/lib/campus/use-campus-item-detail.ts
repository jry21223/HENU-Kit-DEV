"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchCampusItemDetail,
  formatPortalError,
  PortalApiError,
  PortalHttpError,
} from "@/lib/api/client";
import { getCampusItemOrFallback } from "@/lib/campus/gateway";
import type { CampusItem, CampusMessage } from "@/lib/api/types";

/**
 * 单子详情加载状态（判别联合，组件用单一守卫 `loadState !== "ready"`）。
 * item 与 messages 聚合为单一 detail 状态，避免成对赋值。
 * 与资料详情一致：单子不存在（not-found）与暂时读不到（error，可重试）分开。
 */
export type CampusItemDetailState =
  | { loadState: "loading"; item: null; messages: CampusMessage[]; error: null }
  | { loadState: "ready"; item: CampusItem; messages: CampusMessage[]; error: null }
  | { loadState: "not-found"; item: null; messages: CampusMessage[]; error: null }
  | { loadState: "error"; item: null; messages: CampusMessage[]; error: string; retry: () => void };

/**
 * 列表缓存也回退不到这条单子时，这次失败是否说明单子不存在：接口回 404，或者空响应
 * （本地演示模式没有接口，演示数据里也没有这条，重试不会有结果）。服务不可用、回来的不是
 * JSON、断网都只是暂时读不到。
 */
export function campusItemIsMissing(loadError: unknown): boolean {
  if (loadError instanceof PortalHttpError) return loadError.status === 404;
  return loadError instanceof PortalApiError && loadError.code === "PORTAL_EMPTY_RESPONSE";
}

/**
 * 单子详情统一加载：真实 detail 接口 → gateway 缓存/mock 回退 → 404 或空响应为
 * not-found，其余失败为 error。回退决策复用 gateway.getCampusItemOrFallback，与列表页
 * 同一 gateway-first 语义。
 */
export function useCampusItemDetail(id: string): CampusItemDetailState {
  const [state, setState] = useState<CampusItemDetailState>({
    loadState: "loading",
    item: null,
    messages: [],
    error: null,
  });
  const mounted = useRef(true);
  const [attempt, setAttempt] = useState(0);
  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  const load = useCallback(async () => {
    setState({ loadState: "loading", item: null, messages: [], error: null });
    const fallback = getCampusItemOrFallback(id);
    try {
      const response = await fetchCampusItemDetail(id);
      if (!mounted.current) return;
      if (response?.item) {
        setState({
          loadState: "ready",
          item: response.item,
          messages: response.messages ?? [],
          error: null,
        });
        return;
      }
      // 内容不存在：404 文案由展示组件提供，error 保持 null，避免文案叠加。
      setState({ loadState: "not-found", item: null, messages: [], error: null });
    } catch (loadError) {
      if (!mounted.current) return;
      if (fallback.item) {
        setState({ loadState: "ready", item: fallback.item, messages: fallback.messages, error: null });
        return;
      }
      if (campusItemIsMissing(loadError)) {
        setState({ loadState: "not-found", item: null, messages: [], error: null });
        return;
      }
      setState({ loadState: "error", item: null, messages: [], error: formatPortalError(loadError), retry });
    }
  }, [id, retry]);

  useEffect(() => {
    mounted.current = true;
    const timer = window.setTimeout(() => void load(), 0);
    return () => {
      mounted.current = false;
      window.clearTimeout(timer);
    };
  }, [attempt, load]);

  return state;
}
