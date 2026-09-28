import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * 用户可见的错误提示只说发生了什么、可以怎么做（#533）：中文，不带接口路径、
 * HTTP 状态文本或内部组件名。错误对象本身的 message 仍保留诊断细节，只是不上屏。
 */
function expectUserFacingChinese(message: string) {
  expect(message).toMatch(/[一-鿿]/);
  expect(message).not.toContain("/api/");
  expect(message).not.toContain("Gateway");
  expect(message).not.toContain("HTTP");
  // 英文状态文本（Not Found、Bad Gateway…）与英文诊断句都不应出现。
  expect(message).not.toMatch(/[A-Za-z]/);
}

const HTML_404 = "<!DOCTYPE html><html><head><title>404 Not Found</title></head><body>Not Found</body></html>";

describe("formatPortalError", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("explains a network failure without naming the Gateway or the backend", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));

    const { fetchLibraryMaterials, formatPortalError, PortalNetworkError } = await import("./client");
    const error = await fetchLibraryMaterials().catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(PortalNetworkError);
    const message = formatPortalError(error);
    expectUserFacingChinese(message);
    expect(message).toContain("网络");
  });

  it("hides the status text of an HTTP error whose body is not JSON", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(HTML_404, { status: 404, statusText: "Not Found", headers: { "Content-Type": "text/html" } })
      )
    );

    const { fetchLibraryMaterials, formatPortalError, PortalHttpError } = await import("./client");
    const error = await fetchLibraryMaterials().catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(PortalHttpError);
    expectUserFacingChinese(formatPortalError(error));
  });

  it("hides the API path of a response that is not valid JSON", async () => {
    // WAF 挑战页或网关错误页以 200 返回 HTML 时就会走到这里。
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(HTML_404, { status: 200, headers: { "Content-Type": "text/html" } }))
    );

    const { fetchLibraryMaterials, formatPortalError, PortalApiError } = await import("./client");
    const error = await fetchLibraryMaterials().catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(PortalApiError);
    expect((error as Error).message).toContain("/api/v1/library/materials");
    expectUserFacingChinese(formatPortalError(error));
  });

  it("hides the API path when a mock-mode read has no data", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "0");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");

    const { fetchFavoritesOverview, formatPortalError } = await import("./client");
    const error = await fetchFavoritesOverview().catch((cause: unknown) => cause);

    expect((error as Error).message).toContain("/api/v1/practice/favorites");
    expectUserFacingChinese(formatPortalError(error));
  });

  it("keeps configuration and client-side guard details out of the message", async () => {
    const { formatPortalError, PortalApiError, PortalConfigError } = await import("./client");

    expectUserFacingChinese(formatPortalError(new PortalConfigError("[portal-api] 服务未就绪，请联系维护者。")));
    expectUserFacingChinese(
      formatPortalError(new PortalApiError("Invalid Practice session id", { code: "PORTAL_INVALID_PRACTICE_SESSION" }))
    );
  });

  it("asks the user to sign in without naming the auth system", async () => {
    const { formatPortalError, PortalUnauthorizedError } = await import("./client");

    const message = formatPortalError(new PortalUnauthorizedError("/api/v1/account/summary"));
    expectUserFacingChinese(message);
    expect(message).toContain("登录");
    expect(message).not.toContain("统一认证");
  });
});

/**
 * Gateway 错误信封里的 message 是写给用户的中文：放行名单里的码原样展示（#554）。
 * 上游透传、不认识的码仍用 Portal 的中文兜底；403、404 的兜底说清楚可以怎么做。
 */
describe("formatPortalError with a Gateway error envelope", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  async function failWith(status: number, body: unknown) {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
      )
    );
    const client = await import("./client");
    const error = await client.fetchPersonalPracticeStats().catch((cause: unknown) => cause);
    return { error, message: client.formatPortalError(error) };
  }

  it("shows the message of an allowlisted Gateway code as the Gateway wrote it", async () => {
    const { message } = await failWith(503, {
      error: "practice statistics are temporarily unavailable",
      message: "学习统计暂时不可用，请稍后再试",
      request_id: "req_stats_down",
    });

    expect(message).toBe("学习统计暂时不可用，请稍后再试");
  });

  it("shows a Gateway permission message instead of calling a 403 temporary", async () => {
    const { error, message } = await failWith(403, {
      error: "practice access denied",
      message: "暂无练习权限。如有疑问，请到账户中心提交工单。",
      request_id: "req_forbidden",
    });

    const { PortalForbiddenError } = await import("./client");
    expect(error).toBeInstanceOf(PortalForbiddenError);
    expect(message).toBe("暂无练习权限。如有疑问，请到账户中心提交工单。");
  });

  it("points a 403 without a message the Portal can show to a support ticket", async () => {
    const { message } = await failWith(403, { error: "upstream_forbidden", message: "forbidden", request_id: "req_forbidden" });

    expect(message).toBe("你没有权限进行这个操作。如有疑问，请到账户中心提交工单。");
  });

  it("sends the user back from a 404 without a message the Portal can show", async () => {
    const { message } = await failWith(404, { error: "not_found", request_id: "req_gone" });

    expect(message).toBe("内容不存在或已下架，请返回上一页重新选择。");
  });

  it("keeps upstream errors the Gateway only passes through on the Portal's own copy", async () => {
    const { message } = await failWith(503, {
      error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" },
      request_id: "req_upstream",
    });

    expect(message).toBe("服务暂时不可用，请稍后再试。");
  });

  it("does not show an upstream body the Gateway passes through, even with an allowlisted code", async () => {
    // Food、Career 的错误体经 Gateway 原样转发，是嵌套信封；它们也用 INVALID_REQUEST 这类码。
    const { message } = await failWith(400, {
      error: { code: "INVALID_REQUEST", message: "请求体无效：price 必须是整数" },
      request_id: "req_upstream_invalid",
    });

    expect(message).toBe("服务暂时不可用，请稍后再试。");
  });

  it("gives the Portal's 404 copy for the Gateway's generic not-found message", async () => {
    const { message } = await failWith(404, { error: "not found", message: "内容不存在或已下架", request_id: "req_gone" });

    expect(message).toBe("内容不存在或已下架，请返回上一页重新选择。");
  });

  it("does not show an allowlisted code's message unless it is Chinese", async () => {
    const { message } = await failWith(503, { error: "proxy_error", message: "dial tcp 10.0.0.7:8080: connection refused" });

    expectUserFacingChinese(message);
    expect(message).toBe("服务暂时不可用，请稍后再试。");
  });

  it("treats an HTML 404 or 403 page as the service being unavailable, not as missing content", async () => {
    const { formatPortalError, PortalForbiddenError, PortalHttpError } = await import("./client");

    expect(formatPortalError(new PortalHttpError("/api/v1/library/materials", 404, "Not Found"))).toBe(
      "服务暂时不可用，请稍后再试。"
    );
    expect(formatPortalError(new PortalForbiddenError("/api/v1/library/materials", "Forbidden"))).toBe(
      "服务暂时不可用，请稍后再试。"
    );
  });

  it("keeps the Portal's own sign-in copy for a 401 the Gateway calls an expired login", async () => {
    // Gateway 对从没登录过的访客也回“登录已过期”，这句不上屏。
    const { message } = await failWith(401, {
      error: "not authenticated",
      message: "登录已过期，请重新登录",
      request_id: "req_guest",
    });

    expect(message).toBe("需要先登录才能继续，请登录后再试。");
  });
});

describe("portalErrorRequestId", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("returns the request id a user can quote in a support ticket", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(HTML_404, {
          status: 404,
          statusText: "Not Found",
          headers: { "Content-Type": "text/html", "X-Request-Id": "req_edge404" },
        })
      )
    );

    const { fetchLibraryMaterials, portalErrorRequestId } = await import("./client");
    const error = await fetchLibraryMaterials().catch((cause: unknown) => cause);

    expect(portalErrorRequestId(error)).toBe("req_edge404");
  });

  it("prefers the request id of the Gateway error envelope over the response header", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ error: "LIBRARY_TEMPORARILY_UNAVAILABLE", message: "资料库暂时无法加载，请稍后重试。", request_id: "req_library_down" }),
          { status: 503, headers: { "Content-Type": "application/json", "X-Request-Id": "req_header_only" } }
        )
      )
    );

    const { fetchLibraryMaterials, portalErrorRequestId } = await import("./client");
    const error = await fetchLibraryMaterials().catch((cause: unknown) => cause);

    expect(portalErrorRequestId(error)).toBe("req_library_down");
  });

  it("shows nothing when there is no well-formed request id", async () => {
    const { PortalHttpError, portalErrorRequestId } = await import("./client");

    expect(portalErrorRequestId(new PortalHttpError("/api/v1/library/materials", 502, "Bad Gateway"))).toBeNull();
    expect(
      portalErrorRequestId(new PortalHttpError("/api/v1/library/materials", 502, "Bad Gateway", "<script>alert(1)</script>"))
    ).toBeNull();
    expect(portalErrorRequestId(new Error("req_not_a_portal_error"))).toBeNull();
    expect(portalErrorRequestId(undefined)).toBeNull();
  });
});
