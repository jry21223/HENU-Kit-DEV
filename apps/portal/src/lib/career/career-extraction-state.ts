import type { CareerResumeExtraction, CareerExtractionStatus } from "../api/types";
import {
  PortalApiError,
  PortalHttpError,
  PortalNetworkError,
} from "../api/client";

/**
 * 简历上传 → AI 识别的异步状态机（纯逻辑，与 React 无关）。
 *
 * runner 用 setTimeout 驱动轮询，单条循环、终态即停、stop() 幂等；
 * 组件层只做适配，所有分支行为都在这里被单元测试覆盖。
 */

/** 轮询间隔：2 秒，识别通常几秒到几十秒，不用等太久。 */
export const EXTRACTION_POLL_INTERVAL_MS = 2000;

export type ExtractionPollState =
  | { kind: "active"; extraction: CareerResumeExtraction; pollError: string | null }
  | { kind: "completed"; extraction: CareerResumeExtraction }
  | { kind: "failed"; extraction: CareerResumeExtraction };

export type ExtractionStatusFetcher = (extractionID: string) => Promise<CareerResumeExtraction>;

/** queued / running 视为进行中，轮询继续。 */
export function isExtractionActive(extraction: CareerResumeExtraction): boolean {
  return extraction.status === "queued" || extraction.status === "running";
}

/** completed / failed 为终态，轮询停止。 */
export function isExtractionTerminal(extraction: CareerResumeExtraction): boolean {
  return extraction.status === "completed" || extraction.status === "failed";
}

const STATUS_LABELS: Record<CareerExtractionStatus, string> = {
  queued: "排队中",
  running: "识别中",
  completed: "已完成",
  failed: "失败",
};

export function extractionStatusLabel(status: CareerExtractionStatus): string {
  return STATUS_LABELS[status];
}

/**
 * failed 的稳定中文文案。error_code 是后端契约码，映射到可读话术；
 * 未识别码一律给兜底句，绝不把内部细节透出给用户。
 */
export function extractionFailedMessage(errorCode?: string): string {
  switch (errorCode) {
    case "AI_UNCONFIGURED":
    case "EXTRACT_AI_UNCONFIGURED":
      return "简历识别服务尚未配置，请稍后再试或手动填写画像";
    case "EXTRACT_RATE_LIMITED":
      return "识别次数已达上限，请稍后再试";
    case "INVALID_FILE":
      return "简历文件无法识别，请上传 PDF、DOCX 或 TXT 格式";
    case "FILE_TOO_LARGE":
      return "简历文件超过 10 MB 上限，请压缩后重试";
    case "EXTRACT_FAILED":
      return "简历识别失败，请稍后重试；如为 PDF，请确认不超过 10 页，必要时压缩或删页。也可手动填写画像";
    default:
      return "简历识别未完成，请稍后重试";
  }
}

function appendExtractionRequestID(message: string, error: unknown): string {
  const requestID = error instanceof PortalApiError ? error.requestId?.trim() : undefined;
  if (!requestID || !/^req_[A-Za-z0-9_-]{1,116}$/.test(requestID)) return message;
  return `${message}（请求编号：${requestID}）`;
}

/**
 * Upload/create failures happen before polling exists, so they need different
 * guidance from a terminal worker failure. Never echo transport/provider text;
 * retain only the Gateway request id operators can safely search for.
 */
export function extractionCreateFailedMessage(error: unknown): string {
  let message: string;
  if (error instanceof PortalHttpError) {
    if (error.errorCode) {
      message = extractionFailedMessage(error.errorCode);
    } else if (error.status === 413) {
      message = extractionFailedMessage("FILE_TOO_LARGE");
    } else if (error.status === 429) {
      message = extractionFailedMessage("EXTRACT_RATE_LIMITED");
    } else {
      message = "简历尚未上传成功，服务暂时不可用，请稍后重试";
    }
  } else if (error instanceof PortalNetworkError) {
    message = "上传连接中断，无法确认是否已提交。请等待一分钟后再重试，避免重复创建识别任务";
  } else if (
    error instanceof PortalApiError &&
    (error.code === "PORTAL_PARSE_ERROR" || error.code === "PORTAL_EMPTY_RESPONSE")
  ) {
    message = "简历上传响应异常，无法确认任务是否创建。请等待一分钟后再重试";
  } else {
    message = "简历尚未上传成功，请检查网络后重试";
  }
  return appendExtractionRequestID(message, error);
}

export interface ExtractionRunner {
  /** 启动（或重置）一条轮询循环；幂等，绝不叠加第二条循环。 */
  start(extraction: CareerResumeExtraction): void;
  /** 停止轮询；可重复调用，卸载清理用。 */
  stop(): void;
  getState(): ExtractionPollState | null;
}

export interface ExtractionRunnerOptions {
  fetchStatus: ExtractionStatusFetcher;
  onState: (state: ExtractionPollState) => void;
  intervalMs?: number;
}

export function createExtractionRunner(options: ExtractionRunnerOptions): ExtractionRunner {
  const intervalMs = options.intervalMs ?? EXTRACTION_POLL_INTERVAL_MS;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let stopped = true;
  let lastExtraction: CareerResumeExtraction | null = null;
  let state: ExtractionPollState | null = null;

  function clearTimer() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function emit(next: ExtractionPollState) {
    state = next;
    options.onState(next);
  }

  function scheduleNext() {
    if (stopped) return;
    timer = setTimeout(() => {
      timer = null;
      void tick();
    }, intervalMs);
  }

  async function tick() {
    if (stopped || !lastExtraction) return;
    let extraction: CareerResumeExtraction;
    try {
      extraction = await options.fetchStatus(lastExtraction.id);
    } catch {
      // 瞬态失败：保留最近状态继续轮询，并透出一条可读提示。
      if (!stopped) {
        emit({ kind: "active", extraction: lastExtraction, pollError: "状态读取失败，正在自动重试" });
      }
      scheduleNext();
      return;
    }
    if (stopped) return;
    lastExtraction = extraction;
    if (extraction.status === "completed") {
      stop();
      emit({ kind: "completed", extraction });
      return;
    }
    if (extraction.status === "failed") {
      stop();
      emit({ kind: "failed", extraction });
      return;
    }
    emit({ kind: "active", extraction, pollError: null });
    scheduleNext();
  }

  function stop() {
    stopped = true;
    clearTimer();
  }

  return {
    start(extraction) {
      // 无论是否已在轮询都先复位：保证同时只有一条循环。
      clearTimer();
      stopped = false;
      lastExtraction = extraction;
      if (extraction.status === "completed") {
        stop();
        emit({ kind: "completed", extraction });
        return;
      }
      if (extraction.status === "failed") {
        stop();
        emit({ kind: "failed", extraction });
        return;
      }
      emit({ kind: "active", extraction, pollError: null });
      scheduleNext();
    },
    stop,
    getState() {
      return state;
    },
  };
}
