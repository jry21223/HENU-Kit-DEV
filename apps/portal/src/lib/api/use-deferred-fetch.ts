"use client";

import { useCallback, useEffect, useState, type DependencyList } from "react";

/**
 * Runs `fetcher` once per dependency change, deferred to a microtask so the
 * caller's pending state is published before any response can arrive (the
 * effect body itself never calls setState synchronously). A fetcher that
 * returns `undefined` skips the attempt. Returns the latest data/error plus a
 * `retry` that re-runs the fetch with the same fetcher.
 */
export function useDeferredFetch<T>(
  fetcher: () => Promise<T> | undefined,
  deps: DependencyList
): { data: T | undefined; error: unknown; retry: () => void } {
  const [attempt, setAttempt] = useState(0);
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<unknown>(undefined);

  useEffect(() => {
    let cancelled = false;
    void Promise.resolve()
      .then(() => {
        if (cancelled) return;
        setData(undefined);
        setError(undefined);
      })
      .then(() => fetcher())
      .then(
        (value) => {
          if (cancelled || value === undefined) return;
          setData(value);
        },
        (err: unknown) => {
          if (cancelled) return;
          setError(err);
        }
      );
    return () => {
      cancelled = true;
    };
    // The caller owns the dependency list, same contract as useEffect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, attempt]);

  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  return { data, error, retry };
}
