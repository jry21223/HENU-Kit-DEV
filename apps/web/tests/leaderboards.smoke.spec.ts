import { expect, test } from "@playwright/test";

test.use({ channel: "chrome" });

type Envelope<T> = {
  code: number;
  message: string;
  data: T;
};

type LeaderboardResponse = {
  type: "wiki" | "quiz" | "overall";
  entries: Array<{
    rank: number;
    userId: string;
    name: string;
    role: string;
    score: number;
    metrics: Record<string, unknown>;
  }>;
};

const cfg = {
  enabled: process.env.E2E_LEADERBOARDS_SMOKE === "1",
  webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
  apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
};

test.describe("leaderboards browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_LEADERBOARDS_SMOKE=1 plus E2E_* URLs to run the read-only leaderboards smoke.");

  test("public leaderboard APIs and page are reachable without leaking emails", async ({ page, request }) => {
    for (const type of ["wiki", "quiz", "overall"] as const) {
      const response = await request.get(joinURL(cfg.apiBaseURL, `/leaderboards/${type}?limit=5`));
      expect(response.status()).toBe(200);
      const body = await response.text();
      expect(body).not.toContain("@");
      const payload = JSON.parse(body) as Envelope<LeaderboardResponse>;
      expect(payload.code).toBe(0);
      expect(payload.data.type).toBe(type);
      expect(Array.isArray(payload.data.entries)).toBe(true);
    }

    await page.goto(joinURL(cfg.webBaseURL, "/leaderboards"), { waitUntil: "networkidle" });
    await expect(page.getByText("学习榜单").first()).toBeVisible();
    await expect(page.getByText("综合学习榜").first()).toBeVisible();
    await expect(page.getByText("刷题榜").first()).toBeVisible();
    await expect(page.getByText("Wiki 贡献榜").first()).toBeVisible();
  });
});

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}
