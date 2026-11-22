import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { consoleTheme } from "./theme";

function renderApp(initialEntries: string[] = ["/"]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <ConfigProvider locale={zhCN} theme={consoleTheme}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={initialEntries}>
            <App />
          </MemoryRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>,
  );
}

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
      "library.read",
      "food.read",
      "food.review",
      "quizcraft.workshop.read",
      "account.tickets.read",
      "account.membership.write",
      "account.points.adjust",
    ],
    scopes: [
      { kind: "platform" },
      { kind: "product", product_code: "notice" },
      { kind: "product", product_code: "library" },
      { kind: "product", product_code: "food" },
      { kind: "product", product_code: "quizcraft" },
      { kind: "product", product_code: "account-portfolio" },
    ],
    verified_at: "2026-11-22T04:00:00Z",
  },
  expires_at: "2026-11-22T06:00:00Z",
};

const sampleOverview = {
  generated_at: "2026-11-22T04:00:00Z",
  modules: [
    {
      id: "portal",
      status: "ok",
      status_message: "主站运行正常",
      metrics: [{ label: "版本", value: "v2.4.0" }],
      request_id: "req_portal",
    },
    {
      id: "food",
      status: "ok",
      status_message: "美食服务正常",
      metrics: [],
      request_id: "req_food",
    },
  ],
};

describe("HENUKit Console App shell & auth flows", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("shows Console login CTA with safe return_to when signed out (without false expired notice)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse({ error: { code: "UNAUTHORIZED", message: "unauthorized" } }, 401));
    window.history.replaceState({}, "", "/food");
    renderApp(["/food"]);

    expect(await screen.findByText("请先登录管理后台")).toBeTruthy();
    expect(screen.queryByText(/登录已过期/)).toBeNull();
    const loginLinks = await screen.findAllByRole("link", { name: "登录 Console" });
    expect(loginLinks.length).toBeGreaterThan(0);
    expect(loginLinks[0].getAttribute("href")).toBe("/api/v1/auth/login?return_to=%2Ffood");
  });

  it("shows session expired notice only when an authenticated session later receives 401", async () => {
    let calls = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_sess" });
      if (url.includes("/api/v1/overview")) {
        calls += 1;
        if (calls === 1) return jsonResponse({ error: { code: "UNAUTHORIZED", message: "expired" } }, 401);
      }
      return jsonResponse({ data: {}, request_id: "req_default" });
    });

    renderApp(["/"]);
    expect(await screen.findByText("登录已过期，请重新登录")).toBeTruthy();
  });

  it("renders grouped navigation, overview tasks, and switches routes as SPA without reload", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_sess" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: sampleOverview, request_id: "req_ov" });
      if (url.includes("/api/v1/food/facts")) {
        return jsonResponse({
          data: {
            as_of: "2026-11-22T04:00:00Z",
            scope: "global",
            stale: false,
            counts: { pending_submissions: 3, open_anomalies: 1, pending_tier_adjustments: 0 },
            windows: {
              submissions: { filter: "pending", has_more: false, items: [], limit: 100, returned: 0 },
              anomalies: { filter: "open", has_more: false, items: [], limit: 100, returned: 0 },
              tier_adjustments: { filter: "pending", has_more: false, items: [], limit: 100, returned: 0 },
            },
          },
          request_id: "req_food_facts",
        });
      }
      if (url.includes("/api/v1/account/tickets/facts")) {
        return jsonResponse({
          data: {
            as_of: "2026-11-22T04:00:00Z",
            scope: "global",
            stale: false,
            counts: { unresolved: 2, open: 1, in_progress: 1, resolved: 5 },
            window: { filter: "unresolved", has_more: false, items: [], limit: 100, matched_total: 2, returned: 0 },
          },
          request_id: "req_ticket_facts",
        });
      }
      if (url.includes("/api/v1/library/readiness")) {
        return jsonResponse({
          data: { management: "not_ready", checked_at: "2026-11-22T04:00:00Z" },
          request_id: "req_lib",
        });
      }
      if (url.includes("/api/v1/notices")) {
        return jsonResponse({
          data: { generated_at: "2026-11-22T04:00:00Z", items: [] },
          request_id: "req_notice",
        });
      }
      if (url.includes("/api/v1/operations")) {
        return jsonResponse({
          data: {
            access_context: fullSession.access_context,
            accounts: [],
            sessions: [],
            inbox_items: [],
            audit: [],
            mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 0, failed: 0, dead_letters: 0 },
            pagination: {
              accounts: { page: 1, next_page: null, next_cursor: null },
              sessions: { page: 1, next_page: null, next_cursor: null },
              inbox_items: { page: 1, next_page: null, next_cursor: null },
              audit: { page: 1, next_page: null, next_cursor: null },
            },
            dependencies: { postgres: "ready", redis: "ready" },
            generated_at: "2026-11-22T04:00:00Z",
          },
          request_id: "req_ops",
        });
      }
      return jsonResponse({ data: {}, request_id: "req_default" });
    });

    renderApp(["/"]);

    expect(await screen.findByRole("heading", { name: "工作台总览" })).toBeTruthy();
    expect(await screen.findByText("美食内容审核")).toBeTruthy();
    expect(await screen.findByText("题库工坊")).toBeTruthy();

    // Verify SPA navigation to /library keeps heading ID intact
    const user = userEvent.setup();
    await user.click(screen.getByRole("menuitem", { name: /资料库管理/ }));
    await waitFor(() => {
      expect(document.getElementById("library-heading")?.textContent).toBe("资料库管理");
    });
  });

  it("opens sign-out confirmation modal and logs out after confirmation", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes("/api/v1/session/logout")) return jsonResponse({ data: { ok: true }, request_id: "req_out" });
      if (url.includes("/api/v1/session")) return jsonResponse({ data: fullSession, request_id: "req_sess" });
      if (url.includes("/api/v1/overview")) return jsonResponse({ data: sampleOverview, request_id: "req_ov" });
      return jsonResponse({ error: { code: "UNAVAILABLE", message: "down" } }, 503);
    });

    renderApp(["/"]);
    const signOutBtn = await screen.findByRole("button", { name: /退出登录/ });
    const user = userEvent.setup();
    await user.click(signOutBtn);

    expect(await screen.findByText("确认退出登录？")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "确认退出" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/session/logout",
        expect.objectContaining({ method: "POST" }),
      );
    });
    expect(await screen.findByText("请先登录管理后台")).toBeTruthy();
  });
});
