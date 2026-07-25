/**
 * Portal runtime env flags.
 *
 * Production / require-gateway mode must never treat mock as source of truth.
 * Mock is only for local/dev when explicitly allowed.
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

/** Raw configured gateway URL (may be empty at build time). */
export function gatewayUrlRaw(): string {
  return (process.env.NEXT_PUBLIC_PORTAL_GATEWAY_URL || "").replace(/\/$/, "");
}

/**
 * Whether a gateway base URL is configured.
 * Empty string at build is OK for static generation; runtime callers
 * must assert via assertGatewayConfigured() when requireGateway().
 */
export function hasGatewayConfigured(): boolean {
  return gatewayUrlRaw() !== "";
}

/**
 * Throw when production/require-gateway needs a URL but none is set.
 * Safe to call from client data loaders; avoid at module top-level so
 * `next build` can complete without a real URL (placeholder allowed).
 */
export function assertGatewayConfigured(context = "portal"): string {
  const url = gatewayUrlRaw();
  if (url) return url;
  if (requireGateway()) {
    throw new Error(
      `[${context}] NEXT_PUBLIC_PORTAL_GATEWAY_URL is required when NODE_ENV=production or NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1`
    );
  }
  return "";
}
