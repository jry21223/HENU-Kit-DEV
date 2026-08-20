/**
 * Portal runtime env flags.
 *
 * Production / require-gateway mode must never treat mock as source of truth.
 * Mock is only for local/dev when explicitly allowed.
 *
 * Same-origin deploy (nginx /api → portal-gateway): leave
 * NEXT_PUBLIC_PORTAL_GATEWAY_URL empty and set REQUIRE_GATEWAY=1 so fetches
 * use absolute paths like /api/v1/... on the current host.
 */

export function isProductionRuntime(): boolean {
  return process.env.NODE_ENV === "production";
}

/** Force real gateway (also implied by production). */
export function requireGateway(): boolean {
  return (
    isProductionRuntime() ||
    process.env.NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY === "1"
  );
}

/**
 * Mock data + mock login are allowed only when explicitly opted in
 * and gateway is not required.
 */
export function allowMock(): boolean {
  if (requireGateway()) return false;
  return process.env.NEXT_PUBLIC_PORTAL_ALLOW_MOCK === "1";
}

/**
 * The QuizCraft V2 catalog is a coordinated, cutover-only surface. This
 * browser flag intentionally defaults to 0; the Gateway must also opt in with
 * PORTAL_ENABLE_QUIZCRAFT_CATALOG=1 before any real catalog can load.
 *
 * Default alignment (ADR-0036): every server gate and browser flag defaults to
 * 0 (fail-closed). The release-image inventory bakes these browser flags to 1
 * for the #166 cutover build only — see scripts/ops/henukit-release-images.sh.
 * A baked 1 without the matching server-side flag is deliberate: the Gateway
 * answers an honest 404/503 until the whole bundle is switched together,
 * never a mock/legacy fallback.
 */
export function quizCraftCatalogEnabled(): boolean {
  return process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG === "1";
}

/**
 * QuizCraft V2 personal-data reads are dark until the server-side Gateway flag
 * is enabled in the #166 cutover window. This client flag never enables a
 * server route by itself; it only prevents the browser from probing it early;
 * the same default-0 / cutover-bake-1 alignment as quizCraftCatalogEnabled()
 * applies.
 */
export function quizCraftV2ReadsEnabled(): boolean {
  return process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS === "1";
}

/**
 * Gateway base URL (no trailing slash).
 * Empty string = same-origin (/api/v1/... via nginx).
 */
export function gatewayUrlRaw(): string {
  if (typeof process.env.NEXT_PUBLIC_PORTAL_GATEWAY_URL === "string") {
    return process.env.NEXT_PUBLIC_PORTAL_GATEWAY_URL.replace(/\/$/, "");
  }
  return "";
}

/** Whether API calls should go to a real gateway (including same-origin). */
export function hasGatewayConfigured(): boolean {
  if (gatewayUrlRaw() !== "") return true;
  if (requireGateway()) return true;
  return false;
}

/** Returns base URL (may be ""). Throws only when neither gateway nor mock is allowed. */
export function assertGatewayConfigured(context = "portal"): string {
  if (hasGatewayConfigured()) return gatewayUrlRaw();
  if (allowMock()) return "";
  // 配置说明（NEXT_PUBLIC_PORTAL_GATEWAY_URL / NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY / NEXT_PUBLIC_PORTAL_ALLOW_MOCK）仅供排查日志，不展示给用户。
  throw new Error(`[${context}] 服务未就绪，请联系维护者。`);
}
