import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CareerSearch } from "../api/types";
import {
  CAREER_POLL_INTERVAL_MS,
  careerScanFailedMessage,
  careerScanStageLabel,
  careerSearchStatusLabel,
  createCareerScanRunner,
  formatCareerSearchTime,
  isCareerScanActive,
} from "./career-scan-state";

const SEARCH_ID = "11111111-1111-4111-8111-111111111111";
const USER_ID = "22222222-2222-4222-8222-222222222222";

function search(overrides: Partial<CareerSearch> = {}): CareerSearch {
  return {
    id: SEARCH_ID,
    status: "queued",
    user_id: USER_ID,
    has_email: false,
    created_at: "2026-08-16T00:00:00Z",
    ...overrides,
  };
}

describe("career scan state machine", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("polls queued → running → completed and stops after the terminal state", async () => {
    const statuses: CareerSearch["status"][] = ["running", "completed"];
    const fetchStatus = vi.fn(async () =>
      search({ status: statuses.shift() ?? "completed" })
    );
    const onState = vi.fn();
    const runner = createCareerScanRunner({ fetchStatus, onState });

    runner.start(search());
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "active", pollError: null })
    );

    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus).toHaveBeenCalledTimes(1);
    expect(fetchStatus).toHaveBeenCalledWith(SEARCH_ID);
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "active", search: expect.objectContaining({ status: "running" }) })
    );

    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus).toHaveBeenCalledTimes(2);
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "completed", search: expect.objectContaining({ status: "completed" }) })
    );

    const callsAfterCompletion = fetchStatus.mock.calls.length;
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS * 10);
    expect(fetchStatus.mock.calls.length).toBe(callsAfterCompletion);
  });

  it("stops polling once the search fails", async () => {
    const fetchStatus = vi.fn(async () => search({ status: "failed" }));
    const onState = vi.fn();
    const runner = createCareerScanRunner({ fetchStatus, onState });

    runner.start(search({ status: "running" }));
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "failed", search: expect.objectContaining({ status: "failed" }) })
    );

    const callsAfterFailure = fetchStatus.mock.calls.length;
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS * 10);
    expect(fetchStatus.mock.calls.length).toBe(callsAfterFailure);
  });

  it("stops polling on stop() so an unmounted component never fetches again", async () => {
    const fetchStatus = vi.fn(async () => search({ status: "running" }));
    const onState = vi.fn();
    const runner = createCareerScanRunner({ fetchStatus, onState });

    runner.start(search());
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus).toHaveBeenCalledTimes(1);

    runner.stop();
    const callsAfterStop = fetchStatus.mock.calls.length;
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS * 10);
    expect(fetchStatus.mock.calls.length).toBe(callsAfterStop);

    runner.stop();
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus.mock.calls.length).toBe(callsAfterStop);
  });

  it("never starts a second loop when start() is called again", async () => {
    const fetchStatus = vi.fn(async () => search({ status: "running" }));
    const onState = vi.fn();
    const runner = createCareerScanRunner({ fetchStatus, onState });

    runner.start(search());
    runner.start(search({ status: "running" }));
    runner.start(search({ status: "running" }));

    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus).toHaveBeenCalledTimes(2);
  });

  it("recovers a running search on start and polls until the terminal status", async () => {
    const statuses: CareerSearch["status"][] = ["running", "completed"];
    const fetchStatus = vi.fn(async () =>
      search({ status: statuses.shift() ?? "completed" })
    );
    const onState = vi.fn();
    const runner = createCareerScanRunner({ fetchStatus, onState });

    runner.start(search({ status: "running" }));
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "active", search: expect.objectContaining({ status: "running" }) })
    );
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(onState).toHaveBeenLastCalledWith(expect.objectContaining({ kind: "completed" }));
  });

  it("renders a persisted terminal state from the initial search without any polling", async () => {
    const fetchStatus = vi.fn(async () => search({ status: "completed" }));
    const onState = vi.fn();
    const runner = createCareerScanRunner({ fetchStatus, onState });

    runner.start(search({ status: "completed" }));
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "completed", search: expect.objectContaining({ status: "completed" }) })
    );

    runner.start(search({ status: "failed" }));
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "failed", search: expect.objectContaining({ status: "failed" }) })
    );

    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS * 10);
    expect(fetchStatus).not.toHaveBeenCalled();
  });

  it("keeps polling across transient fetch errors and surfaces a readable notice", async () => {
    let failNext = true;
    const fetchStatus = vi.fn(async () => {
      if (failNext) {
        failNext = false;
        throw new Error("network down");
      }
      return search({ status: "running", stage: "matching" });
    });
    const onState = vi.fn();
    const runner = createCareerScanRunner({ fetchStatus, onState });

    runner.start(search({ status: "running" }));
    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus).toHaveBeenCalledTimes(1);
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "active", pollError: expect.any(String) })
    );

    await vi.advanceTimersByTimeAsync(CAREER_POLL_INTERVAL_MS);
    expect(fetchStatus).toHaveBeenCalledTimes(2);
    expect(onState).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "active", pollError: null, search: expect.objectContaining({ stage: "matching" }) })
    );
  });

  it("reports the latest state through getState", () => {
    const runner = createCareerScanRunner({
      fetchStatus: vi.fn(async () => search({ status: "running" })),
      onState: vi.fn(),
    });
    expect(runner.getState()).toBeNull();
    runner.start(search());
    expect(runner.getState()).toMatchObject({ kind: "active" });
    runner.stop();
  });
});

describe("career scan labels", () => {
  it("maps status and stage to stable Chinese labels", () => {
    expect(careerSearchStatusLabel("queued")).toBe("排队中");
    expect(careerSearchStatusLabel("running")).toBe("扫描中");
    expect(careerSearchStatusLabel("completed")).toBe("已完成");
    expect(careerSearchStatusLabel("failed")).toBe("失败");

    expect(careerScanStageLabel("crawling")).toBe("正在抓取岗位来源");
    expect(careerScanStageLabel("matching")).toBe("正在匹配岗位机会");
    expect(careerScanStageLabel("rendering")).toBe("正在生成结果");
    expect(careerScanStageLabel(undefined)).toBeNull();
    expect(careerScanStageLabel("")).toBeNull();
    expect(careerScanStageLabel("unknown" as never)).toBeNull();
  });

  it("treats queued/running as active and completed/failed as terminal", () => {
    expect(isCareerScanActive(search({ status: "queued" }))).toBe(true);
    expect(isCareerScanActive(search({ status: "running" }))).toBe(true);
    expect(isCareerScanActive(search({ status: "completed" }))).toBe(false);
    expect(isCareerScanActive(search({ status: "failed" }))).toBe(false);
  });

  it("returns a stable failure copy without internal details", () => {
    expect(careerScanFailedMessage()).toBe("扫描未能完成，请稍后重试");
  });

  it("formats ISO timestamps and falls back to the raw value", () => {
    expect(formatCareerSearchTime("2026-08-16T00:00:00Z")).toMatch(/\d{1,2}\/\d{1,2}/);
    expect(formatCareerSearchTime("not-a-date")).toBe("not-a-date");
  });
});
