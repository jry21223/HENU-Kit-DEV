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
  AccountCreateTicketInput,
  AccountMembershipOrderResponse,
  AccountMembershipOrdersResponse,
  AccountMembershipResponse,
  AccountNotificationResponse,
  AccountNotificationsResponse,
  AccountPointsResponse,
  AccountSummaryResponse,
	AccountTicketDetailResponse,
	AccountTicketFollowUpInput,
	AccountTicketResponse,
	AccountTicketsResponse,
	CampusItemListResponse,
	CampusItemDetailResponse,
  CategoryListResponse,
  ErrorEnvelope,
  	FavoritesOverviewResponse,
  	FavoriteFolder,
  	FavoriteListResponse,
  	FavoriteQuestion,
  	FavoriteWriteResponse,
  	FoodPostDetailResponse,
  FoodPostListResponse,
  FoodVenuesResponse,
  LibraryCoursesResponse,
  MaterialDetailResponse,
  MaterialListResponse,
  NoticeListResponse,
  PersonalPracticeStatsEnvelope,
  PortalSession,
	PortalPracticeAnswerInput,
	PortalPracticeAnswerResponse,
	PortalPracticeFeedbackInput,
	PortalPracticeFeedbackResponse,
	PortalPracticeFeedbackStatusResponse,
	PortalPracticeSessionInput,
	PortalPracticeSessionResponse,
	QuizCraftCatalogResponse,
  QuizCraftRankingPeriod,
  QuizCraftRankingResponse,
  CareerProfileInput,
  CareerProfileResponse,
  CareerResumeExtractionResponse,
  CareerSearchResponse,
  CareerSearchesResponse,
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
  readonly errorCode?: string;

  constructor(
    path: string,
    status: number,
    message: string,
    requestId?: string,
    errorCode?: string
  ) {
    super(message, {
      code: "PORTAL_HTTP_ERROR",
      status,
      path,
      requestId,
    });
    this.name = "PortalHttpError";
    this.errorCode = errorCode;
  }
}

export class PortalUnauthorizedError extends PortalHttpError {
  constructor(path: string, requestId?: string) {
    super(path, 401, "Not authenticated", requestId);
    this.name = "PortalUnauthorizedError";
    (this as { code: string }).code = "PORTAL_UNAUTHORIZED";
  }
}

/**
 * 403 语义由 Gateway error envelope 的 error 码区分（如 career 的
 * lifetime_required），errorCode 保留该码供调用方分支。
 */
export class PortalForbiddenError extends PortalHttpError {
  readonly errorCode?: string;

  constructor(
    path: string,
    message: string,
    requestId?: string,
    errorCode?: string
  ) {
    super(path, 403, message, requestId);
    this.name = "PortalForbiddenError";
    this.errorCode = errorCode;
    (this as { code: string }).code = "PORTAL_FORBIDDEN";
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
): Promise<{ message: string; requestId?: string; errorCode?: string }> {
  try {
    const body = (await res.json()) as ErrorEnvelope;
    // Gateway errors are a flat string; Career upstream errors are forwarded
    // verbatim as {code, message}. Both carry the stable code the caller
    // branches on (lifetime_required, AI_UNCONFIGURED, EXTRACT_RATE_LIMITED…).
    const raw = body.error;
    const errorCode = typeof raw === "string" ? raw : raw?.code;
    const message =
      typeof raw === "string" ? raw : (raw?.message ?? res.statusText) || `HTTP ${res.status}`;
    return {
      message,
      requestId: body.request_id,
      errorCode,
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
      "服务未就绪，请联系维护者。"
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

  if (res.status === 403) {
    const { message, requestId, errorCode } = await parseErrorBody(res);
    throw new PortalForbiddenError(path, message, requestId, errorCode);
  }

  if (!res.ok) {
    const { message, requestId, errorCode } = await parseErrorBody(res);
    throw new PortalHttpError(path, res.status, message, requestId, errorCode);
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
export async function apiFetchRequired<T>(path: string, init?: RequestInit): Promise<T> {
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
	return apiFetch<PortalSession>("/api/v1/session", { cache: "no-store" });
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
  await apiFetchRequired<{ status: "signed_out" }>("/api/v1/session/logout", {
    method: "POST",
    cache: "no-store",
  });
  // Best-effort Platform Core session revocation (SSO logout): the Core
  // session cookie lives on the same origin (/account-auth, proxied to
  // Platform Core), and a failed revoke must never block the local logout —
  // it degrades to local-only sign-out and the next login simply requires the
  // email code again. Multi-origin gateway setups where /account-auth is not
  // served on the portal origin fail the same way and stay harmless. The
  // bounded timeout keeps a hung Core from stalling the sign-out redirect.
  try {
    const response = await fetch("/account-auth/api/v1/sessions/revoke", {
      method: "POST",
      cache: "no-store",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ all_sessions: false }),
      signal: AbortSignal.timeout(3000),
    });
    // 401/404 mean there was no Core session to revoke (already signed out,
    // or a multi-origin setup without /account-auth) — expected silence.
    // A 5xx is a real revocation failure worth knowing about.
    if (response.status >= 500) {
      console.warn(`Core session revocation failed with status ${response.status}`);
    }
  } catch (error) {
    console.warn("Core session revocation failed", error);
  }
}

/**
 * Clears the legacy cached auth session (henukit.session) written by the old
 * mock-era auth store. In require-Gateway builds the real session lives in the
 * Gateway cookie, so this cache is only ever stale state left behind by older
 * portal versions; clearing it on logout and on session-expiry keeps the OAuth
 * login entry from bouncing straight back into the account console (#412).
 */
export function clearCachedSession(): void {
  try {
    localStorage.removeItem("henukit.session");
  } catch {
    /* private-mode / storage-unavailable contexts: nothing to clear */
  }
}

// ---- Account Portfolio ----

/**
 * Reads the authenticated user's persisted Account Portfolio summary.
 * This intentionally uses the strict boundary: an unavailable service is an
 * error state, never a local or session-backed successful account response.
 */
export async function fetchAccountSummary(): Promise<AccountSummaryResponse> {
  return apiFetchRequired<AccountSummaryResponse>("/api/v1/account/summary");
}

/** Reads one cursor page of the signed-in user's immutable point ledger. */
export async function fetchAccountPoints(cursor?: string): Promise<AccountPointsResponse> {
  if (cursor !== undefined && (cursor.length === 0 || cursor.length > 512 || cursor.trim() !== cursor)) {
    throw new PortalApiError("Invalid account point-ledger cursor", {
      code: "PORTAL_INVALID_POINTS_CURSOR",
      path: "/api/v1/account/points",
    });
  }
  const query = new URLSearchParams({ limit: "20" });
  if (cursor !== undefined) query.set("cursor", cursor);
  return apiFetchRequired<AccountPointsResponse>(
    `/api/v1/account/points?${query.toString()}`,
    { cache: "no-store" }
  );
}

/** Reads only the signed-in user's durable membership entitlement. */
export async function fetchAccountMembership(): Promise<AccountMembershipResponse> {
  return apiFetchRequired<AccountMembershipResponse>("/api/v1/account/membership", {
    cache: "no-store",
  });
}

/**
 * Starts the signed-in user's own lifetime membership purchase.
 *
 * The request carries no body fields: the plan, the amount, and the merchant
 * order number are all server-owned, so nothing here can influence what is
 * charged. Retrying with the same key returns the original order rather than
 * starting a second purchase, so the caller must retain it.
 */
export async function createAccountMembershipOrder(
  idempotencyKey: string
): Promise<AccountMembershipOrderResponse> {
  return apiFetchRequired<AccountMembershipOrderResponse>(
    "/api/v1/account/membership-orders",
    accountCommandInit(idempotencyKey, {})
  );
}

/** Reads only the signed-in user's own durable membership orders. */
export async function fetchAccountMembershipOrders(): Promise<AccountMembershipOrdersResponse> {
  return apiFetchRequired<AccountMembershipOrdersResponse>("/api/v1/account/membership-orders", {
    cache: "no-store",
  });
}

/** Reads only the signed-in user's durable notifications. */
export async function fetchAccountNotifications(): Promise<AccountNotificationsResponse> {
  return apiFetchRequired<AccountNotificationsResponse>("/api/v1/account/notifications", {
    cache: "no-store",
  });
}

/** Reads only the signed-in user's durable support-ticket list. */
export async function fetchAccountTickets(): Promise<AccountTicketsResponse> {
  return apiFetchRequired<AccountTicketsResponse>("/api/v1/account/tickets", {
    cache: "no-store",
  });
}

/** Reads one owner-scoped ticket and its durable history. */
export async function fetchAccountTicket(ticketID: string): Promise<AccountTicketDetailResponse> {
  return apiFetchRequired<AccountTicketDetailResponse>(
    `/api/v1/account/tickets/${encodeURIComponent(ticketID)}`,
    { cache: "no-store" }
  );
}

function accountCommandInit(idempotencyKey: string, body?: unknown): RequestInit {
  return {
    method: "POST",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": idempotencyKey,
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  };
}

/** Creates one durable ticket. The caller must retain the key for a retry. */
export async function createAccountTicket(
  input: AccountCreateTicketInput,
  idempotencyKey: string
): Promise<AccountTicketResponse> {
  return apiFetchRequired<AccountTicketResponse>(
    "/api/v1/account/tickets",
    accountCommandInit(idempotencyKey, input)
  );
}

/** Adds one durable owner follow-up using the ticket revision currently shown. */
export async function createAccountTicketFollowUp(
  ticketID: string,
  input: AccountTicketFollowUpInput,
  idempotencyKey: string
): Promise<AccountTicketResponse> {
  return apiFetchRequired<AccountTicketResponse>(
    `/api/v1/account/tickets/${encodeURIComponent(ticketID)}/follow-ups`,
    accountCommandInit(idempotencyKey, input)
  );
}

/** Marks exactly one durable notification as read. */
export async function markAccountNotificationRead(
  notificationID: string,
  idempotencyKey: string
): Promise<AccountNotificationResponse> {
  return apiFetchRequired<AccountNotificationResponse>(
    `/api/v1/account/notifications/${encodeURIComponent(notificationID)}/read`,
    accountCommandInit(idempotencyKey)
  );
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

export async function fetchLibraryMaterialDetail(
  id: string
): Promise<MaterialDetailResponse | null> {
  return apiFetch<MaterialDetailResponse>(
    `/api/v1/library/materials/${encodeURIComponent(id)}`
  );
}

/** Returns only the same-origin owner entry; signed OSS URLs never enter state. */
export function libraryMaterialDownloadURL(id: string): string {
  assertGatewayConfigured();
  return `${gatewayUrlRaw()}/api/v1/library/materials/${encodeURIComponent(id)}/download`;
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

export async function fetchFoodPost(id: string): Promise<FoodPostDetailResponse> {
  return apiFetchRequired<FoodPostDetailResponse>(
    `/api/v1/food/posts/${encodeURIComponent(id)}`
  );
}

// ---- Practice ----

/**
 * Reads the Gateway's cutover-only QuizCraft V2 catalog. It intentionally
 * uses the strict boundary: an unavailable or disabled route is an error,
 * never a legacy Portal API or client-side mock success response.
 */
export async function fetchQuizCraftCatalog(): Promise<QuizCraftCatalogResponse> {
  return apiFetchRequired<QuizCraftCatalogResponse>("/api/v1/practice/catalog");
}

/** Reads the public ranking derived by Core from immutable scored attempts. */
export async function fetchQuizCraftOverallRanking(
  period: QuizCraftRankingPeriod
): Promise<QuizCraftRankingResponse> {
  return apiFetchRequired<QuizCraftRankingResponse>(
    `/api/v1/rankings/overall?period=${encodeURIComponent(period)}`
  );
}

/**
 * Fetches V2 fact-derived personal Practice stats. Callers must first check
 * quizCraftV2ReadsEnabled(); this strict path has no local/mock success mode.
 */
export async function fetchPersonalPracticeStats(): Promise<PersonalPracticeStatsEnvelope> {
  return apiFetchRequired<PersonalPracticeStatsEnvelope>("/api/v1/practice/stats");
}

function practiceCommandInit(idempotencyKey: string, body: unknown): RequestInit {
  const key = idempotencyKey.trim();
  if (key.length < 16 || key.length > 160) {
    throw new PortalApiError("Invalid Practice idempotency key", {
      code: "PORTAL_INVALID_PRACTICE_IDEMPOTENCY_KEY",
    });
  }
  return {
    method: "POST",
    cache: "no-store",
    // Practice commands are browser-to-Gateway calls only. Do not let this
    // narrow command boundary opt into cross-origin credential forwarding.
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": key,
    },
    body: JSON.stringify(body),
  };
}

/**
 * Starts a real QuizCraft session through Portal Gateway. Callers retain the
 * idempotency key for a retry; this client never falls back to QUIZ_SET or a
 * browser-side score.
 */
export async function createPracticeSession(
  input: PortalPracticeSessionInput,
  idempotencyKey: string
): Promise<PortalPracticeSessionResponse> {
  return apiFetchRequired<PortalPracticeSessionResponse>(
    "/api/v1/practice/sessions",
    practiceCommandInit(idempotencyKey, input)
  );
}

/** Submits one answer to QuizCraft; only the returned result establishes score. */
export async function submitPracticeAnswer(
  sessionID: string,
  input: PortalPracticeAnswerInput,
  idempotencyKey: string
): Promise<PortalPracticeAnswerResponse> {
  if (!sessionID.trim()) {
    throw new PortalApiError("Invalid Practice session id", {
      code: "PORTAL_INVALID_PRACTICE_SESSION",
    });
  }
  return apiFetchRequired<PortalPracticeAnswerResponse>(
    `/api/v1/practice/sessions/${encodeURIComponent(sessionID)}/answers`,
    practiceCommandInit(idempotencyKey, input)
  );
}

/**
 * Submits a signed-in correction for one stable question. Core owns question
 * reference validation and per-user idempotency; Gateway relays the accepted
 * write result. The returned resource_id is the feedback_id for status reads.
 */
export async function createPracticeFeedback(
  input: PortalPracticeFeedbackInput,
  idempotencyKey: string
): Promise<PortalPracticeFeedbackResponse> {
  return apiFetchRequired<PortalPracticeFeedbackResponse>(
    "/api/v1/practice/feedback",
    practiceCommandInit(idempotencyKey, input)
  );
}

/** Reads one signed-in user's correction processing status. */
export async function fetchPracticeFeedbackStatus(
  feedbackID: string
): Promise<PortalPracticeFeedbackStatusResponse> {
  return apiFetchRequired<PortalPracticeFeedbackStatusResponse>(
    `/api/v1/practice/feedback/${encodeURIComponent(feedbackID)}/status`
  );
}

// ---- Practice favorites ----

function favoriteCommandInit(
  idempotencyKey: string,
  method: "PUT" | "DELETE" | "POST"
): RequestInit {
  const key = idempotencyKey.trim();
  if (key.length < 16 || key.length > 160) {
    throw new PortalApiError("Invalid Practice idempotency key", {
      code: "PORTAL_INVALID_PRACTICE_IDEMPOTENCY_KEY",
    });
  }
  return {
    method,
    cache: "no-store",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": key,
    },
    body: JSON.stringify({}),
  };
}

/** Lists the signed-in user's per-bank favorite folders. */
export async function fetchFavoritesOverview(): Promise<FavoritesOverviewResponse> {
  return apiFetchRequired<FavoritesOverviewResponse>("/api/v1/practice/favorites");
}

/** Lists one bank's favorite references for the signed-in user. */
export async function fetchBankFavorites(
  bankID: string
): Promise<FavoriteListResponse> {
  return apiFetchRequired<FavoriteListResponse>(
    `/api/v1/practice/banks/${encodeURIComponent(bankID)}/favorites`
  );
}

/** Favorites one stable question reference idempotently. */
export async function favoriteQuestion(
  bankID: string,
  questionID: string,
  idempotencyKey: string
): Promise<FavoriteWriteResponse> {
  return apiFetchRequired<FavoriteWriteResponse>(
    `/api/v1/practice/banks/${encodeURIComponent(bankID)}/favorites/${encodeURIComponent(questionID)}`,
    favoriteCommandInit(idempotencyKey, "PUT")
  );
}

/** Removes one stable favorite idempotently. */
export async function unfavoriteQuestion(
  bankID: string,
  questionID: string,
  idempotencyKey: string
): Promise<FavoriteWriteResponse> {
  return apiFetchRequired<FavoriteWriteResponse>(
    `/api/v1/practice/banks/${encodeURIComponent(bankID)}/favorites/${encodeURIComponent(questionID)}`,
    favoriteCommandInit(idempotencyKey, "DELETE")
  );
}

/** Starts a Practice Core session from one bank's available favorites. */
export async function createFavoritesSession(
  bankID: string,
  idempotencyKey: string
): Promise<PortalPracticeSessionResponse> {
  return apiFetchRequired<PortalPracticeSessionResponse>(
    `/api/v1/practice/banks/${encodeURIComponent(bankID)}/favorites/practice-sessions`,
    favoriteCommandInit(idempotencyKey, "POST")
  );
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

export async function fetchCampusItemDetail(
  id: string
): Promise<CampusItemDetailResponse> {
  return apiFetchRequired<CampusItemDetailResponse>(
    `/api/v1/campus/items/${encodeURIComponent(id)}`
  );
}

export async function fetchCampusCategories(): Promise<CategoryListResponse> {
  return apiFetchRequired<CategoryListResponse>("/api/v1/campus/categories");
}

// ---- Notices ----

export async function fetchNotices(): Promise<NoticeListResponse | null> {
  return apiFetch<NoticeListResponse>("/api/v1/notices");
}

// ---- Career (Work Radar) ----

/**
 * Career create 命令 init：POST + Idempotency-Key（网关契约要求头必填）。
 * key 校验区间对齐 food submit 的 8..200。
 */
function careerCommandInit(idempotencyKey: string, body: unknown): RequestInit {
  const key = idempotencyKey.trim();
  if (key.length < 8 || key.length > 200) {
    throw new PortalApiError("Invalid Career idempotency key", {
      code: "PORTAL_INVALID_CAREER_IDEMPOTENCY_KEY",
    });
  }
  return {
    method: "POST",
    cache: "no-store",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": key,
    },
    body: JSON.stringify(body),
  };
}

/**
 * 创建一次异步求职搜索。Gateway 从 Session 绑定 actor，浏览器侧不传
 * user_id。调用方必须保留 idempotencyKey 供双击/重试复用。非 Lifetime
 * 会员 → PortalForbiddenError（errorCode = "lifetime_required"）。
 */
export async function createCareerSearch(
  profile: CareerProfileInput,
  idempotencyKey: string
): Promise<CareerSearchResponse> {
  return apiFetchRequired<CareerSearchResponse>(
    "/api/v1/career/searches",
    careerCommandInit(idempotencyKey, { profile })
  );
}

/** 读取当前用户的历史搜索。 */
export async function listCareerSearches(): Promise<CareerSearchesResponse> {
  return apiFetchRequired<CareerSearchesResponse>("/api/v1/career/searches", {
    cache: "no-store",
  });
}

/** 读取单次搜索的当前状态（轮询用）。 */
export async function getCareerSearchStatus(
  searchID: string
): Promise<CareerSearchResponse> {
  if (!searchID.trim()) {
    throw new PortalApiError("Invalid Career search id", {
      code: "PORTAL_INVALID_CAREER_SEARCH_ID",
    });
  }
  return apiFetchRequired<CareerSearchResponse>(
    `/api/v1/career/searches/${encodeURIComponent(searchID)}`,
    { cache: "no-store" }
  );
}

/** 读取当前用户的求职画像；从未设置时返回仅含必填字段的空画像。 */
export async function getCareerProfile(): Promise<CareerProfileResponse> {
  return apiFetchRequired<CareerProfileResponse>("/api/v1/career/profile", {
    cache: "no-store",
  });
}

/** 全量替换当前用户的求职画像；浏览器侧不传 user_id。 */
export async function updateCareerProfile(
  profile: CareerProfileInput
): Promise<CareerProfileResponse> {
  return apiFetchRequired<CareerProfileResponse>("/api/v1/career/profile", {
    method: "PUT",
    cache: "no-store",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(profile),
  });
}

/**
 * 上传简历并创建后台 AI 提取任务（异步：任务入队后立即返回，轮询状态）。
 * FormData 由浏览器自动带 boundary，绝不手写 Content-Type。
 */
export async function createCareerResumeExtraction(
  file: File
): Promise<CareerResumeExtractionResponse> {
  const form = new FormData();
  form.append("file", file);
  return apiFetchRequired<CareerResumeExtractionResponse>(
    "/api/v1/career/profile/extractions",
    {
      method: "POST",
      cache: "no-store",
      credentials: "include",
      body: form,
    }
  );
}

/** 读取一次简历提取任务的状态与提取结果。 */
export async function getCareerResumeExtraction(
  extractionID: string
): Promise<CareerResumeExtractionResponse> {
  if (!extractionID.trim()) {
    throw new PortalApiError("Invalid Career extraction id", {
      code: "PORTAL_INVALID_CAREER_EXTRACTION_ID",
    });
  }
  return apiFetchRequired<CareerResumeExtractionResponse>(
    `/api/v1/career/profile/extractions/${encodeURIComponent(extractionID)}`,
    { cache: "no-store" }
  );
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
    return `服务暂时不可用，请稍后再试。`;
  }
  if (err instanceof PortalApiError) {
    return err.message;
  }
  if (err instanceof Error) return "加载失败，请稍后重试。";
  return "加载失败，请稍后重试。";
}
