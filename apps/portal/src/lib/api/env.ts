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
  throw new Error(
    `[${context}] NEXT_PUBLIC_PORTAL_GATEWAY_URL is required (or set NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1 for same-origin /api, or NEXT_PUBLIC_PORTAL_ALLOW_MOCK=1 for local mock).`
  );
}
