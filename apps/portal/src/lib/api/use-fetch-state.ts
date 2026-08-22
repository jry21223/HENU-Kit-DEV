"use client";

import { useEffect, useState, type DependencyList, type Dispatch, type SetStateAction } from "react";
import { formatPortalError, PortalUnauthorizedError } from "@/lib/api/client";
import { useDeferredFetch } from "@/lib/api/use-deferred-fetch";

/**
 * Discriminated fetch state shared by the signed-in favorites surfaces.
 * `ready` carries the typed payload; a PortalUnauthorizedError settles as
 * `anonymous` instead of an error the user must fix.
 */
export type FetchState<T> =
  | { status: "loading" }
  | { status: "anonymous" }
  | { status: "error"; message: string }
  | { status: "ready"; data: T };

/**
 * useDeferredFetch plus the loading/anonymous/error/ready mapping used by the
 * favorites list and favorites overview pages.
 */
export function useFetchState<T>(
  fetcher: () => Promise<{ data: T }> | undefined,
  deps: DependencyList
): {
  state: FetchState<T>;
  setState: Dispatch<SetStateAction<FetchState<T>>>;
  retry: () => void;
} {
  const { data, error, retry } = useDeferredFetch(fetcher, deps);
  const [state, setState] = useState<FetchState<T>>({ status: "loading" });
  useEffect(() => {
    let cancelled = false;
    void Promise.resolve().then(() => {
      if (cancelled) return;
      if (error !== undefined) {
        if (error instanceof PortalUnauthorizedError) {
          setState({ status: "anonymous" });
          return;
        }
        setState({ status: "error", message: formatPortalError(error) });
        return;
      }
      if (data) {
        setState({ status: "ready", data: data.data });
        return;
      }
      setState({ status: "loading" });
    });
    return () => {
      cancelled = true;
    };
  }, [data, error]);
  return { state, setState, retry };
}
