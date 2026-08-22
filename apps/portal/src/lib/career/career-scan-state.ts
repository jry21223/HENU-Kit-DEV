import type { CareerSearch, CareerSearchStage } from "../api/types";

/**
 * /career 异步扫描状态机（#402）。
 *
 * 纯逻辑、与 React 无关：runner 用 setTimeout 驱动轮询，单条循环、
 * 终态即停、stop() 幂等。组件层（useCareerSearchPolling）只做适配，
 * 所有分支行为都在这里被单元测试覆盖。
 */

/** 轮询间隔：3–5 秒区间取值，避免放大后端请求。 */
export const CAREER_POLL_INTERVAL_MS = 4000;

export type CareerScanPollState =
  | { kind: "active"; search: CareerSearch; pollError: string | null }
  | { kind: "completed"; search: CareerSearch }
  | { kind: "failed"; search: CareerSearch };

export type CareerScanStatusFetcher = (searchID: string) => Promise<CareerSearch>;

/** queued / running 视为进行中，轮询继续。 */
export function isCareerScanActive(search: CareerSearch): boolean {
  return search.status === "queued" || search.status === "running";
}

/** completed / failed 为终态，轮询停止。 */
export function isCareerScanTerminal(search: CareerSearch): boolean {
  return search.status === "completed" || search.status === "failed";
}

export function needsCareerDigestFollowup(search: CareerSearch): boolean {
  return (
    search.status === "completed" &&
    (search.digest_status === "sending" || search.digest_status === "retry")
  );
}

const STATUS_LABELS: Record<CareerSearch["status"], string> = {
  queued: "排队中",
  running: "扫描中",
  completed: "已完成",
  failed: "失败",
};

export function careerSearchStatusLabel(status: CareerSearch["status"]): string {
  return STATUS_LABELS[status];
}

/** Browser copy for the durable digest-enqueue ledger. "sent" means
 * Platform Core accepted the mail into its queue; it never claims inbox
 * delivery. */
export function careerDigestStatusLabel(search: CareerSearch): string | null {
  switch (search.digest_status) {
    case "sent":
      return "邮件简报已进入发送队列";
    case "retry":
      return "邮件简报发送暂时失败，系统正在重试";
    case "sending":
      return "正在将邮件简报加入发送队列";
    case "skipped":
      return "未启用邮件简报";
    default:
      if (search.has_email) return "邮件简报已进入发送队列";
      return search.status === "completed" ? "历史任务无邮件状态" : null;
  }
}

const STAGE_LABELS: Partial<Record<CareerSearchStage, string>> = {
  crawling: "正在抓取岗位来源",
  matching: "正在匹配岗位机会",
  rendering: "正在生成结果",
};

/**
 * stage 中文文案；未知或缺失 stage 返回 null，由展示层给兜底句。
 * 参数放宽到 string：后端空串（""）按缺失处理。
 */
export function careerScanStageLabel(stage: CareerSearchStage | string | null | undefined): string | null {
  if (!stage) return null;
  return STAGE_LABELS[stage as CareerSearchStage] ?? null;
}

/**
 * failed 的稳定中文文案。后端的 error_message 只保证浏览器安全
 * （无堆栈/凭据），不保证可读，因此前端统一给出可理解话术，
 * 不把内部细节透出给用户。
 */
export function careerScanFailedMessage(): string {
  return "扫描未能完成，请稍后重试";
}

/** ISO 时间 → 「MM-DD HH:mm」本地展示；非法输入原样返回。 */
export function formatCareerSearchTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export interface CareerScanRunner {
  /** 启动（或重置）一条轮询循环；幂等，绝不叠加第二条循环。 */
  start(search: CareerSearch): void;
  /** 停止轮询；可重复调用，卸载清理用。 */
  stop(): void;
  getState(): CareerScanPollState | null;
}

export interface CareerScanRunnerOptions {
  fetchStatus: CareerScanStatusFetcher;
  onState: (state: CareerScanPollState) => void;
  intervalMs?: number;
}

export function createCareerScanRunner(
  options: CareerScanRunnerOptions
): CareerScanRunner {
  const intervalMs = options.intervalMs ?? CAREER_POLL_INTERVAL_MS;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let stopped = true;
  let generation = 0;
  let lastSearch: CareerSearch | null = null;
  let state: CareerScanPollState | null = null;

  function clearTimer() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function emit(next: CareerScanPollState) {
    state = next;
    options.onState(next);
  }

  function scheduleNext(expectedGeneration = generation) {
    if (stopped || expectedGeneration !== generation) return;
    timer = setTimeout(() => {
      timer = null;
      void tick(expectedGeneration);
    }, intervalMs);
  }

  async function tick(expectedGeneration: number) {
    if (stopped || expectedGeneration !== generation || !lastSearch) return;
    const searchID = lastSearch.id;
    let search: CareerSearch;
    try {
      search = await options.fetchStatus(searchID);
    } catch {
      if (stopped || expectedGeneration !== generation) return;
      // 瞬态失败：保留最近状态继续轮询，并透出一条可读提示。
      if (!stopped) {
        if (lastSearch.status !== "completed") {
          emit({ kind: "active", search: lastSearch, pollError: "状态读取失败，正在自动重试" });
        }
      }
      scheduleNext(expectedGeneration);
      return;
    }
    if (stopped || expectedGeneration !== generation) return;
    lastSearch = search;
    if (search.status === "completed") {
      emit({ kind: "completed", search });
      if (needsCareerDigestFollowup(search)) {
        scheduleNext(expectedGeneration);
      } else {
        stop();
      }
      return;
    }
    if (search.status === "failed") {
      stop();
      emit({ kind: "failed", search });
      return;
    }
    emit({ kind: "active", search, pollError: null });
    scheduleNext(expectedGeneration);
  }

  function stop() {
    stopped = true;
    generation += 1;
    clearTimer();
  }

  return {
    start(search) {
      // 无论是否已在轮询都先复位：保证同时只有一条循环。
      clearTimer();
      generation += 1;
      stopped = false;
      lastSearch = search;
      if (search.status === "completed") {
        emit({ kind: "completed", search });
        if (needsCareerDigestFollowup(search)) {
          scheduleNext(generation);
        } else {
          stop();
        }
        return;
      }
      if (search.status === "failed") {
        stop();
        emit({ kind: "failed", search });
        return;
      }
      emit({ kind: "active", search, pollError: null });
      scheduleNext(generation);
    },
    stop,
    getState() {
      return state;
    },
  };
}
