import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { consoleTheme } from "./theme";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const fullSession = {
  user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
  access_context: {
    permissions: [
      "console.overview.read",
      "platform.operations.read",
      "platform.operations.write",
      "notice.read",
      "notice.manage",
      "notice.review",
      "notice.distribute",
      "library.read",
      "library.manage",
      "library.review",
      "career.read",
      "career.manage",
      "campus.read",
      "campus.moderate",
      "food.read",
      "food.review",
      "food.anomaly",
      "food.tier_adjust",
      "quizcraft.workshop.read",
      "quizcraft.workshop.write",
      "quizcraft.workshop.publish",
      "account.tickets.read",
      "account.tickets.reply",
      "account.tickets.transition",
      "account.membership.write",
      "account.orders.read",
      "account.orders.close",
      "account.orders.refund",
      "account.points.adjust",
    ],
    scopes: [
      { kind: "platform" },
      { kind: "product", product_code: "notice" },
      { kind: "product", product_code: "library" },
      { kind: "product", product_code: "career" },
      { kind: "product", product_code: "campus" },
      { kind: "product", product_code: "food" },
      { kind: "product", product_code: "quizcraft" },
      { kind: "product", product_code: "account-portfolio" },
    ],
    verified_at: "2026-11-22T04:00:00Z",
  },
  expires_at: "2026-11-22T06:00:00Z",
};

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <ConfigProvider locale={zhCN} theme={{ ...consoleTheme, token: { ...consoleTheme.token, motion: false } }}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={[path]}>
            <App />
          </MemoryRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>,
  );
}

describe("Console module pages", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("opens the native Library workspace and submits a versioned course update", async () => {
    const writes: { body: unknown; key: string | null }[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const pathname = String(input);
      if (pathname.endsWith("/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (pathname.endsWith("/readiness")) return jsonResponse({ data: { management: "ready", reason: "native_catalog_ready", capabilities: { workspace: true, commands: true, operation_results: true, courses_write: true }, checked_at: fullSession.expires_at }, request_id: "req_readiness" });
      if (pathname.endsWith("/library")) return jsonResponse({ data: { degraded: false, courses: [{ id: "course-one", code: "MATH", name: "离散数学", department: "数学", status: "active", version: 3 }], submissions: [], materials: [], corrections: [] }, request_id: "req_library" });
      if (pathname.endsWith("/commands")) {
        writes.push({ body: JSON.parse(String(init?.body)), key: new Headers(init?.headers).get("Idempotency-Key") });
        return jsonResponse({ data: { state: "succeeded", version: 4 }, request_id: "req_write" });
      }
      return jsonResponse({}, 404);
    });
    renderAt("/library");
    const releaseTab = await screen.findByText("公开版本与下载统计");
    expect(releaseTab.closest('[role="tab"]')).toBeTruthy();
    fireEvent.click(screen.getByText("编辑课程", { selector: "button span" }));
    fireEvent.change(screen.getByLabelText("课程名称"), { target: { value: "离散数学基础" } });
    fireEvent.click(screen.getByText(/^保\s*存$/, { selector: "button span" }));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({ key: expect.any(String), body: { kind: "course_update", resource_id: "course-one", expected_version: "3", payload: { name: "离散数学基础" } } });
  }, 30000);

  it("monitors Career while the source is paused and confirms a digest retry", async () => {
    const writes: (string | null)[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const pathname = String(input);
      if (pathname.endsWith("/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (pathname.endsWith("/retry-digest")) {
        writes.push(new Headers(init?.headers).get("Idempotency-Key"));
        return jsonResponse({ data: { status: "retry" }, request_id: "req_retry" });
      }
      if (pathname.endsWith("/career/workspace")) return jsonResponse({ data: {
        job_source_enabled: false, counts: { profiles: 2, searches_queued: 0, searches_running: 0, searches_completed: 1, searches_failed: 0, digests_pending: 1 },
        searches: [], extractions: [{ id: "extract-one", user_id: fullSession.user.id, status: "completed", error_code: "", error_message: "", created_at: fullSession.expires_at, completed_at: fullSession.expires_at, failed_at: null }],
        digest_deliveries: [{ search_id: "scan-one", status: "retry", updated_at: fullSession.expires_at }],
      }, request_id: "req_career" });
      return jsonResponse({}, 404);
    });
    renderAt("/career");
    expect(await screen.findByText("岗位数据源当前暂停，仍可查看历史任务。")).toBeTruthy();
    const extractionTab = screen.getByText("简历解析任务");
    expect(extractionTab.closest('[role="tab"]')).toBeTruthy();
    fireEvent.click(extractionTab);
    expect(screen.queryByText("私人简历.pdf")).toBeNull();
    const digestTab = screen.getByText("邮件简报投递");
    expect(digestTab.closest('[role="tab"]')).toBeTruthy();
    fireEvent.click(digestTab);
    fireEvent.click(await screen.findByText("重新投递", { selector: "button span" }));
    expect(writes).toHaveLength(0);
    fireEvent.click(await screen.findByText("确认投递", { selector: "button span" }));
    await waitFor(() => expect(writes).toEqual([expect.any(String)]));
  }, 30000);

  it("governs Campus items by version and does not render private snapshots", async () => {
    const writes: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const pathname = String(input);
      if (pathname.endsWith("/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (pathname.endsWith("/items/item-one/moderate")) {
        writes.push(JSON.parse(String(init?.body)));
        return jsonResponse({ data: { id: "item-one", status: "hidden", version: 8 }, request_id: "req_moderate" });
      }
      if (pathname.endsWith("/campus/workspace")) return jsonResponse({ data: {
        counts: { open: 1, ongoing: 0, done: 0, hidden: 0, exchanges_total: 2, exchanges_accepted: 1 },
        items: [{ id: "item-one", type: "help", title: "代取教材", desc: "明伦校区", status: "open", price: 0, version: 7, owner_snapshot: { phone: "10012345678" } }],
        messages: [],
      }, request_id: "req_campus" });
      return jsonResponse({}, 404);
    });
    renderAt("/campus");
    const workspace = within(await screen.findByRole("region", { name: "校园互助治理" }));
    fireEvent.click(await workspace.findByText("查看单子"));
    expect(screen.queryByText("10012345678")).toBeNull();
    fireEvent.click(await screen.findByText("隐藏单子", { selector: "button span" }));
    const confirmation = await screen.findByText(/^确\s*认$/, { selector: "button span" });
    fireEvent.click(confirmation);
    await waitFor(() => expect(writes).toEqual([{ action: "hide", expected_version: 7 }]));
  }, 30000);

  it("renders FoodPage with tabs, search, and approval modal sending Idempotency-Key", async () => {
    const commandCalls: Array<{ key: string | null; body: unknown }> = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/food/commands")) {
        const headers = new Headers(init?.headers);
        commandCalls.push({
          key: headers.get("Idempotency-Key"),
          body: JSON.parse(String(init?.body ?? "{}")),
        });
        return jsonResponse({
          data: {
            operation: "submission_approve",
            resource_id: "11111111-1111-1111-1111-111111111111",
            state: "succeeded",
            version: 2,
          },
          request_id: "req_cmd_ok",
        });
      }
      if (url.includes("/api/v1/food")) {
        return jsonResponse({
          data: {
            as_of: "2026-11-22T04:00:00Z",
            stale: false,
            status: "ok",
            status_message: "ok",
            submissions: [
              {
                id: "11111111-1111-1111-1111-111111111111",
                venue_name: "南门牛肉面",
                item_name: "红烧牛肉面",
                description: "汤鲜肉足",
                campus: "minglun",
                status: "pending",
                version: 1,
                submitted_at: "2026-11-22T03:00:00Z",
                updated_at: "2026-11-22T03:00:00Z",
              },
            ],
            posts: [],
            anomaly_tickets: [],
            tier_adjustments: [],
          },
          request_id: "req_food_ws",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/food");
    expect(await screen.findByText("南门牛肉面")).toBeTruthy();

    fireEvent.click(screen.getByText(/批\s*准/).closest("button")!);
    expect(await screen.findByText("批准投稿上架")).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText(/信息核实无误/), {
      target: { value: "核实店铺真实存在" },
    });
    fireEvent.click(screen.getByText("确认批准").closest("button")!);

    await waitFor(() => {
      expect(commandCalls.length).toBe(1);
    });
    expect(commandCalls[0].key).toMatch(/^idem_food_submission_approve_/);
  });

  it("renders PracticeWorkshopPage catalog and gracefully handles disabled bridge", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/quizcraft/workshop/banks/") && url.includes("/versions/")) {
        return jsonResponse({
          data: {
            id: "33333333-3333-4333-8333-333333333333",
            bank_id: "22222222-2222-2222-2222-222222222222",
            version_number: 1,
            state: "published",
            questions: [
              {
                source_question_id: "q-judge-1",
                type: "judge",
                content: "TCP 是面向连接的可靠传输协议。",
                answer: true,
              },
            ],
          },
          request_id: "r_qc_ver",
        });
      }
      if (url.includes("/api/v1/quizcraft/workshop/catalog")) {
        return jsonResponse(
          { error: { code: "QUIZCRAFT_CONSOLE_DISABLED", message: "disabled" }, request_id: "r_qc" },
          503,
        );
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/practice");
    expect(await screen.findByText("题库管理后台尚未开启")).toBeTruthy();
    expect(screen.queryByText(/QUIZCRAFT_CONSOLE_ENABLED/)).toBeNull();
  });

  it("renders PracticeWorkshopPage with banks when bridge is enabled", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/quizcraft/workshop/banks/") && url.includes("/versions/")) {
        return jsonResponse({
          data: {
            id: "33333333-3333-4333-8333-333333333333",
            bank_id: "22222222-2222-2222-2222-222222222222",
            version_number: 1,
            state: "published",
            questions: [
              {
                source_question_id: "q-judge-1",
                type: "judge",
                content: "TCP 是面向连接的可靠传输协议。",
                answer: true,
              },
            ],
          },
          request_id: "r_qc_ver",
        });
      }
      if (url.includes("/api/v1/quizcraft/workshop/catalog")) {
        return jsonResponse({
          data: {
            banks: [
              {
                id: "22222222-2222-2222-2222-222222222222",
                slug: "computer-networks",
                name: "计算机网络题库",
                version: 2,
                active_version_number: 1,
                updated_at: "2026-11-22T03:00:00Z",
                versions: [
                  {
                    id: "33333333-3333-3333-3333-333333333333",
                    bank_id: "22222222-2222-2222-2222-222222222222",
                    version_number: 1,
                    state: "published",
                    question_count: 120,
                    created_at: "2026-11-22T02:00:00Z",
                  },
                ],
              },
            ],
          },
          request_id: "r_qc_ok",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/practice");
    expect(await screen.findByText("计算机网络题库")).toBeTruthy();
    expect(await screen.findByText("120 题")).toBeTruthy();

    fireEvent.click(screen.getByText("审阅题目").closest("button")!);
    expect(await screen.findByText("参考答案：正确")).toBeTruthy();

    fireEvent.click(screen.getByText("导入新版本").closest("button")!);
    expect(await screen.findByText("选择 JSON 题库文件")).toBeTruthy();
  });

  it("normalizes real QuizCraft V2 catalog & version detail fields (bank_id, bank_version_id, multi, analysis)", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (
        url.includes(
          "/api/v1/quizcraft/workshop/banks/d1111111-1111-4111-8111-111111111101/versions/d2222222-2222-4222-8222-222222222201",
        )
      ) {
        return jsonResponse({
          data: {
            bank_id: "d1111111-1111-4111-8111-111111111101",
            bank_version_id: "d2222222-2222-4222-8222-222222222201",
            state: "validated",
            questions: [
              {
                question_id: "d3333333-3333-4333-8333-333333333301",
                source_question_id: "cn-q01",
                type: "single",
                chapter: "第五章 运输层",
                content: "SYN 报文段确认号是多少？",
                options: ["100", "101", "102", "随机"],
                answer: 1,
                analysis: "SYN 消耗一个序号，因此为 `101`。",
              },
              {
                question_id: "d3333333-3333-4333-8333-333333333303",
                source_question_id: "cn-q03",
                type: "multi",
                chapter: "第五章 运输层",
                content: "关于 TCP 拥塞控制说法正确的有？",
                options: ["慢开始翻倍", "超时减半", "快重传", "等价流量控制"],
                answer: [0, 1, 2],
                analysis: "流量控制与拥塞控制不等价。",
              },
            ],
          },
          request_id: "r_qc_v2_ver",
        });
      }
      if (url.includes("/api/v1/quizcraft/workshop/catalog")) {
        return jsonResponse({
          data: {
            banks: [
              {
                bank_id: "d1111111-1111-4111-8111-111111111101",
                bank_key: "computer-networks-2026",
                name: "计算机网络期末核心题库（2026）",
                lifecycle_version: 2,
                active_version_id: "d2222222-2222-4222-8222-222222222201",
                versions: [
                  {
                    bank_version_id: "d2222222-2222-4222-8222-222222222201",
                    question_count: 3,
                    state: "validated",
                    active: true,
                    validated_at: "2026-11-22T02:00:00Z",
                  },
                ],
              },
            ],
          },
          request_id: "r_qc_v2_ok",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/practice");
    expect(await screen.findByText("计算机网络期末核心题库（2026）")).toBeTruthy();
    expect(await screen.findByText("v1 已上架")).toBeTruthy();
    expect(await screen.findByText("3 题")).toBeTruthy();

    fireEvent.click(screen.getByText("审阅题目").closest("button")!);
    expect(await screen.findByText("参考答案：B")).toBeTruthy();
    expect(await screen.findByText("参考答案：A、B、C")).toBeTruthy();
    expect(await screen.findByText("多选")).toBeTruthy();
    expect(await screen.findByText(/流量控制与拥塞控制不等价/)).toBeTruthy();
  });

  it("renders AccountPointsPage with human-friendly lookup and quick amount buttons", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/operations/account-lookups")) {
        return jsonResponse({
          data: {
            account: {
              id: "44444444-4444-4444-4444-444444444444",
              display_name: "李同学",
              email: "li@henu.edu.cn",
              status: "active",
            },
          },
          request_id: "req_lookup_ok",
        });
      }
      if (url.includes("/api/v1/account/points/44444444-4444-4444-4444-444444444444")) {
        return jsonResponse({
          data: {
            balance: 320,
            entries: [
              {
                id: "99999999-9999-4999-8999-999999999991",
                amount: 50,
                reason: "首次投稿审核通过奖励",
                created_at: "2026-11-21T08:00:00Z",
              },
            ],
          },
          request_id: "req_points_detail",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/account/points");
    expect(await screen.findByText("积分调整", { selector: "h1" })).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText("student@henu.edu.cn"), {
      target: { value: "li@henu.edu.cn" },
    });
    fireEvent.click(screen.getByText(/^查\s*找$/).closest("button")!);

    expect((await screen.findAllByText(/李同学/)).length).toBeGreaterThan(0);
    expect(await screen.findByText("320")).toBeTruthy();
    expect(await screen.findByText("首次投稿审核通过奖励")).toBeTruthy();
    expect(screen.getAllByText("+50").length).toBeGreaterThanOrEqual(2);
  });

  it("renders AccountTicketsPage queue and opens detail conversation", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/account/tickets/facts")) {
        return jsonResponse({
          data: {
            as_of: "2026-11-22T04:00:00Z",
            scope: "global",
            stale: false,
            counts: { unresolved: 1, open: 1, in_progress: 0, resolved: 0 },
            window: {
              filter: "unresolved",
              limit: 100,
              matched_total: 1,
              returned: 1,
              has_more: false,
              items: [
                {
                  id: "88888888-8888-4888-8888-888888888888",
                  reference: "HKT-88888888-8888-4888-8888-888888888888",
                  title: "刷题记录同步异常",
                  status: "open",
                  version: 1,
                  updated_at: "2026-11-22T03:00:00Z",
                },
              ],
            },
          },
          request_id: "req_tk_facts",
        });
      }
      if (url.includes("/api/v1/account/tickets/88888888-8888-4888-8888-888888888888")) {
        return jsonResponse({
          data: {
            ticket: {
              id: "88888888-8888-4888-8888-888888888888",
              reference: "HKT-88888888-8888-4888-8888-888888888888",
              display_name: "王同学",
              email: "wang@henu.edu.cn",
              title: "刷题记录同步异常",
              category: "practice",
              status: "open",
              version: 1,
              created_at: "2026-11-22T03:00:00Z",
              updated_at: "2026-11-22T03:00:00Z",
            },
            messages: [
              {
                id: "99999999-9999-4999-8999-999999999999",
                author_kind: "user",
                body: "请问昨天做的题目记录为什么没看到？",
                created_at: "2026-11-22T03:00:00Z",
              },
            ],
            events: [],
          },
          request_id: "req_tk_detail",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/account");
    const item = await screen.findByText("刷题记录同步异常");
    fireEvent.click(item);
    expect(await screen.findByText("请问昨天做的题目记录为什么没看到？")).toBeTruthy();
  });

  it("renders AccountMembershipsPage table and opens management drawer", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/account/memberships/search")) {
        return jsonResponse({
          data: {
            accounts: [
              {
                id: "77777777-7777-4777-8777-777777777777",
                display_name: "赵同学",
                email: "zhao@henu.edu.cn",
                status: "active",
                membership: { plan: "free", lifetime: false, version: 1 },
              },
            ],
            next_page: null,
          },
          request_id: "req_mem_search",
        });
      }
      if (url.includes("/api/v1/account/membership-orders")) {
        return jsonResponse({
          data: {
            orders: [
              {
                id: "9f1c2a44-6f3d-4c2b-9a71-2b6d5e8c4a10",
                user_id: "77777777-7777-4777-8777-777777777777",
                display_name: "赵同学",
                email: "zhao@henu.edu.cn",
                plan: "lifetime",
                amount_cents: 1990,
                status: "paid",
                version: 2,
                created_at: "2026-11-22T01:00:00Z",
                updated_at: "2026-11-22T01:01:00Z",
              },
            ],
          },
          request_id: "req_orders_list",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/account/memberships");
    expect(await screen.findByText("赵同学")).toBeTruthy();
    fireEvent.click(screen.getByText("管理权益").closest("button")!);
    expect(await screen.findByText("管理会员权益")).toBeTruthy();

    fireEvent.click(screen.getByText(/会员订单与退款/));
    expect(await screen.findByText("¥19.90")).toBeTruthy();
  });

  it("renders PlatformPage tabs and opens access drawer with preset role selector", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/operations")) {
        return jsonResponse({
          data: {
            access_context: fullSession.access_context,
            accounts: [
              {
                id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
                display_name: "张老师",
                email: "zhang@henu.edu.cn",
                email_verified: true,
                status: "active",
                authorization_revision: 1,
                created_at: "2026-11-22T00:00:00Z",
                grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }],
              },
            ],
            sessions: [],
            inbox_items: [
              {
                id: "11111111-1111-4111-8111-111111111199",
                priority: "high",
                source_product_code: "quizcraft",
                source_resource_type: "feedback",
                source_resource_id: "fb-cn-2026-001",
                status: "open",
                version: 1,
                created_at: "2026-11-22T02:00:00Z",
                updated_at: "2026-11-22T02:00:00Z",
              },
            ],
            audit: [
              {
                actor_user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
                display_name: "张老师",
                email: "zhang@henu.edu.cn",
                permission_code: "platform.operations.read",
                target_kind: "platform",
                decision: "allowed",
                reason_code: "role_grant_matched",
                request_id: "req_audit_1",
                created_at: "2026-11-22T03:30:00Z",
              },
            ],
            mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 5, failed: 0, dead_letters: 0 },
            pagination: {
              accounts: { page: 1, next_page: null, next_cursor: null },
              sessions: { page: 1, next_page: null, next_cursor: null },
              inbox_items: { page: 1, next_page: null, next_cursor: null },
              audit: { page: 1, next_page: null, next_cursor: null },
            },
            dependencies: { postgres: "ready", redis: "ready" },
            generated_at: "2026-11-22T04:00:00Z",
          },
          request_id: "req_ops_ok",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/operations");
    expect(await screen.findByText("张老师")).toBeTruthy();

    fireEvent.click(screen.getByText("管理权限").closest("button")!);
    expect(await screen.findByText("管理权限：张老师")).toBeTruthy();

    fireEvent.click(screen.getByText(/授权审计/));
    expect(await screen.findByText("角色授权校验通过")).toBeTruthy();
    expect(screen.queryByText("role_grant_matched")).toBeNull();

    fireEvent.click(screen.getByText(/跨产品待办/));
    expect(await screen.findByText("刷题练习")).toBeTruthy();
    expect(await screen.findByText("题目纠错反馈")).toBeTruthy();
  });

  it("archives a resolved inbox item as cancelled and keeps PlatformPage valid on reload", async () => {
    let archived = false;
    const inboxUpdates: unknown[] = [];
    const buildOps = () => ({
      access_context: fullSession.access_context,
      accounts: [
        {
          id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
          display_name: "张老师",
          email: "zhang@henu.edu.cn",
          email_verified: true,
          status: "active",
          authorization_revision: 1,
          created_at: "2026-11-22T00:00:00Z",
          grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }],
        },
      ],
      sessions: [],
      inbox_items: [
        {
          id: "11111111-1111-4111-8111-111111111199",
          priority: "high",
          source_product_code: "quizcraft",
          source_resource_type: "feedback",
          source_resource_id: "fb-archive-001",
          status: archived ? "cancelled" : "resolved",
          version: archived ? 2 : 1,
          created_at: "2026-11-22T02:00:00Z",
          updated_at: "2026-11-22T02:00:00Z",
        },
      ],
      audit: [],
      mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 5, failed: 0, dead_letters: 0 },
      pagination: {
        accounts: { page: 1, next_page: null, next_cursor: null },
        sessions: { page: 1, next_page: null, next_cursor: null },
        inbox_items: { page: 1, next_page: null, next_cursor: null },
        audit: { page: 1, next_page: null, next_cursor: null },
      },
      dependencies: { postgres: "ready", redis: "ready" },
      generated_at: "2026-11-22T04:00:00Z",
    });

    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname + input.search : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/operations/roles")) return jsonResponse({ data: { roles: [] }, request_id: "req_roles" });
      if (url.includes("/api/v1/operations/inbox/11111111-1111-4111-8111-111111111199/updates")) {
        const body = JSON.parse(String(init?.body ?? "{}"));
        inboxUpdates.push(body);
        archived = true;
        return jsonResponse({
          data: {
            id: "11111111-1111-4111-8111-111111111199",
            priority: "high",
            source_product_code: "quizcraft",
            source_resource_type: "feedback",
            source_resource_id: "fb-archive-001",
            status: "cancelled",
            version: 2,
            created_at: "2026-11-22T02:00:00Z",
            updated_at: "2026-11-22T02:05:00Z",
          },
          request_id: "req_archive_ok",
        });
      }
      if (url.includes("/api/v1/operations")) {
        return jsonResponse({ data: buildOps(), request_id: "req_ops_archive" });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/operations");
    expect(await screen.findByText("张老师")).toBeTruthy();

    fireEvent.click(screen.getByText(/跨产品待办/));
    expect(await screen.findByText("fb-archive-001")).toBeTruthy();

    fireEvent.click(screen.getByText(/^归\s*档$/).closest("button")!);
    await waitFor(() => expect(inboxUpdates).toHaveLength(1));
    expect(inboxUpdates[0]).toMatchObject({ expected_version: 1, status: "cancelled" });
    expect(await screen.findByText("已归档")).toBeTruthy();

    // 刷新工作台重新校验包含 cancelled 状态的完整快照：不应触发 invalid / 不可用态
    fireEvent.click(screen.getByText(/^刷\s*新$/).closest("button")!);
    expect(await screen.findByText("fb-archive-001")).toBeTruthy();
    expect(await screen.findByText("已归档")).toBeTruthy();
  });

  it("paginates PlatformPage inbox_items via inbox_page cursor and resets filter on refresh", async () => {
    const operationUrls: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname + input.search : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/operations/roles")) return jsonResponse({ data: { roles: [] }, request_id: "req_roles" });
      if (url.includes("/api/v1/operations")) {
        operationUrls.push(url);
        const parsed = new URL(url, "https://console.henukit.test");
        const inboxPage = Number(parsed.searchParams.get("inbox_page") ?? "1");
        const isPage2 = inboxPage === 2 && parsed.searchParams.get("inbox_cursor") === "cursor_inbox_p2";
        return jsonResponse({
          data: {
            access_context: fullSession.access_context,
            accounts: [],
            sessions: [],
            inbox_items: [
              isPage2
                ? {
                    id: "22222222-2222-4222-8222-222222222202",
                    priority: "normal",
                    source_product_code: "food",
                    source_resource_type: "anomaly_ticket",
                    source_resource_id: "inbox-item-page-2",
                    status: "open",
                    version: 1,
                    created_at: "2026-11-21T02:00:00Z",
                    updated_at: "2026-11-21T02:00:00Z",
                  }
                : {
                    id: "11111111-1111-4111-8111-111111111101",
                    priority: "high",
                    source_product_code: "quizcraft",
                    source_resource_type: "feedback",
                    source_resource_id: "inbox-item-page-1",
                    status: "open",
                    version: 1,
                    created_at: "2026-11-22T02:00:00Z",
                    updated_at: "2026-11-22T02:00:00Z",
                  },
            ],
            audit: [],
            mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 5, failed: 0, dead_letters: 0 },
            pagination: {
              accounts: { page: 1, next_page: null, next_cursor: null },
              sessions: { page: 1, next_page: null, next_cursor: null },
              inbox_items: isPage2
                ? { page: 2, next_page: null, next_cursor: null }
                : { page: 1, next_page: 2, next_cursor: "cursor_inbox_p2" },
              audit: { page: 1, next_page: null, next_cursor: null },
            },
            dependencies: { postgres: "ready", redis: "ready" },
            generated_at: "2026-11-22T04:00:00Z",
          },
          request_id: "req_ops_paged",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/operations");
    fireEvent.click(await screen.findByText(/跨产品待办/));
    expect(await screen.findByText("inbox-item-page-1")).toBeTruthy();

    const pager = screen.getByTestId("inbox-pagination");
    expect(within(pager).getByText("第 1 页")).toBeTruthy();
    expect(within(pager).getByText("上一页").closest("button")).toHaveProperty("disabled", true);
    const nextBtn = within(pager).getByText("下一页").closest("button")!;
    expect(nextBtn.disabled).toBe(false);

    fireEvent.click(nextBtn);
    expect(await screen.findByText("inbox-item-page-2")).toBeTruthy();
    expect(within(screen.getByTestId("inbox-pagination")).getByText("第 2 页")).toBeTruthy();
    expect(
      operationUrls.some((u) => u.includes("inbox_page=2") && u.includes("inbox_cursor=cursor_inbox_p2")),
    ).toBe(true);

    fireEvent.click(within(screen.getByTestId("inbox-pagination")).getByText("上一页").closest("button")!);
    expect(await screen.findByText("inbox-item-page-1")).toBeTruthy();
    expect(within(screen.getByTestId("inbox-pagination")).getByText("第 1 页")).toBeTruthy();
    const lastUrl = operationUrls[operationUrls.length - 1] ?? "";
    expect(lastUrl.includes("inbox_page=1") && !lastUrl.includes("inbox_cursor=")).toBe(true);

    // 翻到第 2 页后点刷新，页码复位到第 1 页
    fireEvent.click(within(screen.getByTestId("inbox-pagination")).getByText("下一页").closest("button")!);
    expect(await screen.findByText("inbox-item-page-2")).toBeTruthy();
    fireEvent.click(screen.getByText(/^刷\s*新$/).closest("button")!);
    expect(await screen.findByText("inbox-item-page-1")).toBeTruthy();
    expect(within(screen.getByTestId("inbox-pagination")).getByText("第 1 页")).toBeTruthy();
  });

  it("renders NoticePage with status segmented filter and source selection", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_session" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: { generated_at: "2026-11-22T04:00:00Z", modules: [] }, request_id: "req_overview" });
      if (url.includes("/api/v1/notices/sources")) {
        return jsonResponse({
          data: {
            sources: [
              {
                id: "66666666-6666-4666-8666-666666666666",
                code: "henu-jwc",
                name: "教务处",
                canonical_url: "https://jwc.henu.edu.cn",
                created_at: "2026-11-20T00:00:00Z",
              },
            ],
          },
          request_id: "req_notice_sources",
        });
      }
      if (url.includes("/api/v1/notices")) {
        return jsonResponse({
          data: {
            generated_at: "2026-11-22T04:00:00Z",
            items: [
              {
                id: "55555555-5555-4555-8555-555555555555",
                source: { id: "66666666-6666-4666-8666-666666666666", code: "henu-jwc", name: "教务处" },
                version: 1,
                revision: 1,
                title: "2026秋季期末考试安排通知",
                body: "请各学院同学按时查看考场安排。",
                content_hash: "a".repeat(64),
                source_url: "https://jwc.henu.edu.cn/notice/1",
                state: "pending_review",
                distribution_count: 0,
                created_at: "2026-11-22T03:00:00Z",
              },
            ],
          },
          request_id: "req_notice_ok",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_fallback" });
    });

    renderAt("/notices");
    expect(await screen.findByText("2026秋季期末考试安排通知")).toBeTruthy();

    fireEvent.click(screen.getByText(/通知来源管理/));
    expect(await screen.findByText("https://jwc.henu.edu.cn")).toBeTruthy();
  });
});
