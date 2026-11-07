import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchAccountTicketFacts, fetchFoodFacts } from "./console-gateway";
import { beginConsolePrincipal, onConsoleSessionExpired } from "./console-session";
const stamp = "2026-11-07T12:00:00Z";
const emptyFood = () => ({ scope: "global", as_of: stamp, stale: false, counts: { pending_submissions: 0, open_anomalies: 0, pending_tier_adjustments: 0 }, windows: Object.fromEntries([["submissions", "pending"], ["anomalies", "open"], ["tier_adjustments", "pending"]].map(([name, filter]) => [name, { filter, limit: 200, returned: 0, has_more: false, items: [] }])) });
const tickets = () => ({ scope: "global", as_of: stamp, stale: false, counts: { open: 1, in_progress: 0, resolved: 2, unresolved: 1 }, window: { filter: "unresolved", matched_total: 1, limit: 100, returned: 1, has_more: false, items: [{ id: "11111111-1111-4111-8111-111111111111", reference: "HKT-11111111-1111-4111-8111-111111111111", title: "受控工单", status: "open", version: 1, updated_at: stamp }] } });
const respond = (data: unknown) => new Response(JSON.stringify({ data, request_id: "req_business_facts" }), { status: 200 });
beforeEach(() => beginConsolePrincipal()); afterEach(() => vi.unstubAllGlobals());
describe("fresh business facts", () => {
  it("accepts real zero only with complete empty windows and forwards abort signal", async () => {
    const mock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => respond(emptyFood())); vi.stubGlobal("fetch", mock); const controller = new AbortController();
    expect((await fetchFoodFacts({ signal: controller.signal })).state).toBe("authenticated"); expect(String(mock.mock.calls[0]![0])).toBe("/api/v1/food/facts"); expect(mock.mock.calls[0]![1]).toMatchObject({ signal: controller.signal });
  });
  it.each(["missing-count", "stale", "window-count", "private-field"])("rejects Food %s instead of fabricating success", async (kind) => {
    const facts: any = emptyFood(); if (kind === "missing-count") delete facts.counts.pending_submissions; if (kind === "stale") facts.stale = true; if (kind === "window-count") facts.counts.pending_submissions = 201; if (kind === "private-field") facts.operator_id = "internal";
    vi.stubGlobal("fetch", vi.fn(async () => respond(facts))); expect(await fetchFoodFacts()).toEqual({ state: "unavailable" });
  });
  it("uses unresolved queue and owner reference/title without extra identity requests", async () => {
    const mock = vi.fn(async (_input: RequestInfo | URL) => respond(tickets())); vi.stubGlobal("fetch", mock); const result = await fetchAccountTicketFacts(); expect(result.state).toBe("authenticated"); expect(mock).toHaveBeenCalledTimes(1); expect(String(mock.mock.calls[0]![0])).toBe("/api/v1/account/tickets/facts?status=unresolved");
  });
  it.each(["wrong-total", "resolved-in-unresolved", "no-title", "wrong-filter", "private-field"])("rejects ticket %s", async (kind) => {
    const facts: any = tickets(); if (kind === "wrong-total") facts.counts.unresolved = 2; if (kind === "resolved-in-unresolved") facts.window.items[0].status = "resolved"; if (kind === "no-title") delete facts.window.items[0].title; if (kind === "wrong-filter") facts.window.filter = "all"; if (kind === "private-field") facts.window.items[0].user_id = "11111111-1111-4111-8111-111111111111";
    vi.stubGlobal("fetch", vi.fn(async () => respond(facts))); expect(await fetchAccountTicketFacts()).toEqual({ state: "unavailable" });
  });
  it.each([
    ["Chinese54", "中", 54, true], ["Chinese160", "中", 160, true], ["Chinese161", "中", 161, false],
    ["Emoji100", "😀", 100, true], ["Emoji160", "😀", 160, true], ["Emoji161", "😀", 161, false],
  ] as const)("counts Unicode code points in %s", async (_name, unit, length, valid) => {
    const facts = tickets(); facts.window.items[0]!.title = unit.repeat(length);
    vi.stubGlobal("fetch", vi.fn(async () => respond(facts)));
    expect((await fetchAccountTicketFacts()).state).toBe(valid ? "authenticated" : "unavailable");
  });
  it("separates owner unavailable/denied from verified session expiry", async () => {
    const expired = vi.fn(); const off = onConsoleSessionExpired(expired); try {
      vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 503 }))); expect(await fetchFoodFacts()).toEqual({ state: "unavailable" }); expect(expired).not.toHaveBeenCalled();
      vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 403 }))); expect(await fetchAccountTicketFacts()).toEqual({ state: "denied" }); expect(expired).not.toHaveBeenCalled();
      vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 401 }))); expect(await fetchFoodFacts()).toEqual({ state: "signed_out" }); expect(expired).toHaveBeenCalledTimes(1);
    } finally { off(); }
  });
  it("rejects an old principal's late response", async () => {
    let finish!: (value: Response) => void; vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(resolve => { finish = resolve; }))); const pending = fetchAccountTicketFacts(); beginConsolePrincipal(); finish(respond(tickets())); expect(await pending).toEqual({ state: "unavailable" });
  });
});
