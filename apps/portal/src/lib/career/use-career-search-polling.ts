"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { requestCareerSearchStatus } from "@/lib/career/gateway";
import {
  createCareerScanRunner,
} from "@/lib/career/career-scan-state";
import type {
  CareerScanPollState,
  CareerScanRunner,
} from "@/lib/career/career-scan-state";
import type { CareerSearch } from "@/lib/api/types";

export interface CareerSearchPolling {
  /** 最近一次状态；null 表示尚无任务。 */
  state: CareerScanPollState | null;
  /** 是否处于轮询中（active 即 true；completed/failed 后自动为 false）。 */
  isPolling: boolean;
  /** 启动（或重置）对一次搜索的轮询；重复调用不会叠加循环。 */
  start: (search: CareerSearch) => void;
  /** 停止轮询；组件卸载清理用。 */
  stop: () => void;
}

/**
 * 异步扫描状态跟踪（#402）：把纯状态机 runner 适配进 React。
 *
 * runner 单例挂在 ref 上，卸载时 stop() 清理定时器；
 * 轮询在 queued/running 间持续，completed/failed 自动停止。
 */
export function useCareerSearchPolling(): CareerSearchPolling {
  const [state, setState] = useState<CareerScanPollState | null>(null);
  const [isPolling, setIsPolling] = useState(false);
  const runnerRef = useRef<CareerScanRunner | null>(null);

  if (runnerRef.current === null) {
    runnerRef.current = createCareerScanRunner({
      fetchStatus: async (searchID) => {
        const response = await requestCareerSearchStatus(searchID);
        return response.search;
      },
      onState: (next) => {
        setState(next);
        setIsPolling(next.kind === "active");
      },
    });
  }

  const start = useCallback((search: CareerSearch) => {
    runnerRef.current?.start(search);
  }, []);

  const stop = useCallback(() => {
    runnerRef.current?.stop();
  }, []);

  useEffect(() => {
    const runner = runnerRef.current;
    return () => runner?.stop();
  }, []);

  return { state, isPolling, start, stop };
}
