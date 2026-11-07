import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import PlatformOperationsView from "./components/PlatformOperationsView.vue";

const actor = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const other = "271f1c6f-7b10-4c92-91a2-b39bf5af5302";
const itemID = "371f1c6f-7b10-4c92-91a2-b39bf5af5302";
const resourceID = "88888888-8888-4888-8888-888888888888";
const snapshot = "2026-11-07T12:00:00Z";
const page = (n = 1, cursor = "opaque-next") => ({ page: n, next_page: n + 1, next_cursor: cursor });
function inbox() { return { id: itemID, source_product_code: "quizcraft", source_resource_type: "feedback", source_resource_id: resourceID, source_resource_url: "https://evil.example/wrong", priority: "normal", status: "open", version: 1, created_at: snapshot, updated_at: snapshot }; }
function operations(query = new URLSearchParams(), items = [inbox()]) {
  return { access_context: { permissions: ["platform.operations.read"], scopes: [{ kind: "platform" }], verified_at: snapshot }, accounts: [], sessions: [], audit: [], inbox_items: items,
    mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 0, failed: 0, dead_letters: 0 }, dependencies: { postgres: "ready", redis: "ready" }, generated_at: snapshot,
    pagination: { accounts: page(Number(query.get("accounts_page") ?? 1), "account-opaque"), sessions: page(Number(query.get("sessions_page") ?? 1), "session-opaque"), inbox_items: page(Number(query.get("inbox_page") ?? 1), "inbox-opaque"), audit: page(Number(query.get("audit_page") ?? 1), "audit-opaque") } };
}
function envelope(data: unknown, status = 200) { return new Response(JSON.stringify({ data, request_id: "req_context_test" }), { status, headers: { "Content-Type": "application/json" } }); }
const requests: Array<{ url: URL; method: string; body: unknown }> = [];
let route: (url: URL, init?: RequestInit) => Promise<Response>;
let wrapper: VueWrapper | undefined;
const dialogShow = Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, "showModal");
const dialogClose = Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, "close");
beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value: function(this: HTMLDialogElement) { this.setAttribute("open", ""); } });
  Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value: function(this: HTMLDialogElement) { this.removeAttribute("open"); } });
  window.history.replaceState({}, "", "/operations"); sessionStorage.clear(); requests.length = 0;
  Object.defineProperty(window, "scrollY", { configurable: true, value: 0 });
  vi.stubGlobal("scrollTo", vi.fn());
  route = async (url) => envelope(operations(url.searchParams));
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), window.location.origin); requests.push({ url, method: init?.method ?? "GET", body: init?.body ? JSON.parse(String(init.body)) : undefined }); return route(url, init);
  }));
});
afterEach(() => {
  wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals();
  if (dialogShow) Object.defineProperty(HTMLDialogElement.prototype, "showModal", dialogShow); else delete (HTMLDialogElement.prototype as unknown as Record<string, unknown>).showModal;
  if (dialogClose) Object.defineProperty(HTMLDialogElement.prototype, "close", dialogClose); else delete (HTMLDialogElement.prototype as unknown as Record<string, unknown>).close;
});
async function openPlatform(operatorID = actor) { wrapper = mount(PlatformOperationsView, { props: { authState: "authenticated", operatorID } }); await flushPromises(); }
function button(text: string) { const found = wrapper!.findAll("button").find((value) => value.text() === text); if (!found) throw new Error(`Missing button ${text}`); return found; }

it("restores each opaque cursor independently at the same creation fence after refresh", async () => {
  await openPlatform();
  await button("账户下一页").trigger("click"); await flushPromises();
  await button("收件箱下一页").trigger("click"); await flushPromises();
  wrapper!.unmount(); wrapper = undefined; await openPlatform();
  const last = requests[requests.length - 1]!.url.searchParams;
  expect(last.get("accounts_page")).toBe("2"); expect(last.get("accounts_cursor")).toBe("account-opaque");
  expect(last.get("inbox_page")).toBe("2"); expect(last.get("inbox_cursor")).toBe("inbox-opaque");
  expect(last.get("sessions_page")).toBe("1"); expect(last.get("audit_page")).toBe("1"); expect(last.get("snapshot_at")).toBe(snapshot);
  expect(wrapper!.text()).toContain("本页收件箱");
});

it("does not use an untrusted source URL or invent an unsupported source route", async () => {
  await openPlatform(); expect(wrapper!.html()).not.toContain("evil.example"); expect(wrapper!.text()).toContain("来源尚未接入");
});

it("offers an explicit reset for an expired cursor instead of silently changing the list", async () => {
  await openPlatform(); await button("收件箱下一页").trigger("click"); await flushPromises(); wrapper!.unmount(); wrapper = undefined;
  route = async (url) => url.searchParams.get("inbox_cursor") ? envelope({}, 400) : envelope(operations(url.searchParams));
  await openPlatform(); expect(wrapper!.text()).toContain("分页上下文已失效"); expect(requests[requests.length - 1]!.url.searchParams.get("inbox_page")).toBe("2");
  await button("重新载入第一页").trigger("click"); await flushPromises();
  expect(requests[requests.length - 1]!.url.searchParams.get("inbox_page")).toBe("1"); expect(requests[requests.length - 1]!.url.searchParams.has("snapshot_at")).toBe(false);
});

it("drops a previous actor's late read when the account changes", async () => {
  let resolve!: (value: Response) => void;
  route = async () => new Promise<Response>((done) => { resolve = done; });
  wrapper = mount(PlatformOperationsView, { props: { authState: "authenticated", operatorID: actor } });
  await wrapper.setProps({ authState: "signed_out", operatorID: undefined });
  resolve(envelope(operations())); await flushPromises();
  expect(wrapper.text()).not.toContain(resourceID);
  route = async () => envelope(operations(undefined, [])); await wrapper.setProps({ authState: "authenticated", operatorID: other }); await flushPromises();
  expect(wrapper.text()).not.toContain(resourceID);
  expect(requests[requests.length - 1]!.url.searchParams.get("inbox_page")).toBe("1");
});

it("keeps identity search in its POST body and out of refresh storage", async () => {
  route = async (url) => url.pathname.endsWith("/accounts/search") ? envelope({ accounts: [], next_page: null, next_cursor: null }) : envelope(operations(url.searchParams));
  await openPlatform(); await wrapper!.find('input[placeholder="显示名称或完整邮箱"]').setValue("private@henu.edu.cn"); await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(requests[requests.length - 1]).toMatchObject({ method: "POST", body: { query: "private@henu.edu.cn" } });
  window.dispatchEvent(new Event("pagehide")); expect(JSON.stringify(sessionStorage)).not.toContain("private@henu.edu.cn"); expect(window.location.href).not.toContain("private");
});

it("restores the same actor's non-PII status, selected reference and scroll after relogin and return", async () => {
  wrapper = mount(PlatformOperationsView, { attachTo: document.body, props: { authState: "authenticated", operatorID: actor } }); await flushPromises();
  await wrapper.find('select').setValue("open");
  await wrapper.find(`[data-inbox-id="${itemID}"] button`).trigger("click");
  Object.defineProperty(window, "scrollY", { configurable: true, value: 740 });
  await wrapper.setProps({ authState: "signed_out", operatorID: undefined }); await flushPromises();
  expect(wrapper.find(`[data-inbox-id="${itemID}"]`).exists()).toBe(false);
  await wrapper.setProps({ authState: "authenticated", operatorID: actor }); await flushPromises();
  expect(wrapper.find('select').element).toHaveProperty("value", "open");
  expect(wrapper.find(`[data-inbox-id="${itemID}"]`).attributes("aria-current")).toBe("true");
  expect(document.activeElement).toBe(wrapper.find(`[data-inbox-id="${itemID}"] button`).element);
  expect(window.scrollTo).toHaveBeenCalledWith({ top: 740, behavior: "instant" });
  wrapper.unmount(); wrapper = undefined; await openPlatform();
  expect(wrapper!.find('select').element).toHaveProperty("value", "open"); expect(wrapper!.find(`[data-inbox-id="${itemID}"]`).attributes("aria-current")).toBe("true");
});

it("drops a late identity search and clears its query when another actor signs in", async () => {
  let resolve!: (value: Response) => void;
  route = async (url) => url.pathname.endsWith("/accounts/search") ? new Promise<Response>((done) => { resolve = done; }) : envelope(operations(url.searchParams));
  await openPlatform(); await wrapper!.find('input[placeholder="显示名称或完整邮箱"]').setValue("private@henu.edu.cn"); await wrapper!.find("form").trigger("submit");
  await wrapper!.setProps({ operatorID: other }); await flushPromises();
  resolve(envelope({ accounts: [{ id: actor, display_name: "旧身份私人结果", email: "private@henu.edu.cn", email_verified: true, status: "active", authorization_revision: 1, created_at: snapshot, grants: [] }], next_page: null, next_cursor: null })); await flushPromises();
  expect(wrapper!.text()).not.toContain("旧身份私人结果"); expect(wrapper!.find('input[placeholder="显示名称或完整邮箱"]').element).toHaveProperty("value", ""); expect(JSON.stringify(sessionStorage)).not.toContain("private@henu.edu.cn");
});

it("keeps search text and its cursors transient while restoring other collections after refresh", async () => {
  route = async (url) => url.pathname.endsWith("/accounts/search") ? envelope({ accounts: [], next_page: 2, next_cursor: "private-search-cursor" }) : envelope(operations(url.searchParams));
  await openPlatform(); await button("审计下一页").trigger("click"); await flushPromises();
  await wrapper!.find('input[placeholder="显示名称或完整邮箱"]').setValue("private@henu.edu.cn"); await wrapper!.find("form").trigger("submit"); await flushPromises();
  wrapper!.unmount(); wrapper = undefined; await openPlatform();
  expect(requests[requests.length - 1]!.url.searchParams.get("audit_page")).toBe("2");
  expect(wrapper!.text()).toContain("账户搜索条件未保存，请重新输入"); expect(wrapper!.find('input[placeholder="显示名称或完整邮箱"]').element).toHaveProperty("value", "");
  expect(JSON.stringify(sessionStorage)).not.toContain("private-search-cursor");
});

it("rejects tampered view storage without sending its fields to the server", async () => {
  await openPlatform(); wrapper!.unmount(); wrapper = undefined;
  const key = "henukit.console.platform-view.v1";
  const saved = JSON.parse(sessionStorage.getItem(key)!); saved.context.pages.email = "private@henu.edu.cn";
  sessionStorage.setItem(key, JSON.stringify(saved)); await openPlatform();
  expect(requests[requests.length - 1]!.url.searchParams.get("snapshot_at")).toBeNull(); expect(JSON.stringify(sessionStorage)).not.toContain("private@henu.edu.cn");
});

it("does not persist a late response after the view has unmounted", async () => {
  let resolve!: (value: Response) => void;
  route = async () => new Promise<Response>((done) => { resolve = done; });
  wrapper = mount(PlatformOperationsView, { props: { authState: "authenticated", operatorID: actor } });
  wrapper.unmount(); wrapper = undefined;
  resolve(envelope(operations())); await flushPromises(); expect(sessionStorage.getItem("henukit.console.platform-view.v1")).toBeNull();
});

it("does not silently select another reference when the restored selection is no longer returned", async () => {
  await openPlatform(); await wrapper!.find(`[data-inbox-id="${itemID}"] button`).trigger("click"); wrapper!.unmount(); wrapper = undefined;
  const replacement = { ...inbox(), id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302" };
  route = async () => envelope(operations(undefined, [replacement])); await openPlatform();
  expect(wrapper!.find('[aria-current="true"]').exists()).toBe(false); expect(wrapper!.text()).toContain("先前选中的引用未出现在本页筛选结果中");
});

const sessionA = "51111111-1111-4111-8111-111111111111";
const sessionB = "61111111-1111-4111-8111-111111111111";
const pendingKey = "henukit.console.pending-platform-operation.v1";
function deferredResponse() { let resolve!: (value: Response) => void; const promise = new Promise<Response>((done) => { resolve = done; }); return { promise, resolve }; }
function writableOperations(fence = snapshot, label = "当前账户", role = "operations-operator") {
  const data = operations(); data.access_context.permissions.push("platform.operations.write");
  return { ...data, generated_at: fence, accounts: [{ id: actor, display_name: label, email: "synthetic.operator@henu.edu.cn", email_verified: true, status: "active", authorization_revision: 1, created_at: snapshot, grants: [{ role_code: role, scope: { kind: "platform" } }] }], sessions: [sessionA, sessionB].map((id, index) => ({ id, user_id: actor, display_name: `合成会话 ${index + 1}`, email: "synthetic.operator@henu.edu.cn", kind: "core", last_seen_at: snapshot, expires_at: "2026-11-08T12:00:00Z" })) };
}
function completed(id: string) { return envelope({ operation: "session_revoke", status: "succeeded", resource_id: id }); }
async function revoke(index: number) {
  const controls = wrapper!.findAll("button").filter((value) => value.text() === "撤销登录");
  await controls[index]!.trigger("click"); await flushPromises();
  await button("确认撤销会话").trigger("click"); await flushPromises();
}

it.each(["succeeded", "conflict", "unknown"])("keeps new command B intact when the first of two old A result reads arrives as %s", async (lateState) => {
  const firstLookup = deferredResponse(), bWrite = deferredResponse(); let lookups = 0, posts = 0;
  route = async (url, init) => {
    if (init?.method === "POST") { posts++; return posts === 1 ? envelope({}, 503) : bWrite.promise; }
    if (url.pathname.includes("/results/")) { lookups++; return lookups === 1 ? firstLookup.promise : completed(sessionA); }
    return envelope(writableOperations());
  };
  await openPlatform(); await revoke(0);
  await button("查询结果").trigger("click"); await button("查询结果").trigger("click"); await flushPromises();
  expect(lookups).toBe(2); expect(sessionStorage.getItem(pendingKey)).toBeNull();
  await revoke(1);
  const savedB = sessionStorage.getItem(pendingKey)!; const noticeB = wrapper!.find(".operation-notice").text();
  expect(JSON.parse(savedB)).toMatchObject({ resource_id: sessionB });
  firstLookup.resolve(lateState === "succeeded" ? completed(sessionA) : lateState === "conflict" ? envelope({}, 409) : envelope({ operation: "session_revoke", status: "unknown" })); await flushPromises();
  expect(sessionStorage.getItem(pendingKey)).toBe(savedB); expect(wrapper!.find(".operation-notice").text()).toBe(noticeB);
  expect(button("等待结果").attributes("disabled")).toBeDefined(); expect(posts).toBe(2);
});

it.each(["old-search-first", "fresh-load-first"])("rejects an old-fence search across a completed write (%s)", async (order) => {
  const write = deferredResponse(), oldSearch = deferredResponse(), fresh = deferredResponse(); let reads = 0;
  const newFence = "2026-11-07T12:05:00Z";
  route = async (url, init) => {
    if (url.pathname.endsWith("/accounts/search")) return oldSearch.promise;
    if (init?.method === "POST") return write.promise;
    reads++; return reads === 1 ? envelope(writableOperations()) : fresh.promise;
  };
  await openPlatform(); await revoke(0);
  await wrapper!.find('input[placeholder="显示名称或完整邮箱"]').setValue("old-query"); await wrapper!.find("form").trigger("submit");
  write.resolve(completed(sessionA)); await flushPromises();
  const staleAccount = writableOperations(snapshot, "旧快照搜索账户", "old-role").accounts[0];
  const staleResult = envelope({ accounts: [staleAccount], next_page: 2, next_cursor: "old-query-cursor" });
  const freshData = writableOperations(newFence, "新快照账户", "fresh-role");
  freshData.pagination.accounts.next_cursor = "fresh-account-cursor";
  if (order === "old-search-first") { oldSearch.resolve(staleResult); await flushPromises(); fresh.resolve(envelope(freshData)); }
  else { fresh.resolve(envelope(freshData)); await flushPromises(); expect(button("搜索账户").attributes("disabled")).toBeUndefined(); oldSearch.resolve(staleResult); }
  await flushPromises();
  expect(wrapper!.text()).toContain("新快照账户"); expect(wrapper!.text()).not.toContain("旧快照搜索账户"); expect(wrapper!.find("input[pattern]").element).toHaveProperty("value", "fresh-role");
  expect(button("搜索账户").attributes("disabled")).toBeUndefined();
  await button("账户下一页").trigger("click"); await flushPromises();
  const last = requests[requests.length - 1]!;
  expect(last.method).toBe("GET"); expect(last.url.searchParams.get("accounts_cursor")).toBe("fresh-account-cursor"); expect(last.url.searchParams.get("snapshot_at")).toBe(newFence);
});

it("reissues the last completed search at the new fence and keeps its busy state when an older draft search arrives", async () => {
  const write = deferredResponse(), oldDraft = deferredResponse(), newCommitted = deferredResponse(); let reads = 0, committedSearches = 0;
  const newFence = "2026-11-07T12:05:00Z";
  route = async (url, init) => {
    if (url.pathname.endsWith("/accounts/search")) {
      const body = JSON.parse(String(init?.body));
      if (body.query === "draft-query") return oldDraft.promise;
      committedSearches++; return committedSearches === 1 ? envelope({ accounts: writableOperations().accounts, next_page: 2, next_cursor: "old-committed-cursor" }) : newCommitted.promise;
    }
    if (init?.method === "POST") return write.promise;
    reads++; return envelope(writableOperations(reads === 1 ? snapshot : newFence));
  };
  await openPlatform();
  await wrapper!.find('input[placeholder="显示名称或完整邮箱"]').setValue("committed-query"); await wrapper!.find("form").trigger("submit"); await flushPromises();
  await revoke(0);
  await wrapper!.find('input[placeholder="显示名称或完整邮箱"]').setValue("draft-query"); await wrapper!.find("form").trigger("submit");
  write.resolve(completed(sessionA)); await flushPromises();
  expect(committedSearches).toBe(2);
  expect(requests[requests.length - 1]!.body).toEqual({ query: "committed-query", page: 1, snapshot_at: newFence });
  oldDraft.resolve(envelope({ accounts: writableOperations(snapshot, "旧草稿搜索").accounts, next_page: 2, next_cursor: "old-draft-cursor" })); await flushPromises();
  expect(button("搜索账户").attributes("disabled")).toBeDefined(); expect(wrapper!.text()).not.toContain("旧草稿搜索");
  newCommitted.resolve(envelope({ accounts: writableOperations(newFence, "新快照已提交搜索").accounts, next_page: null, next_cursor: null })); await flushPromises();
  expect(wrapper!.text()).toContain("新快照已提交搜索"); expect(button("搜索账户").attributes("disabled")).toBeUndefined();
});

it.each(["succeeded", "conflict", "unknown"])("ignores A's late POST continuation after a result read completed A and B started (%s)", async (lateState) => {
  const aWrite = deferredResponse(), bWrite = deferredResponse(); let posts = 0;
  route = async (url, init) => {
    if (init?.method === "POST") { posts++; return posts === 1 ? aWrite.promise : bWrite.promise; }
    if (url.pathname.includes("/results/")) return completed(sessionA);
    return envelope(writableOperations());
  };
  await openPlatform(); await revoke(0); await button("查询结果").trigger("click"); await flushPromises();
  await revoke(1);
  const savedB = sessionStorage.getItem(pendingKey)!; const noticeB = wrapper!.find(".operation-notice").text();
  expect(JSON.parse(savedB)).toMatchObject({ resource_id: sessionB });
  aWrite.resolve(lateState === "succeeded" ? completed(sessionA) : lateState === "conflict" ? envelope({}, 409) : envelope({}, 503)); await flushPromises();
  expect(sessionStorage.getItem(pendingKey)).toBe(savedB); expect(wrapper!.find(".operation-notice").text()).toBe(noticeB);
  expect(button("等待结果").attributes("disabled")).toBeDefined(); expect(posts).toBe(2);
});

it.each(["restored-search-unavailable", "fresh-list-unavailable-then-retried"])("keeps canonical cursors out of a failed search restore (%s)", async (failure) => {
  let reads = 0, searches = 0, writes = 0;
  const newFence = "2026-11-07T12:05:00Z";
  route = async (url, init) => {
    if (url.pathname.endsWith("/accounts/search")) {
      searches++;
      return searches === 1 ? envelope({ accounts: writableOperations(snapshot, "旧搜索结果").accounts, next_page: 2, next_cursor: "search-only-cursor" }) : envelope({}, 503);
    }
    if (init?.method === "POST") { writes++; return completed(sessionA); }
    reads++;
    if (reads === 2 && failure === "fresh-list-unavailable-then-retried") return envelope({}, 503);
    const data = writableOperations(url.searchParams.get("snapshot_at") ?? (reads === 1 ? snapshot : newFence), reads === 1 ? "初次普通账户" : "当前普通账户");
    data.pagination.accounts = { page: Number(url.searchParams.get("accounts_page") ?? 1), next_page: 2, next_cursor: "canonical-only-cursor" };
    return envelope(data);
  };
  await openPlatform();
  await wrapper!.find('input[placeholder="显示名称或完整邮箱"]').setValue("previously-completed-query"); await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(wrapper!.text()).toContain("旧搜索结果");
  await revoke(0);
  if (failure === "fresh-list-unavailable-then-retried") {
    expect(wrapper!.text()).toContain("运营数据暂不可用");
    await button("重试").trigger("click"); await flushPromises();
  }
  expect(wrapper!.text()).toContain("当前普通账户");
  expect(wrapper!.find('input[placeholder="显示名称或完整邮箱"]').element).toHaveProperty("value", "previously-completed-query");
  await button("账户下一页").trigger("click"); await flushPromises();
  const last = requests[requests.length - 1]!;
  expect(last.method).toBe("GET"); expect(last.url.pathname).toBe("/api/v1/operations");
  expect(last.url.searchParams.get("accounts_page")).toBe("2"); expect(last.url.searchParams.get("accounts_cursor")).toBe("canonical-only-cursor"); expect(last.url.searchParams.get("snapshot_at")).toBe(newFence);
  expect(searches).toBe(failure === "restored-search-unavailable" ? 2 : 1); expect(writes).toBe(1);
  expect(wrapper!.text()).toContain("当前显示普通账户列表"); expect(wrapper!.text()).toContain("可重新提交搜索");
});

const freshFence = "2026-11-07T12:05:00Z";
function coreLikeOperations(url: URL, clockFence: string) {
  // Core280-285 honors the supplied creation fence; only a request without a
  // snapshot gets the clock fence. Cursor strings remain opaque to the view.
  const fence = url.searchParams.get("snapshot_at") ?? clockFence;
  const data = writableOperations(fence);
  const pagination = Object.fromEntries([ ["accounts", "accounts_page"], ["sessions", "sessions_page"], ["inbox_items", "inbox_page"], ["audit", "audit_page"] ].map(([section, field]) => {
    const value = Number(url.searchParams.get(field!) ?? 1);
    return [section, { page: value, next_page: value < 3 ? value + 1 : null, next_cursor: value < 3 ? `${fence === freshFence ? "fresh" : "old"}-${section}-to-${value + 1}` : null }];
  }));
  return { ...data, pagination, audit: fence === freshFence ? [{ request_id: "req_after_write", actor_user_id: actor, permission_code: "platform.operations.read", target_kind: "platform", decision: "allowed", reason_code: "GRANTED", created_at: freshFence }] : [] };
}
function lastRead() { return requests[requests.length - 1]!.url.searchParams; }

it("retries the failed fresh first-page target after a third-page write and rebuilds usable paging", async () => {
  let writes = 0, failedFresh = false;
  route = async (url, init) => {
    if (init?.method === "POST") { writes++; return completed(sessionA); }
    if (writes && !failedFresh) { failedFresh = true; return envelope({}, 503); }
    return envelope(coreLikeOperations(url, writes ? freshFence : snapshot));
  };
  await openPlatform();
  await button("收件箱下一页").trigger("click"); await flushPromises(); await button("收件箱下一页").trigger("click"); await flushPromises();
  expect(wrapper!.text()).toContain("收件箱第 3 页");
  await revoke(0); expect(wrapper!.text()).toContain("运营数据暂不可用");
  await button("重试").trigger("click"); await flushPromises();
  const retry = lastRead();
  for (const field of ["accounts_page", "sessions_page", "inbox_page", "audit_page"]) expect(retry.get(field)).toBe("1");
  for (const field of ["snapshot_at", "accounts_cursor", "sessions_cursor", "inbox_cursor", "audit_cursor"]) expect(retry.has(field)).toBe(false);
  expect(wrapper!.text()).toContain("req_after_write");
  await button("收件箱下一页").trigger("click"); await flushPromises();
  expect(wrapper!.text()).toContain("收件箱第 2 页"); expect(lastRead().get("inbox_cursor")).toBe("fresh-inbox_items-to-2"); expect(lastRead().get("snapshot_at")).toBe(freshFence);
  await button("收件箱上一页").trigger("click"); await flushPromises();
  expect(wrapper!.text()).toContain("收件箱第 1 页"); expect(lastRead().has("inbox_cursor")).toBe(false); expect(writes).toBe(1);
});

it("retries a failed ordinary page3 request rather than returning to the last successful page2", async () => {
  let failedThird = false;
  route = async (url) => {
    if (url.searchParams.get("sessions_page") === "3" && !failedThird) { failedThird = true; return envelope({}, 503); }
    return envelope(coreLikeOperations(url, snapshot));
  };
  await openPlatform(); await button("会话下一页").trigger("click"); await flushPromises(); await button("会话下一页").trigger("click"); await flushPromises();
  expect(wrapper!.text()).toContain("运营数据暂不可用");
  await button("重试").trigger("click"); await flushPromises();
  expect(lastRead().get("sessions_page")).toBe("3"); expect(lastRead().get("sessions_cursor")).toBe("old-sessions-to-3"); expect(lastRead().get("snapshot_at")).toBe(snapshot);
  expect(wrapper!.text()).toContain("会话第 3 页");
  await button("会话上一页").trigger("click"); await flushPromises();
  expect(wrapper!.text()).toContain("会话第 2 页"); expect(lastRead().get("sessions_cursor")).toBe("old-sessions-to-2");
  expect(requests.filter((request) => request.method === "POST")).toHaveLength(0);
});

it("clears a failed read target on actor change and rejects the old target's late retry", async () => {
  const lateRetry = deferredResponse(); let thirdAttempts = 0, changedActor = false;
  route = async (url) => {
    if (!changedActor && url.searchParams.get("audit_page") === "3") {
      thirdAttempts++; return thirdAttempts === 1 ? envelope({}, 503) : lateRetry.promise;
    }
    return envelope(coreLikeOperations(url, changedActor ? freshFence : snapshot));
  };
  await openPlatform(); await button("审计下一页").trigger("click"); await flushPromises(); await button("审计下一页").trigger("click"); await flushPromises();
  await button("重试").trigger("click");
  changedActor = true; await wrapper!.setProps({ operatorID: other }); await flushPromises();
  for (const field of ["accounts_page", "sessions_page", "inbox_page", "audit_page"]) expect(lastRead().get(field)).toBe("1");
  expect(lastRead().has("snapshot_at")).toBe(false); expect(lastRead().has("audit_cursor")).toBe(false);
  const newActorRead = requests[requests.length - 1]!.url;
  lateRetry.resolve(envelope(coreLikeOperations(new URL("/api/v1/operations?audit_page=3&snapshot_at=" + encodeURIComponent(snapshot), window.location.origin), snapshot))); await flushPromises();
  expect(wrapper!.text()).toContain("审计第 1 页"); expect(wrapper!.text()).not.toContain("审计第 3 页"); expect(requests[requests.length - 1]!.url).toBe(newActorRead);
});
