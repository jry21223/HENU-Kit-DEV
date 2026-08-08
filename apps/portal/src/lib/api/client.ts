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
  NoticeFeedEnvelope,
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
  PracticeBanksResponse,
	QuizCraftCatalogResponse,
  QuizCraftRankingPeriod,
  QuizCraftRankingResponse,
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

export async function fetchPracticeBanks(): Promise<PracticeBanksResponse | null> {
  return apiFetch<PracticeBanksResponse>("/api/v1/practice/banks");
}

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

export async function fetchPracticeSchools(): Promise<SchoolListResponse> {
  return apiFetchRequired<SchoolListResponse>("/api/v1/practice/schools");
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

/**
 * Loads the Notice owner's real published snapshot through Portal Gateway.
 * The Gateway returns the Notice service's bounded snapshot envelope, so the
 * full immutable content travels with the feed and detail expands in place.
 * This read never falls back to mock or cached data.
 */
export async function fetchNoticeFeed(): Promise<NoticeFeedEnvelope> {
  return apiFetchRequired<NoticeFeedEnvelope>("/api/v1/notices");
}
