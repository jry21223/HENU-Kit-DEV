/**
 * Portal Gateway / Portal API client.
 *
 * - When NEXT_PUBLIC_PORTAL_GATEWAY_URL is set, all calls go to the real backend.
 * - In production or when NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1, missing URL throws
 *   and fetch failures are never treated as success.
 * - Mock fallback is only for local dev when NEXT_PUBLIC_PORTAL_ALLOW_MOCK=1.
 */

import {
  allowMock,
  assertGatewayConfigured,
  gatewayUrlRaw,
  hasGatewayConfigured,
  requireGateway,
} from "./env";
import type {
  CampusItemListResponse,
  CategoryListResponse,
  ErrorEnvelope,
  FoodPostListResponse,
  FoodVenuesResponse,
  LibraryCoursesResponse,
  MaterialListResponse,
  NoticeListResponse,
  PortalSession,
  PracticeBanksResponse,
  PracticeStatsResponse,
  SchoolListResponse,
} from "./types";

// ---- Error types ----

export class PortalApiError extends Error {
  readonly status?: number;
  readonly code: string;
  readonly path?: string;
  readonly requestId?: string;

  constructor(
    message: string,
    opts: {
      code?: string;
      status?: number;
      path?: string;
      requestId?: string;
      cause?: unknown;
    } = {}
  ) {
    super(message, opts.cause !== undefined ? { cause: opts.cause } : undefined);
    this.name = "PortalApiError";
    this.code = opts.code ?? "PORTAL_API_ERROR";
    this.status = opts.status;
    this.path = opts.path;
    this.requestId = opts.requestId;
  }
}

export class PortalConfigError extends PortalApiError {
  constructor(message: string) {
    super(message, { code: "PORTAL_CONFIG_ERROR" });
    this.name = "PortalConfigError";
  }
}

export class PortalNetworkError extends PortalApiError {
  constructor(path: string, cause?: unknown) {
    super(`Network error calling ${path}`, {
      code: "PORTAL_NETWORK_ERROR",
      path,
      cause,
    });
    this.name = "PortalNetworkError";
  }
}

export class PortalHttpError extends PortalApiError {
  constructor(
    path: string,
    status: number,
    message: string,
    requestId?: string
  ) {
    super(message, {
      code: "PORTAL_HTTP_ERROR",
      status,
      path,
      requestId,
    });
    this.name = "PortalHttpError";
  }
}

export class PortalUnauthorizedError extends PortalHttpError {
  constructor(path: string, requestId?: string) {
    super(path, 401, "Not authenticated", requestId);
    this.name = "PortalUnauthorizedError";
    (this as { code: string }).code = "PORTAL_UNAUTHORIZED";
  }
}

/** Whether the real gateway URL is configured (build-safe). */
export const hasGateway = hasGatewayConfigured();

/** Production / require-gateway: mock must not be used. */
export const mockAllowed = allowMock();

/** True when production data must come from the gateway. */
export const gatewayRequired = requireGateway();

function resolveBaseUrl(): string {
  try {
    return assertGatewayConfigured("portal-api");
  } catch (e) {
    throw new PortalConfigError(
      e instanceof Error ? e.message : "Gateway URL is not configured"
    );
  }
}

function baseUrlOrEmpty(): string {
  return gatewayUrlRaw();
}

async function parseErrorBody(
  res: Response
): Promise<{ message: string; requestId?: string }> {
  try {
    const body = (await res.json()) as ErrorEnvelope;
    return {
      message: body.error || res.statusText || `HTTP ${res.status}`,
      requestId: body.request_id,
    };
  } catch {
    return { message: res.statusText || `HTTP ${res.status}` };
  }
}

/**
 * Core fetch. Throws on network / HTTP failures when gateway is required
 * or when a URL is configured. Never returns "success" on failure.
 *
 * When no gateway and mock is allowed, returns null so callers can fall back.
 */
async function apiFetch<T>(
  path: string,
  init?: RequestInit
): Promise<T | null> {
  // Same-origin: empty base + path `/api/v1/...` (nginx proxies /api to gateway).
  // Absolute: base = https://host, path still starts with /api/v1.
  if (!hasGatewayConfigured()) {
    if (allowMock()) return null;
    throw new PortalConfigError(
      "Gateway is not configured. Set NEXT_PUBLIC_PORTAL_GATEWAY_URL, or NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1 for same-origin /api, or NEXT_PUBLIC_PORTAL_ALLOW_MOCK=1 for local mock."
    );
  }

  const base = baseUrlOrEmpty();
  const url = `${base}${path}`;

  let res: Response;
  try {
    res = await fetch(url, {
      credentials: "include",
      ...init,
      headers: {
        Accept: "application/json",
        ...(init?.headers ?? {}),
      },
    });
  } catch (e) {
    throw new PortalNetworkError(path, e);
  }

  if (res.status === 401) {
    const { requestId } = await parseErrorBody(res);
    // Session endpoints: 401 means logged out (not a hard failure).
    if (path === "/api/v1/session") return null;
    throw new PortalUnauthorizedError(path, requestId);
  }

  if (!res.ok) {
    const { message, requestId } = await parseErrorBody(res);
    throw new PortalHttpError(path, res.status, message, requestId);
  }

  try {
    return (await res.json()) as T;
  } catch (e) {
    throw new PortalApiError(`Invalid JSON from ${path}`, {
      code: "PORTAL_PARSE_ERROR",
      path,
      cause: e,
    });
  }
}

/** Strict fetch — always throws when data cannot be loaded (no null success). */
async function apiFetchRequired<T>(path: string, init?: RequestInit): Promise<T> {
  // Ensure config assert runs even if URL empty under require mode
  if (requireGateway()) resolveBaseUrl();
  const data = await apiFetch<T>(path, init);
  if (data === null) {
    throw new PortalApiError(`No data from ${path}`, {
      code: "PORTAL_EMPTY_RESPONSE",
      path,
    });
  }
  return data;
}

// ---- Auth ----

export async function fetchSession(): Promise<PortalSession | null> {
  // Mock-only mode (no gateway URL and not require-gateway).
  if (!hasGatewayConfigured()) return null;
  // Absolute origin or same-origin (empty base + REQUIRE_GATEWAY=1).
  return apiFetch<PortalSession>("/api/v1/session");
}

export function redirectToLogin(returnTo = "/") {
  if (typeof window === "undefined") return;
  // Prefer same-origin absolute path so nginx can proxy /api → portal-gateway.
  // resolveBaseUrl() may be "" (same-origin) or an absolute origin.
  const base = (() => {
    try {
      return resolveBaseUrl();
    } catch {
      return "";
    }
  })();
  const target = `${base}/api/v1/auth/login?return_to=${encodeURIComponent(returnTo)}`;
  window.location.assign(target);
}

export async function logout(): Promise<void> {
  if (!hasGatewayConfigured()) return;
  const base = baseUrlOrEmpty();
  await fetch(`${base}/api/v1/session/logout`, {
    method: "POST",
    credentials: "include",
  });
}

// ---- Library ----

export async function fetchLibraryCourses(): Promise<LibraryCoursesResponse | null> {
  return apiFetch<LibraryCoursesResponse>("/api/v1/library/courses");
}

export async function fetchLibraryMaterials(params?: {
  type?: string;
  subject?: string;
  q?: string;
}): Promise<MaterialListResponse> {
  const qs = new URLSearchParams();
  if (params?.type) qs.set("type", params.type);
  if (params?.subject) qs.set("subject", params.subject);
  if (params?.q) qs.set("q", params.q);
  const query = qs.toString();
  return apiFetchRequired<MaterialListResponse>(
    `/api/v1/library/materials${query ? `?${query}` : ""}`
  );
}

// ---- Food ----

export async function fetchFoodVenues(
  campus: string
): Promise<FoodVenuesResponse | null> {
  return apiFetch<FoodVenuesResponse>(
    `/api/v1/food/venues?campus=${encodeURIComponent(campus)}`
  );
}

export async function fetchFoodPosts(campus?: string): Promise<FoodPostListResponse> {
  const qs = campus ? `?campus=${encodeURIComponent(campus)}` : "";
  return apiFetchRequired<FoodPostListResponse>(`/api/v1/food/posts${qs}`);
}

// ---- Practice ----

export async function fetchPracticeBanks(): Promise<PracticeBanksResponse | null> {
  return apiFetch<PracticeBanksResponse>("/api/v1/practice/banks");
}

export async function fetchPracticeStats(): Promise<PracticeStatsResponse> {
  return apiFetchRequired<PracticeStatsResponse>("/api/v1/practice/stats");
}

export async function fetchPracticeSchools(): Promise<SchoolListResponse> {
  return apiFetchRequired<SchoolListResponse>("/api/v1/practice/schools");
}

// ---- Campus ----

export async function fetchCampusItems(params?: {
  type?: string;
  category?: string;
  q?: string;
}): Promise<CampusItemListResponse> {
  const qs = new URLSearchParams();
  if (params?.type) qs.set("type", params.type);
  if (params?.category) qs.set("category", params.category);
  if (params?.q) qs.set("q", params.q);
  const query = qs.toString();
  return apiFetchRequired<CampusItemListResponse>(
    `/api/v1/campus/items${query ? `?${query}` : ""}`
  );
}

export async function fetchCampusCategories(): Promise<CategoryListResponse> {
  return apiFetchRequired<CategoryListResponse>("/api/v1/campus/categories");
}

// ---- Notices ----

export async function fetchNotices(): Promise<NoticeListResponse | null> {
  return apiFetch<NoticeListResponse>("/api/v1/notices");
}

/** Human-readable error for UI banners. */
export function formatPortalError(err: unknown): string {
  if (err instanceof PortalConfigError) {
    return err.message;
  }
  if (err instanceof PortalUnauthorizedError) {
    return "需要登录后才能加载数据，请先完成统一认证。";
  }
  if (err instanceof PortalNetworkError) {
    return "无法连接 Gateway，请检查网络或后端服务状态。";
  }
  if (err instanceof PortalHttpError) {
    return `接口错误 (${err.status ?? "?"}): ${err.message}`;
  }
  if (err instanceof PortalApiError) {
    return err.message;
  }
  if (err instanceof Error) return err.message;
  return "加载失败，请稍后重试。";
}
