"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchCampusItemDetail, formatPortalError } from "@/lib/api/client";
import { getCampusItemOrFallback } from "@/lib/campus/gateway";
import type { CampusItem, CampusMessage } from "@/lib/api/types";

/**
 * 单子详情加载三态（判别联合，组件用单一守卫 `loadState !== "ready"`）。
 * item 与 messages 聚合为单一 detail 状态，避免成对赋值。
 */
export type CampusItemDetailState =
  | { loadState: "loading"; item: null; messages: CampusMessage[]; error: null }
  | { loadState: "ready"; item: CampusItem; messages: CampusMessage[]; error: null }
  | { loadState: "error"; item: null; messages: CampusMessage[]; error: string | null };

/**
 * 单子详情统一加载：真实 detail 接口 → gateway 缓存/mock 回退 → error。
 * 回退决策复用 gateway.getCampusItemOrFallback，与列表页同一 gateway-first 语义。
 */
export function useCampusItemDetail(id: string): CampusItemDetailState {
  const [state, setState] = useState<CampusItemDetailState>({
    loadState: "loading",
    item: null,
    messages: [],
    error: null,
  });
  const mounted = useRef(true);

  const load = useCallback(async () => {
    setState({ loadState: "loading", item: null, messages: [], error: null });
    const fallback = getCampusItemOrFallback(id);
    try {
      const response = await fetchCampusItemDetail(id);
      if (!mounted.current) return;
      setState({
        loadState: "ready",
        item: response.item,
        messages: response.messages ?? [],
        error: null,
      });
    } catch (loadError) {
      if (!mounted.current) return;
      if (fallback.item) {
        setState({ loadState: "ready", item: fallback.item, messages: fallback.messages, error: null });
        return;
      }
      setState({ loadState: "error", item: null, messages: [], error: formatPortalError(loadError) });
    }
  }, [id]);

  useEffect(() => {
    mounted.current = true;
    const timer = window.setTimeout(() => void load(), 0);
    return () => {
      mounted.current = false;
      window.clearTimeout(timer);
    };
  }, [load]);

  return state;
}
