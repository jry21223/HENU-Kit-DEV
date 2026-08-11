// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchLearningState: vi.fn(),
}));

vi.mock("../api/client", () => ({
  fetchLearningState: mocks.fetchLearningState,
  formatPortalError: () => "unavailable",
  PortalUnauthorizedError: class PortalUnauthorizedError extends Error {},
}));

vi.mock("../api/env", () => ({
  quizCraftV2ReadsEnabled: () => true,
}));

import { useLearningState } from "./learning-state";

const facts = Array.from({ length: 21 }, (_, index) => ({
  bank_id: "33333333-3333-4333-8333-333333333333",
  question_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
  question_version_id: `10000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
  wrong: true,
  attempt_count: 1,
  correct_count: 0,
  updated_at: "2026-08-11T08:00:00Z",
}));

function envelope(page: number, items: typeof facts, total: number) {
  return {
    request_id: `req_page_${page}`,
    data: {
      items,
      pagination: {
        page,
        page_size: 20,
        total,
        total_pages: total === 0 ? 0 : Math.ceil(total / 20),
      },
    },
  };
}

function Harness() {
  const { state, nextPage, reload } = useLearningState();
  return (
    <div>
      <output data-testid="status">{state.status}</output>
      <output data-testid="facts">
        {state.status === "ready" ? state.items.map((item) => item.question_id).join(",") : ""}
      </output>
      <button type="button" onClick={nextPage}>next</button>
      <button type="button" onClick={reload}>reload</button>
    </div>
  );
}

async function waitFor(assertion: () => void) {
  let lastError: unknown;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    try {
      assertion();
      return;
    } catch (error) {
      lastError = error;
    }
  }
  throw lastError;
}

describe("useLearningState pagination", () => {
  afterEach(() => {
    mocks.fetchLearningState.mockReset();
    document.body.replaceChildren();
  });

  it("returns from a vanished second page to the remaining first page", async () => {
    let shrunk = false;
    mocks.fetchLearningState.mockImplementation(async (page: number) => {
      if (page === 1) return envelope(1, facts.slice(0, 20), shrunk ? 20 : 21);
      return envelope(2, shrunk ? [] : facts.slice(20), shrunk ? 20 : 21);
    });

    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    await act(async () => root.render(<Harness />));
    await waitFor(() => expect(container.textContent).toContain(facts[0].question_id));

    await act(async () => {
      container.querySelector<HTMLButtonElement>("button")?.click();
    });
    await waitFor(() => expect(container.textContent).toContain(facts[20].question_id));

    shrunk = true;
    await act(async () => {
      container.querySelectorAll<HTMLButtonElement>("button")[1]?.click();
    });
    await waitFor(() => expect(container.textContent).toContain(facts[0].question_id));
    expect(container.querySelector('[data-testid="status"]')?.textContent).toBe("ready");
    expect(mocks.fetchLearningState.mock.calls.slice(-2)).toEqual([
      [2, 20, true],
      [1, 20, true],
    ]);

    await act(async () => root.unmount());
  });
});
