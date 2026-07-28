"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchPersonalPracticeStats,
  formatPortalError,
  PortalUnauthorizedError,
} from "@/lib/api/client";
import { quizCraftV2ReadsEnabled } from "@/lib/api/env";
import type { PersonalPracticeStats } from "@/lib/api/types";
import {
  EMPTY_MASTERY,
  type MasterySnapshot,
} from "@/components/practice/bank-hero-mastery";

export type PersonalPracticeStatsState =
  | { status: "disabled" }
  | { status: "loading" }
  | { status: "unauthenticated" }
  | { status: "error"; message: string }
  | { status: "empty"; data: PersonalPracticeStats }
  | { status: "ready"; data: PersonalPracticeStats };

export function isEmptyPersonalPracticeStats(data: PersonalPracticeStats): boolean {
  return data.total_answers === 0;
}

export function toMasterySnapshot(
  data: PersonalPracticeStats | undefined
): MasterySnapshot {
  if (!data) return EMPTY_MASTERY;
  return {
    subjects: data.mastery.map((subject) => ({
      label: subject.label,
      value: subject.value,
    })),
    accuracy: data.accuracy,
    streakDays: data.streak_days,
    totalQuestions: data.total_answers,
  };
}

/**
 * Reads only the explicitly cut-over QuizCraft V2 endpoint. A failed request
 * remains loading/error/unauthenticated in the UI; it can never become a mock
 * success state.
 */
export function usePersonalPracticeStats(): {
  state: PersonalPracticeStatsState;
  retry: () => void;
} {
  const enabled = quizCraftV2ReadsEnabled();
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<PersonalPracticeStatsState>({
    status: "loading",
  });

  useEffect(() => {
    let active = true;
    if (!enabled) return () => {
      active = false;
    };
    void fetchPersonalPracticeStats()
      .then((response) => {
        if (!active) return;
        const next = response.data;
        setState(
          isEmptyPersonalPracticeStats(next)
            ? { status: "empty", data: next }
            : { status: "ready", data: next }
        );
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
  }, [attempt, enabled]);

  const retry = useCallback(() => {
    if (enabled) {
      setState({ status: "loading" });
      setAttempt((current) => current + 1);
    }
  }, [enabled]);

  return { state: enabled ? state : { status: "disabled" }, retry };
}
