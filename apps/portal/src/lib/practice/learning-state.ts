"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchLearningState,
  formatPortalError,
  PortalUnauthorizedError,
} from "../api/client";
import { quizCraftV2ReadsEnabled } from "../api/env";
import type { LearningStateItem, LearningStatePagination } from "../api/types";

export type LearningStateView =
  | { status: "disabled" }
  | { status: "loading" }
  | { status: "unauthenticated" }
  | { status: "error"; message: string }
  | { status: "empty"; pagination: LearningStatePagination }
  | { status: "ready"; items: LearningStateItem[]; pagination: LearningStatePagination };

export function useLearningState(): {
  state: LearningStateView;
  reload: () => void;
  previousPage: () => void;
  nextPage: () => void;
} {
  const enabled = quizCraftV2ReadsEnabled();
  const [attempt, setAttempt] = useState(0);
  const [page, setPage] = useState(1);
  const [state, setState] = useState<LearningStateView>({ status: "loading" });

  useEffect(() => {
    let active = true;
    if (!enabled) return () => {
      active = false;
    };
    void fetchLearningState(page, 20, true)
      .then((response) => {
        if (!active) return;
        const { items, pagination } = response.data;
        if (pagination.total > 0 && pagination.total_pages > 0 && page > pagination.total_pages) {
          setPage(pagination.total_pages);
          return;
        }
        setState(items.length === 0 ? { status: "empty", pagination } : { status: "ready", items, pagination });
      })
      .catch((error: unknown) => {
        if (!active) return;
        if (error instanceof PortalUnauthorizedError) {
          setState({ status: "unauthenticated" });
          return;
        }
        setState({ status: "error", message: formatPortalError(error) });
      });
    return () => {
      active = false;
    };
  }, [attempt, enabled, page]);

  const reload = useCallback(() => {
    if (!enabled) return;
    setState({ status: "loading" });
    setAttempt((current) => current + 1);
  }, [enabled]);

  const previousPage = useCallback(() => {
    if (!enabled || (state.status !== "ready" && state.status !== "empty") || state.pagination.page <= 1) return;
    setState({ status: "loading" });
    setPage((current) => current - 1);
  }, [enabled, state]);

  const nextPage = useCallback(() => {
    if (!enabled || (state.status !== "ready" && state.status !== "empty") || state.pagination.page >= state.pagination.total_pages) return;
    setState({ status: "loading" });
    setPage((current) => current + 1);
  }, [enabled, state]);

  return { state: enabled ? state : { status: "disabled" }, reload, previousPage, nextPage };
}
