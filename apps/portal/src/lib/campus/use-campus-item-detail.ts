"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchCampusItemDetail,
  formatPortalError,
  PortalApiError,
  portalErrorRequestId,
  PortalHttpError,
} from "@/lib/api/client";
import { forgetCampusItem, getCampusItemOrFallback } from "@/lib/campus/gateway";
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
  | {
      loadState: "error";
      item: null;
      messages: CampusMessage[];
      error: string;
      requestId: string | null;
      retry: () => void;
    };

const NOT_FOUND = { loadState: "not-found", item: null, messages: [], error: null } satisfies CampusItemDetailState;

function isNotFound(error: unknown): boolean {
  return error instanceof PortalHttpError && error.status === 404;
}

export function resolveCampusItemDetailFailure(
  loadError: unknown,
  fallback: ReturnType<typeof getCampusItemOrFallback>,
  retry: () => void,
): CampusItemDetailState {
  if (isNotFound(loadError)) return NOT_FOUND;
  if (fallback.item && fallback.item.status !== "hidden") {
    return { loadState: "ready", item: fallback.item, messages: fallback.messages, error: null };
  }
  if (loadError instanceof PortalApiError && loadError.code === "PORTAL_EMPTY_RESPONSE") {
    return NOT_FOUND;
  }
  return {
    loadState: "error",
    item: null,
    messages: [],
    error: formatPortalError(loadError),
    requestId: portalErrorRequestId(loadError),
    retry,
  };
}

export async function loadCampusItemDetail(id: string, retry: () => void): Promise<CampusItemDetailState> {
  try {
    const response = await fetchCampusItemDetail(id);
    if (!response?.item || response.item.status === "hidden") {
      forgetCampusItem(id);
      return NOT_FOUND;
    }
    return { loadState: "ready", item: response.item, messages: response.messages ?? [], error: null };
  } catch (loadError) {
    if (isNotFound(loadError)) {
      forgetCampusItem(id);
      return NOT_FOUND;
    }
    return resolveCampusItemDetailFailure(loadError, getCampusItemOrFallback(id), retry);
  }
}

export function useCampusItemDetail(id: string): CampusItemDetailState {
  const [state, setState] = useState<CampusItemDetailState>({
    loadState: "loading",
    item: null,
    messages: [],
    error: null,
  });
  const [attempt, setAttempt] = useState(0);
  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  useEffect(() => {
    let active = true;
    const timer = window.setTimeout(async () => {
      setState({ loadState: "loading", item: null, messages: [], error: null });
      const detail = await loadCampusItemDetail(id, retry);
      if (active) setState(detail);
    }, 0);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [attempt, id, retry]);

  return state;
}
