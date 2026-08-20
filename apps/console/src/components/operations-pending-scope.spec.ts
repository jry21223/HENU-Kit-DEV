import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import FoodOperationsView from "./FoodOperationsView.vue";
import LibraryOperationsView from "./LibraryOperationsView.vue";

/**
 * An unconfirmed command is parked in sessionStorage so the operator who issued
 * it can retry after a reload. sessionStorage survives a same-tab account
 * switch, so it must be scoped to the operator who issued it: otherwise the
 * next person to sign in is offered "按原请求重试" for a decision that is not
 * theirs, replays it under their own session, and takes the blame for it in the
 * audit trail.
 */

const OPERATOR_A = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa";
const OPERATOR_B = "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb";

vi.mock("./ui", async () => await vi.importActual("./ui"));

vi.mock("@/lib/console-gateway", () => ({
  fetchLibraryWorkspace: vi.fn(async () => ({
    state: "authenticated" as const,
    workspace: { status: "ok", status_message: "", as_of: "2026-08-20T00:00:00Z", degraded: false, courses: [], materials: [], submissions: [], corrections: [], downloads: [] },
  })),
  fetchFoodWorkspace: vi.fn(async () => ({
    state: "authenticated" as const,
    workspace: { status: "ok", status_message: "", as_of: "2026-08-20T00:00:00Z", stale: false, posts: [], submissions: [], anomaly_tickets: [], tier_adjustments: [] },
  })),
  executeLibraryCommand: vi.fn(),
  executeFoodCommand: vi.fn(),
  resolveLibraryOperation: vi.fn(),
  resolveFoodOperation: vi.fn(),
}));

const RETRY_BANNER = "按原请求重试";

const cases = [
  {
    name: "Library",
    component: LibraryOperationsView,
    storageKey: "henukit.library.pending-operation",
    permissions: ["library.content.write"],
    pending: (operatorID: string) => ({
      kind: "submission_reject",
      operatorID,
      key: "idem_library_submission_reject_1",
      input: { kind: "submission_reject", resource_id: "11111111-1111-4111-8111-111111111111", expected_version: "2026-08-20T00:00:00Z", payload: { reviewReason: "内容不完整" } },
      success: "投稿已驳回。",
    }),
  },
  {
    name: "Food",
    component: FoodOperationsView,
    storageKey: "henukit.food.pending-operation",
    permissions: ["food.content.write"],
    pending: (operatorID: string) => ({
      kind: "submission_reject",
      operatorID,
      key: "idem_food_submission_reject_1",
      input: { kind: "submission_reject", resource_id: "22222222-2222-4222-8222-222222222222", expected_version: 1, payload: { note: "重复投稿" } },
      success: "投稿已驳回。",
    }),
  },
];

describe.each(cases)("$name operations pending command", ({ component, storageKey, permissions, pending }) => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  afterEach(() => {
    sessionStorage.clear();
  });

  async function mountFor(operatorID: string) {
    const wrapper = mount(component, {
      props: { authState: "authenticated" as const, operatorID, permissions },
    });
    await flushPromises();
    return wrapper;
  }

  it("offers the retry to the operator who issued it", async () => {
    sessionStorage.setItem(storageKey, JSON.stringify(pending(OPERATOR_A)));

    const wrapper = await mountFor(OPERATOR_A);

    expect(wrapper.text()).toContain(RETRY_BANNER);
  });

  it("does not offer another operator's unconfirmed command after an account switch", async () => {
    sessionStorage.setItem(storageKey, JSON.stringify(pending(OPERATOR_A)));

    const wrapper = await mountFor(OPERATOR_B);

    expect(wrapper.text()).not.toContain(RETRY_BANNER);
    // The stale record is cleared rather than left to resurface on the next
    // mount under yet another identity.
    expect(sessionStorage.getItem(storageKey)).toBeNull();
  });

  it("discards a record with no operator, which is what pre-scoping builds wrote", async () => {
    const legacy = pending(OPERATOR_A) as Record<string, unknown>;
    delete legacy.operatorID;
    sessionStorage.setItem(storageKey, JSON.stringify(legacy));

    const wrapper = await mountFor(OPERATOR_A);

    expect(wrapper.text()).not.toContain(RETRY_BANNER);
  });
});
