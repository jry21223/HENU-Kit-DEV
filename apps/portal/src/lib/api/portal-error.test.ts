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
