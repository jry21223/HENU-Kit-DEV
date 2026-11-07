import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import AccountMembershipOperationsView from "./components/AccountMembershipOperationsView.vue";
import AccountPointOperationsView from "./components/AccountPointOperationsView.vue";

const actor = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const otherActor = "271f1c6f-7b10-4c92-91a2-b39bf5af5302";
const a = "88888888-8888-4888-8888-888888888888";
const b = "77777777-7777-4777-8777-777777777777";
const memberKey = "henukit.console.pending-membership-operation.v1";
const pointKey = "henukit.console.pending-point-operation.v1";
const membership = (version = 1, lifetime = false) => ({ plan: lifetime ? "lifetime" : "free", lifetime, version });
const identity = (id: string) => ({ id, display_name: id === a ? "甲同学" : "乙同学", email: id === a ? "a@henu.edu.cn" : "b@henu.edu.cn", status: "active" });
const memberAccount = (id: string, value: unknown = membership()) => ({ ...identity(id), membership: value });
const pointResult = (amount = 7) => ({ balance: 12, entry: { id: "33333333-3333-4333-8333-333333333333", amount, reason: "Private reason", created_at: "2026-11-07T00:00:00Z" } });
const response = (data: unknown, status = 200) => new Response(JSON.stringify({ data, request_id: "req_recovery_fixture" }), { status, headers: { "Content-Type": "application/json" } });
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }
let wrapper: VueWrapper | undefined;
let attachedHost: HTMLElement | undefined;
const requests: Array<{ path: string; method: string; body: unknown; key: string | null }> = [];
let route: (path: string, init?: RequestInit) => Promise<Response>;
beforeEach(() => {
  sessionStorage.clear(); requests.length = 0;
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a), memberAccount(b)], next_page: null })
    : path.includes("/account-lookups") ? response({ account: identity(a) }) : response({ membership: membership() });
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    requests.push({ path, method: init?.method ?? "GET", body: init?.body ? JSON.parse(String(init.body)) : undefined, key: new Headers(init?.headers).get("Idempotency-Key") });
    return route(path, init);
  }));
});
afterEach(() => { wrapper?.unmount(); wrapper = undefined; attachedHost?.remove(); attachedHost = undefined; vi.restoreAllMocks(); vi.unstubAllGlobals(); });
async function open(kind: "membership" | "points", operatorID = actor) {
  wrapper = mount(kind === "membership" ? AccountMembershipOperationsView : AccountPointOperationsView, { props: { authState: "authenticated", operatorID, permissions: [kind === "membership" ? "account.membership.write" : "account.points.adjust"] } });
  await flushPromises();
}
function button(text: string) { const value = wrapper!.findAll("button").find((item) => item.text().includes(text)); if (!value) throw new Error(`Missing button ${text}`); return value; }
async function confirmMember() {
  await wrapper!.find("textarea").setValue("Private reason");
  await wrapper!.findAll("form").slice(-1)[0]!.trigger("submit");
  await button("确认执行").trigger("click"); await flushPromises();
}
async function lookup(id = a) {
  await wrapper!.find('input[type="email"]').setValue(identity(id).email);
  await button("查找账户").trigger("click"); await flushPromises();
}
async function confirmPoints(amount = "7") {
  await wrapper!.find('input[inputmode="numeric"]').setValue(amount);
  await wrapper!.find("textarea").setValue("Private reason");
  await wrapper!.find("form").trigger("submit");
  await button("确认写入").trigger("click"); await flushPromises();
}

it("membership lost responses and reloads only GET the original key with minimal actor-bound metadata", async () => {
  let recovered = false;
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a)], next_page: null })
    : path.endsWith("/grants") ? Promise.reject(new Error("response lost"))
    : path.includes("/account/operations/") ? response({ operation: "membership_grant", status: recovered ? "succeeded" : "unknown", ...(recovered ? { result: { membership: membership(2, true) } } : {}) })
    : response({ membership: membership(recovered ? 2 : 1, recovered) });
  await open("membership"); await button("甲同学").trigger("click"); await confirmMember();
  const post = requests.find((item) => item.path.endsWith("/grants"))!;
  expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
  expect(JSON.parse(sessionStorage.getItem(memberKey)!)).toEqual({ version: 1, operator_id: actor, operation: "membership_grant", resource_id: a, expected_version: 1, idempotency_key: post.key });
  expect(sessionStorage.getItem(memberKey)).not.toContain("Private reason");
  expect(sessionStorage.getItem(memberKey)).not.toContain("henu.edu.cn");
  wrapper!.unmount(); wrapper = undefined;
  await open("membership");
  expect(requests.filter((item) => item.path.endsWith("/grants"))).toHaveLength(1);
  recovered = true;
  await button("查询原操作结果").trigger("click"); await flushPromises();
  expect(requests.filter((item) => item.path.includes("/account/operations/")).every((item) => item.path.endsWith(post.key!))).toBe(true);
  expect(wrapper!.text()).toContain("终身会员权益已发放");
  expect(sessionStorage.getItem(memberKey)).toBeNull();
});

it("points recover the original ledger fact by GET and never infer success from a later lookup", async () => {
  let recovered = false;
  route = async (path) => path.includes("/account-lookups") ? response({ account: identity(a) })
    : path.endsWith("/adjustments") ? Promise.reject(new Error("response lost"))
    : response({ operation: "point_adjustment", status: recovered ? "succeeded" : "unknown", ...(recovered ? { result: pointResult() } : {}) });
  await open("points"); await lookup(); await confirmPoints();
  const post = requests.find((item) => item.path.endsWith("/adjustments"))!;
  expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
  expect(JSON.parse(sessionStorage.getItem(pointKey)!)).toEqual({ version: 1, operator_id: actor, operation: "point_adjustment", resource_id: a, expected_amount: 7, idempotency_key: post.key });
  expect(sessionStorage.getItem(pointKey)).not.toContain("Private reason");
  wrapper!.unmount(); wrapper = undefined;
  await open("points"); await lookup();
  expect(wrapper!.text()).not.toContain("本次调整后的余额");
  recovered = true;
  await button("查询原操作结果").trigger("click"); await flushPromises();
  expect(requests.filter((item) => item.path.endsWith("/adjustments"))).toHaveLength(1);
  expect(requests.filter((item) => item.path.includes("/account/operations/")).every((item) => item.path.endsWith(post.key!))).toBe(true);
  expect(wrapper!.text()).toContain("本次调整后的余额 12");
  expect(wrapper!.text()).toContain("Private reason");
  expect(sessionStorage.getItem(pointKey)).toBeNull();
});

it("points do not present an earlier success as the result of a new rejected adjustment", async () => {
  let writes = 0;
  route = async (path) => path.includes("/account-lookups") ? response({ account: identity(a) })
    : ++writes === 1 ? response(pointResult()) : response({}, 409);
  await open("points"); await lookup(); await confirmPoints();
  expect(wrapper!.text()).toContain("本次调整后的余额 12");
  await confirmPoints("-20");
  expect(wrapper!.text()).toContain("积分不足或数据有变化");
  expect(wrapper!.text()).not.toContain("本次调整后的余额 12");
  expect(wrapper!.find('input[inputmode="numeric"]').element).toHaveProperty("value", "-20");
  expect(wrapper!.find("textarea").element).toHaveProperty("value", "Private reason");
});

it.each(["membership", "points"] as const)("%s upgrades a prior actor-bound full command to read-only minimal recovery", async (kind) => {
  const key = `idem_account_${kind}_11111111-1111-4111-8111-111111111111`;
  const legacyKey = kind === "membership" ? "henukit.account-membership.pending-command" : "henukit.account-points.pending-command";
  sessionStorage.setItem(legacyKey, JSON.stringify(kind === "membership"
    ? { kind: "grant", operatorID: actor, userID: a, key, input: { reason: "Private reason", expected_version: 1 }, identity: { email: identity(a).email } }
    : { operatorID: actor, key, input: { user_id: a, amount: 7, reason: "Private reason" }, identity: { email: identity(a).email } }));
  route = async (path) => path.endsWith("/search") ? response({ accounts: [], next_page: null }) : response({ operation: kind === "membership" ? "membership_grant" : "point_adjustment", status: "unknown" });
  await open(kind);
  expect(requests.filter((item) => item.path.includes("/account/operations/"))).toMatchObject([{ path: expect.stringContaining(key), method: "GET" }]);
  expect(sessionStorage.getItem(legacyKey)).toBeNull();
  const stored = sessionStorage.getItem(kind === "membership" ? memberKey : pointKey)!;
  expect(stored).not.toContain("Private reason"); expect(stored).not.toContain("henu.edu.cn");
  expect(stored).toContain(key);
});

it.each(["membership", "points"] as const)("%s stops before POST if recovery storage fails", async (kind) => {
  await open(kind);
  if (kind === "membership") await button("甲同学").trigger("click"); else await lookup();
  vi.spyOn(Object.getPrototypeOf(window.sessionStorage), "setItem").mockImplementation(() => { throw new Error("storage denied"); });
  if (kind === "membership") await confirmMember(); else await confirmPoints();
  expect(wrapper!.text()).toContain("操作未提交");
  expect(requests.filter((item) => /\/(grants|revocations|adjustments)$/.test(item.path))).toHaveLength(0);
});

it.each(["membership", "points"] as const)("%s retains its original key after malformed success and failed result queries", async (kind) => {
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a)], next_page: null })
    : path.includes("/account-lookups") ? response({ account: identity(a) })
    : path.includes("/account/operations/") ? response({}, 503)
    : kind === "membership" ? response({ membership: membership(2, false) }) : response(pointResult(8));
  await open(kind);
  if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); } else { await lookup(); await confirmPoints(); }
  const post = requests.find((item) => /\/(grants|adjustments)$/.test(item.path))!;
  expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
  expect(wrapper!.text()).toContain("结果尚未确认");
  await button("查询原操作结果").trigger("click"); await flushPromises();
  expect(requests.filter((item) => /\/(grants|adjustments)$/.test(item.path))).toHaveLength(1);
  expect(requests.filter((item) => item.path.includes("/account/operations/")).every((item) => item.path.endsWith(post.key!))).toBe(true);
  expect(sessionStorage.getItem(kind === "membership" ? memberKey : pointKey)).toContain(post.key!);
  expect(wrapper!.text()).not.toContain("本次调整后的余额");
  expect(wrapper!.text()).not.toContain("终身会员权益已发放");
});

it.each(["membership", "points"] as const)("%s keeps B selected when A's delayed success or conflict arrives", async (kind) => {
  for (const outcome of [200, 409, 503]) {
    const delayed = deferred<Response>();
    route = async (path, init) => path.endsWith("/search") ? response({ accounts: [memberAccount(a), memberAccount(b)], next_page: null })
      : path.includes("/account-lookups") ? response({ account: identity((JSON.parse(String(init?.body)) as { email: string }).email === identity(b).email ? b : a) })
      : path.includes("/account/operations/") ? response({ operation: kind === "membership" ? "membership_grant" : "point_adjustment", status: "unknown" }) : delayed.promise;
    await open(kind);
    if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); await button("乙同学").trigger("click"); }
    else { await lookup(); await confirmPoints(); await lookup(b); }
    delayed.resolve(outcome === 200 ? response(kind === "membership" ? { membership: membership(2, true) } : pointResult()) : response({}, outcome)); await flushPromises();
    const selected = wrapper!.find(kind === "membership" ? '[data-account-membership-detail-state="ready"]' : '[data-account-points-result]');
    expect(selected.text()).toContain("乙同学");
    expect(selected.text()).not.toContain("甲同学");
    expect(wrapper!.text()).not.toContain("终身会员权益已发放");
    expect(wrapper!.text()).not.toContain("会员版本已变化");
    expect(wrapper!.text()).not.toContain("本次调整后的余额");
    expect(requests.some((item) => item.path.endsWith(`/${b}/grants`) || (item.path.endsWith("/adjustments") && (item.body as { user_id: string }).user_id === b))).toBe(false);
    wrapper!.unmount(); wrapper = undefined; sessionStorage.clear(); requests.length = 0;
  }
});

it("membership null and a later 404 remain uninitialized rather than confirmed free", async () => {
  let absent = true;
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a, absent ? null : membership())], next_page: null })
    : path.endsWith("/grants") ? response({}, 409) : response({}, 404);
  await open("membership"); await button("甲同学").trigger("click");
  expect(wrapper!.text()).toContain("尚未建立会员账户");
  expect(wrapper!.find("textarea").exists()).toBe(false);
  expect(wrapper!.text()).not.toContain("免费会员");
  absent = false;
  await wrapper!.find("form").trigger("submit"); await flushPromises(); await button("甲同学").trigger("click"); await confirmMember();
  expect(wrapper!.find('[data-account-membership-detail-state="not_found"]').exists()).toBe(true);
  expect(wrapper!.find("textarea").exists()).toBe(false);
  expect(wrapper!.text()).not.toContain("免费会员");
  expect(wrapper!.text()).not.toContain("已刷新最新权益");
});

it("membership's slow submitted search cannot overwrite a newer search and page context keeps no query text", async () => {
  const delayed = deferred<Response>();
  route = async (_path, init) => {
    const input = JSON.parse(String(init?.body)) as { query: string; page: number };
    return input.query === "Private A" ? delayed.promise : response({ accounts: [memberAccount(b)], next_page: input.page === 1 ? 2 : null });
  };
  await open("membership");
  await wrapper!.find("input").setValue("Private A"); await wrapper!.find("form").trigger("submit");
  await wrapper!.find("input").setValue("Private B"); await wrapper!.find("form").trigger("submit"); await flushPromises();
  delayed.resolve(response({ accounts: [memberAccount(a)], next_page: null })); await flushPromises();
  expect(wrapper!.text()).toContain("乙同学"); expect(wrapper!.text()).not.toContain("甲同学");
  await wrapper!.find("input").setValue("Draft C"); await button("下一页").trigger("click"); await flushPromises();
  expect(requests[requests.length - 1]?.body).toEqual({ query: "Private B", page: 2 });
  expect(sessionStorage.getItem("henukit.console.membership-view.v1")).not.toContain("Private");
  expect(window.location.search).toBe("");
});

it("points discard a late A lookup after the email changes to B", async () => {
  const delayed = deferred<Response>();
  route = async (_path, init) => (JSON.parse(String(init?.body)) as { email: string }).email === identity(a).email ? delayed.promise : response({ account: identity(b) });
  await open("points"); await lookup(); await lookup(b);
  delayed.resolve(response({ account: identity(a) })); await flushPromises();
  expect(wrapper!.find('[data-account-points-result]').text()).toContain("乙同学");
  expect(wrapper!.text()).not.toContain("甲同学");
  expect(requests.every((item) => !item.path.includes("henu.edu.cn"))).toBe(true);
});

it.each(["membership", "points"] as const)("%s keeps minimum metadata through expiry and only restores it to the same actor", async (kind) => {
  const delayed = deferred<Response>();
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a)], next_page: null })
    : path.includes("/account-lookups") ? response({ account: identity(a) })
    : path.includes("/account/operations/") ? response({ operation: kind === "membership" ? "membership_grant" : "point_adjustment", status: "unknown" }) : delayed.promise;
  await open(kind);
  if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); } else { await lookup(); await confirmPoints(); }
  const storageKey = kind === "membership" ? memberKey : pointKey;
  const stored = sessionStorage.getItem(storageKey)!;
  await wrapper!.setProps({ authState: "signed_out" }); await flushPromises();
  expect(wrapper!.text()).not.toContain("甲同学");
  expect(sessionStorage.getItem(storageKey)).toBe(stored);
  delayed.resolve(response(kind === "membership" ? { membership: membership(2, true) } : pointResult())); await flushPromises();
  expect(sessionStorage.getItem(storageKey)).toBe(stored);
  await wrapper!.setProps({ authState: "authenticated" }); await flushPromises();
  expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
  expect(sessionStorage.getItem(storageKey)).toBe(stored);
  route = async () => response({ accounts: [], next_page: null });
  await wrapper!.setProps({ operatorID: otherActor }); await flushPromises();
  expect(sessionStorage.getItem(storageKey)).toBeNull();
  expect(wrapper!.text()).not.toContain("Private reason");
  expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
  expect(requests.filter((item) => /\/(grants|adjustments)$/.test(item.path))).toHaveLength(1);
});

it("membership recovery cannot be downgraded by an earlier search response", async () => {
  const delayed = deferred<Response>();
  sessionStorage.setItem(memberKey, JSON.stringify({ version: 1, operator_id: actor, operation: "membership_grant", resource_id: a, expected_version: 1, idempotency_key: "idem_account_grant_11111111-1111-4111-8111-111111111111" }));
  route = async (path) => path.endsWith("/search") ? delayed.promise : response({ operation: "membership_grant", status: "succeeded", result: { membership: membership(2, true) } });
  await open("membership");
  delayed.resolve(response({ accounts: [memberAccount(a)], next_page: null })); await flushPromises();
  await button("甲同学").trigger("click");
  expect(wrapper!.find('[data-account-membership-detail-state="ready"]').text()).toContain("终身会员");
  expect(wrapper!.text()).toContain("版本 2");
});

it.each(["membership", "points"] as const)("%s never posts again for denied, absent, mismatched or incomplete result reads", async (kind) => {
  const operation = kind === "membership" ? "membership_grant" : "point_adjustment";
  for (const queryResult of [response({}, 403), response({}, 404), response({}, 503), response({ operation: "ticket_reply", status: "succeeded", result: { membership: membership(2, true) } }), response({ operation, status: "succeeded" })]) {
    route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a)], next_page: null })
      : path.includes("/account-lookups") ? response({ account: identity(a) })
      : path.includes("/account/operations/") ? queryResult : Promise.reject(new Error("response lost"));
    await open(kind);
    if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); } else { await lookup(); await confirmPoints(); }
    expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
    expect(sessionStorage.getItem(kind === "membership" ? memberKey : pointKey)).not.toBeNull();
    expect(requests.filter((item) => /\/(grants|adjustments)$/.test(item.path))).toHaveLength(1);
    expect(wrapper!.text()).not.toContain("本次调整后的余额");
    expect(wrapper!.text()).not.toContain("终身会员权益已发放");
    wrapper!.unmount(); wrapper = undefined; sessionStorage.clear(); requests.length = 0;
  }
});

it("points reject unsafe, fractional and zero values before confirmation", async () => {
  await open("points"); await lookup();
  await wrapper!.find("textarea").setValue("Private reason");
  for (const invalid of ["0", "1.5", "1e3", "9007199254740992"]) {
    await wrapper!.find('input[inputmode="numeric"]').setValue(invalid);
    await wrapper!.find("form").trigger("submit");
    expect(wrapper!.find("[data-points-confirm-step]").exists()).toBe(false);
  }
  expect(requests.filter((item) => item.path.endsWith("/adjustments"))).toHaveLength(0);
});

it("membership keeps the conflict reason and requires one new explicit confirmation on the fresh revision", async () => {
  let writes = 0;
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a)], next_page: null })
    : path.endsWith("/grants") ? (++writes, response({}, 409))
    : path.endsWith("/revocations") ? (++writes, response({ membership: membership(3) })) : response({ membership: membership(2, true) });
  await open("membership"); await button("甲同学").trigger("click"); await confirmMember();
  expect(writes).toBe(1);
  expect(wrapper!.text()).toContain("版本 2");
  expect(wrapper!.find("textarea").element).toHaveProperty("value", "Private reason");
  await wrapper!.findAll("form").slice(-1)[0]!.trigger("submit");
  const confirm = button("确认执行"); await confirm.trigger("click"); await confirm.trigger("click"); await flushPromises();
  expect(writes).toBe(2);
  expect(requests.find((item) => item.path.endsWith("/revocations"))?.body).toMatchObject({ expected_version: 2 });
  expect(wrapper!.text()).toContain("终身会员权益已撤销");
});

it.each(["membership", "points"] as const)("%s ignores an old actor's completion while the next actor owns a new pending command", async (kind) => {
  const oldWrite = deferred<Response>(), newWrite = deferred<Response>();
  route = async (path, init) => path.endsWith("/search") ? response({ accounts: [memberAccount(a), memberAccount(b)], next_page: null })
    : path.includes("/account-lookups") ? response({ account: identity((JSON.parse(String(init?.body)) as { email: string }).email === identity(b).email ? b : a) })
    : path.includes("/account/operations/") ? response({ operation: kind === "membership" ? "membership_grant" : "point_adjustment", status: "unknown" })
    : path.includes(a) || (kind === "points" && (JSON.parse(String(init?.body)) as { user_id: string }).user_id === a) ? oldWrite.promise : newWrite.promise;
  await open(kind);
  if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); } else { await lookup(); await confirmPoints(); }
  await wrapper!.setProps({ operatorID: otherActor }); await flushPromises();
  if (kind === "membership") { await button("乙同学").trigger("click"); await confirmMember(); } else { await lookup(b); await confirmPoints(); }
  const storageKey = kind === "membership" ? memberKey : pointKey, stored = sessionStorage.getItem(storageKey)!;
  expect(JSON.parse(stored)).toMatchObject({ operator_id: otherActor, resource_id: b });
  oldWrite.resolve(response(kind === "membership" ? { membership: membership(2, true) } : pointResult())); await flushPromises();
  expect(sessionStorage.getItem(storageKey)).toBe(stored);
  expect(wrapper!.text()).not.toContain("已发放"); expect(wrapper!.text()).not.toContain("积分调整已确认");
  expect(wrapper!.text()).toContain("正在提交");
  newWrite.resolve(response({}, 503)); await flushPromises();
  expect(sessionStorage.getItem(storageKey)).toBe(stored);
});

it.each(["membership", "points"] as const)("%s clears identity UI on a real API 401 while preserving original-key recovery", async (kind) => {
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a)], next_page: null })
    : path.includes("/account-lookups") ? response({ account: identity(a) })
    : path.includes("/account/operations/") ? response({ operation: kind === "membership" ? "membership_grant" : "point_adjustment", status: "unknown" }) : response({}, 401);
  await open(kind);
  if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); } else { await lookup(); await confirmPoints(); }
  expect(wrapper!.text()).not.toContain("甲同学");
  expect(wrapper!.text()).toContain("登录状态已过期");
  const stored = sessionStorage.getItem(kind === "membership" ? memberKey : pointKey)!;
  expect(stored).not.toBeNull();
  await wrapper!.setProps({ authState: "signed_out" }); await wrapper!.setProps({ authState: "authenticated" }); await flushPromises();
  expect(sessionStorage.getItem(kind === "membership" ? memberKey : pointKey)).toBe(stored);
  expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
  expect(requests.filter((item) => /\/(grants|adjustments)$/.test(item.path))).toHaveLength(1);
});

it.each(["membership", "points"] as const)("%s treats missing facts or an undeclared target identity as unknown", async (kind) => {
  for (const malformed of [kind === "membership" ? { membership: { plan: "lifetime", lifetime: true } } : { balance: 12 }, kind === "membership" ? { membership: { ...membership(2, true), user_id: b } } : { ...pointResult(), user_id: b }]) {
    route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a)], next_page: null })
      : path.includes("/account-lookups") ? response({ account: identity(a) })
      : path.includes("/account/operations/") ? response({ operation: kind === "membership" ? "membership_grant" : "point_adjustment", status: "unknown" }) : response(malformed);
    await open(kind);
    if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); } else { await lookup(); await confirmPoints(); }
    expect(requests.filter((item) => item.path.includes("/account/operations/"))).toHaveLength(1);
    expect(sessionStorage.getItem(kind === "membership" ? memberKey : pointKey)).not.toBeNull();
    expect(wrapper!.text()).toContain("结果尚未确认");
    wrapper!.unmount(); wrapper = undefined; sessionStorage.clear(); requests.length = 0;
  }
});

it("membership restores an unfiltered second page and exact target without storing identity or a reason", async () => {
  route = async (_path, init) => {
    const input = JSON.parse(String(init?.body)) as { page: number };
    return response({ accounts: input.page === 2 ? [memberAccount(b)] : Array.from({ length: 20 }, (_, index) => memberAccount(index === 0 ? a : `000000${String(index).padStart(2, "0")}-1111-4111-8111-111111111111`)), next_page: input.page === 1 ? 2 : null });
  };
  await open("membership"); await button("下一页").trigger("click"); await flushPromises(); await button("乙同学").trigger("click");
  await wrapper!.find("textarea").setValue("Private reason"); await button("返回用户列表").trigger("click");
  const stored = sessionStorage.getItem("henukit.console.membership-view.v1")!;
  expect(JSON.parse(stored)).toEqual({ version: 1, operator_id: actor, page: 2, selected_id: b });
  wrapper!.unmount(); wrapper = undefined; await open("membership");
  expect(requests[requests.length - 1]?.body).toEqual({ query: "", page: 2 });
  expect(wrapper!.find('[data-account-membership-detail-state="ready"]').text()).toContain("乙同学");
  expect(wrapper!.find("textarea").element).toHaveProperty("value", "");
});

it.each(["restored A", "manual B"])("membership's pending result synchronizes the trusted revision while preserving %s selection", async (selection) => {
  const delayed = deferred<Response>();
  sessionStorage.setItem(memberKey, JSON.stringify({ version: 1, operator_id: actor, operation: "membership_grant", resource_id: a, expected_version: 1, idempotency_key: "idem_account_grant_11111111-1111-4111-8111-111111111111" }));
  sessionStorage.setItem("henukit.console.membership-view.v1", JSON.stringify({ version: 1, operator_id: actor, page: 1, selected_id: a }));
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a), memberAccount(b)], next_page: null }) : delayed.promise;
  await open("membership");
  if (selection === "manual B") await button("乙同学").trigger("click");
  delayed.resolve(response({ operation: "membership_grant", status: "succeeded", result: { membership: membership(2, true) } })); await flushPromises();
  const selected = wrapper!.find('[data-account-membership-detail-state="ready"]');
  expect(selected.find("h2").text()).toBe(selection === "restored A" ? "甲同学" : "乙同学");
  expect(selected.find("h3").text()).toBe(selection === "restored A" ? "终身会员" : "免费会员");
  expect(selected.text()).toContain(selection === "restored A" ? "版本 2" : "版本 1");
  expect(button("甲同学").text()).toContain("终身会员");
  expect(sessionStorage.getItem(memberKey)).toBeNull();
  expect(requests.some((item) => item.path.endsWith("/grants"))).toBe(false);
});

it.each([503, "unknown"] as const)("membership's historical recovery cannot downgrade selected revision 3 after initial %s", async (initial) => {
  let queries = 0;
  sessionStorage.setItem(memberKey, JSON.stringify({ version: 1, operator_id: actor, operation: "membership_grant", resource_id: a, expected_version: 1, idempotency_key: "idem_account_grant_11111111-1111-4111-8111-111111111111" }));
  sessionStorage.setItem("henukit.console.membership-view.v1", JSON.stringify({ version: 1, operator_id: actor, page: 1, selected_id: a }));
  route = async (path) => path.endsWith("/search") ? response({ accounts: [memberAccount(a, membership(3))], next_page: null })
    : ++queries === 1 ? initial === 503 ? response({}, 503) : response({ operation: "membership_grant", status: "unknown" })
    : response({ operation: "membership_grant", status: "succeeded", result: { membership: membership(2, true) } });
  await open("membership"); await button("查询原操作结果").trigger("click"); await flushPromises();
  const selected = wrapper!.find('[data-account-membership-detail-state="ready"]');
  expect(selected.find("h3").text()).toBe("免费会员");
  expect(selected.text()).toContain("版本 3");
  expect(button("甲同学").text()).toContain("免费会员");
  expect(sessionStorage.getItem(memberKey)).toBeNull();
});

it("points native cancel click does not submit the form or reopen confirmation", async () => {
  attachedHost = document.createElement("div"); document.body.append(attachedHost);
  wrapper = mount(AccountPointOperationsView, { attachTo: attachedHost, props: { authState: "authenticated", operatorID: actor, permissions: ["account.points.adjust"] } });
  await flushPromises(); await lookup();
  await wrapper!.find('input[inputmode="numeric"]').setValue("7");
  await wrapper!.find("textarea").setValue("Private reason");
  let nativeSubmissions = 0;
  wrapper!.find("form").element.addEventListener("submit", () => nativeSubmissions++);
  (button("提交积分调整").element as HTMLButtonElement).click(); await flushPromises();
  expect(wrapper!.find("[data-points-confirm-step]").exists()).toBe(true);
  (button("取消").element as HTMLButtonElement).click(); await flushPromises();
  expect(wrapper!.find("[data-points-confirm-step]").exists()).toBe(false);
  expect(wrapper!.text()).toContain("已取消，未写入任何积分流水");
  expect(nativeSubmissions).toBe(1);
  expect(requests.some((item) => item.path.endsWith("/adjustments"))).toBe(false);
});

it.each(["membership", "points"] as const)("%s remains readable when storage read and cleanup both throw SecurityError, and cannot POST", async (kind) => {
  const realm = Object.getPrototypeOf(window.sessionStorage);
  vi.spyOn(realm, "getItem").mockImplementation(() => { throw new window.DOMException("blocked read", "SecurityError"); });
  vi.spyOn(realm, "removeItem").mockImplementation(() => { throw new window.DOMException("blocked cleanup", "SecurityError"); });
  vi.spyOn(realm, "setItem").mockImplementation(() => { throw new window.DOMException("blocked write", "SecurityError"); });
  await open(kind);
  if (kind === "membership") { await button("甲同学").trigger("click"); await confirmMember(); } else { await lookup(); await confirmPoints(); }
  expect(wrapper!.text()).toContain("甲同学");
  expect(wrapper!.text()).toContain("操作未提交");
  expect(requests.some((item) => /\/(grants|adjustments)$/.test(item.path))).toBe(false);
});
