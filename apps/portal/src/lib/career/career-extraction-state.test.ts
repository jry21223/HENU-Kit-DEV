import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createExtractionRunner,
  extractionCreateFailedMessage,
  extractionFailedMessage,
  extractionStatusLabel,
  isExtractionActive,
  isExtractionTerminal,
} from "./career-extraction-state";
import {
  PortalApiError,
  PortalHttpError,
  PortalNetworkError,
} from "../api/client";
import type { CareerResumeExtraction } from "../api/types";

function extraction(
  status: CareerResumeExtraction["status"],
  errorCode?: string
): CareerResumeExtraction {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    status,
    user_id: "22222222-2222-4222-8222-222222222222",
    file_name: "resume.txt",
    error_code: errorCode,
    created_at: "2026-08-16T00:00:00Z",
  };
}

describe("extraction status helpers", () => {
  it("treats queued and running as active, completed and failed as terminal", () => {
    expect(isExtractionActive(extraction("queued"))).toBe(true);
    expect(isExtractionActive(extraction("running"))).toBe(true);
    expect(isExtractionActive(extraction("completed"))).toBe(false);
    expect(isExtractionActive(extraction("failed"))).toBe(false);
    expect(isExtractionTerminal(extraction("completed"))).toBe(true);
    expect(isExtractionTerminal(extraction("failed"))).toBe(true);
  });

  it("labels statuses in Chinese", () => {
    expect(extractionStatusLabel("queued")).toBe("排队中");
    expect(extractionStatusLabel("running")).toBe("识别中");
    expect(extractionStatusLabel("completed")).toBe("已完成");
    expect(extractionStatusLabel("failed")).toBe("失败");
  });

  it("maps stable error codes to readable messages and never leaks internals", () => {
    expect(extractionFailedMessage("AI_UNCONFIGURED")).toContain("尚未配置");
    expect(extractionFailedMessage("EXTRACT_RATE_LIMITED")).toContain("上限");
    expect(extractionFailedMessage("INVALID_FILE")).toContain("PDF、DOCX");
    expect(extractionFailedMessage("FILE_TOO_LARGE")).toContain("10 MB");
    expect(extractionFailedMessage("EXTRACT_FAILED")).toContain("稍后重试");
    expect(extractionFailedMessage("EXTRACT_FAILED")).toContain("PDF");
    expect(extractionFailedMessage("EXTRACT_FAILED")).toContain("不超过 10 页");
    expect(extractionFailedMessage("EXTRACT_FAILED")).toContain("压缩或删页");
    expect(extractionFailedMessage("UNKNOWN_WEIRD")).toContain("稍后重试");
  });

  it("explains upload-stage network, parse, and HTTP failures with safe request ids", () => {
    const network = extractionCreateFailedMessage(
      new PortalNetworkError(
        "/api/v1/career/profile/extractions",
        new TypeError("connection reset"),
        "req_career_upload_0123456789abcdef0123456789abcdef"
      )
    );
    expect(network).toContain("无法确认是否已提交");
    expect(network).toContain("等待一分钟");
    expect(network).toContain("req_career_upload_0123456789abcdef0123456789abcdef");
    expect(network).not.toContain("connection reset");

    const parse = extractionCreateFailedMessage(
      new PortalApiError("invalid provider HTML", {
        code: "PORTAL_PARSE_ERROR",
        requestId: "req_career_parse_failure",
      })
    );
    expect(parse).toContain("响应异常");
    expect(parse).toContain("req_career_parse_failure");
    expect(parse).not.toContain("provider HTML");

    const tooLarge = extractionCreateFailedMessage(
      new PortalHttpError(
        "/api/v1/career/profile/extractions",
        413,
        "nginx rejected body",
        "req_career_too_large"
      )
    );
    expect(tooLarge).toContain("10 MB");
    expect(tooLarge).toContain("req_career_too_large");
    expect(tooLarge).not.toContain("nginx");
  });
});

describe("createExtractionRunner", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("polls until completed and stops", async () => {
    const states: string[] = [];
    const fetcher = vi
      .fn<() => Promise<CareerResumeExtraction>>()
      .mockResolvedValueOnce(extraction("queued"))
      .mockResolvedValueOnce(extraction("running"))
      .mockResolvedValueOnce(
        extraction("completed", undefined)
      );
    const runner = createExtractionRunner({
      fetchStatus: fetcher,
      onState: (state) => states.push(state.kind),
    });
    runner.start(extraction("queued"));

    await vi.advanceTimersByTimeAsync(2000);
    await vi.advanceTimersByTimeAsync(2000);
    await vi.advanceTimersByTimeAsync(2000);
    await vi.advanceTimersByTimeAsync(2000);

    expect(states).toEqual(["active", "active", "active", "completed"]);
    expect(runner.getState()?.kind).toBe("completed");
    // 终态后不再轮询。
    const calls = fetcher.mock.calls.length;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(fetcher.mock.calls.length).toBe(calls);
  });

  it("stops on failed and surfaces the extraction for a readable message", async () => {
    const states: string[] = [];
    const failed = extraction("failed", "EXTRACT_FAILED");
    const fetcher = vi
      .fn<() => Promise<CareerResumeExtraction>>()
      .mockResolvedValueOnce(extraction("running"))
      .mockResolvedValueOnce(failed);
    const runner = createExtractionRunner({
      fetchStatus: fetcher,
      onState: (state) => states.push(state.kind),
    });
    runner.start(extraction("queued"));

    await vi.advanceTimersByTimeAsync(2000);
    await vi.advanceTimersByTimeAsync(2000);
    await vi.advanceTimersByTimeAsync(10_000);

    expect(states).toEqual(["active", "active", "failed"]);
    const finalState = runner.getState();
    expect(finalState?.kind).toBe("failed");
    if (finalState?.kind === "failed") {
      expect(extractionFailedMessage(finalState.extraction.error_code)).toContain("重试");
    }
  });

  it("keeps polling across transient fetch errors and surfaces a readable notice", async () => {
    const states: string[] = [];
    let failNext = true;
    const fetcher = vi.fn(async () => {
      if (failNext) {
        failNext = false;
        throw new Error("network down");
      }
      return extraction("completed", undefined);
    });
    const runner = createExtractionRunner({
      fetchStatus: fetcher,
      onState: (state) =>
        states.push(`${state.kind}:${state.kind === "active" ? state.pollError ?? "" : ""}`),
    });
    runner.start(extraction("running"));

    await vi.advanceTimersByTimeAsync(2000);
    await vi.advanceTimersByTimeAsync(2000);
    await vi.advanceTimersByTimeAsync(2000);

    expect(states).toEqual([
      "active:",
      "active:状态读取失败，正在自动重试",
      "completed:",
    ]);
  });

  it("starts immediately terminal when the extraction is already done", () => {
    const states: string[] = [];
    const runner = createExtractionRunner({
      fetchStatus: vi.fn(),
      onState: (state) => states.push(state.kind),
    });
    runner.start(extraction("completed", undefined));
    expect(states).toEqual(["completed"]);
  });

  it("stop is idempotent and cancels the loop", async () => {
    const fetcher = vi
      .fn<() => Promise<CareerResumeExtraction>>()
      .mockResolvedValue(extraction("queued"));
    const runner = createExtractionRunner({
      fetchStatus: fetcher,
      onState: () => undefined,
    });
    runner.start(extraction("queued"));
    runner.stop();
    runner.stop();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(fetcher).not.toHaveBeenCalled();
  });
});
