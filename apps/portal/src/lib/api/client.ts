/**
 * Portal Gateway API client.
 *
 * When NEXT_PUBLIC_PORTAL_GATEWAY_URL is set, all calls go to the real gateway.
 * Otherwise, functions return null and callers should fall back to mock data.
 *
 * All fetch calls use credentials: "same-origin" to send the session cookie.
 */

import type {
  PortalSession,
  ErrorEnvelope,
  LibraryCoursesResponse,
  LibraryMaterialsResponse,
  MaterialDetailResponse,
  FoodPostListResponse,
  FoodPostDetailResponse,
  FoodCommentListResponse,
  PracticeBanksResponse,
  QuizListDetailResponse,
  LeaderboardResponse,
  UserStatsResponse,
  NoticeListResponse,
  CampusItemListResponse,
  CampusItemDetailResponse,
  CategoryListResponse,
} from "./types";

const GATEWAY_URL =
  typeof window !== "undefined"
    ? process.env.NEXT_PUBLIC_PORTAL_GATEWAY_URL || ""
    : "";

/** Whether the real gateway is configured. */
export const hasGateway = GATEWAY_URL !== "";

async function apiFetch<T>(path: string): Promise<T | null> {
  if (!hasGateway) return null;
  try {
    const res = await fetch(`${GATEWAY_URL}${path}`, {
      credentials: "same-origin",
    });
    if (!res.ok) {
      if (res.status === 401) return null;
      const err: ErrorEnvelope = await res.json();
      console.warn(`[portal-api] ${path}: ${err.error}`);
      return null;
    }
    return (await res.json()) as T;
  } catch (e) {
    console.warn(`[portal-api] ${path}: network error`, e);
    return null;
  }
}

// ─── Auth ─────────────────────────────────────────────────

export async function fetchSession(): Promise<PortalSession | null> {
  return apiFetch<PortalSession>("/api/v1/session");
}

export function redirectToLogin(returnTo = "/") {
  if (!hasGateway) return;
  window.location.href = `${GATEWAY_URL}/api/v1/auth/login?return_to=${encodeURIComponent(returnTo)}`;
}

export async function logout(): Promise<void> {
  if (!hasGateway) return;
  await fetch(`${GATEWAY_URL}/api/v1/session/logout`, {
    method: "POST",
    credentials: "same-origin",
  });
}

// ─── Library ──────────────────────────────────────────────

export async function fetchCourses(): Promise<LibraryCoursesResponse | null> {
  return apiFetch<LibraryCoursesResponse>("/api/v1/library/courses");
}

export async function fetchCourseMaterials(
  courseId: string
): Promise<LibraryMaterialsResponse | null> {
  return apiFetch<LibraryMaterialsResponse>(
    `/api/v1/library/courses/${encodeURIComponent(courseId)}/materials`
  );
}

export async function fetchMaterialDetail(
  id: string
): Promise<MaterialDetailResponse | null> {
  return apiFetch<MaterialDetailResponse>(
    `/api/v1/library/materials/${encodeURIComponent(id)}`
  );
}

// ─── Food ─────────────────────────────────────────────────

export async function fetchFoodPosts(
  campus?: string
): Promise<FoodPostListResponse | null> {
  const qs = campus ? `?campus=${encodeURIComponent(campus)}` : "";
  return apiFetch<FoodPostListResponse>(`/api/v1/food/posts${qs}`);
}

export async function fetchFoodPostDetail(
  id: string
): Promise<FoodPostDetailResponse | null> {
  return apiFetch<FoodPostDetailResponse>(
    `/api/v1/food/posts/${encodeURIComponent(id)}`
  );
}

export async function fetchFoodComments(
  postId: string
): Promise<FoodCommentListResponse | null> {
  return apiFetch<FoodCommentListResponse>(
    `/api/v1/food/posts/${encodeURIComponent(postId)}/comments`
  );
}

// ─── Practice ─────────────────────────────────────────────

export async function fetchBanks(): Promise<PracticeBanksResponse | null> {
  return apiFetch<PracticeBanksResponse>("/api/v1/practice/banks");
}

export async function fetchQuizList(
  id: string
): Promise<QuizListDetailResponse | null> {
  return apiFetch<QuizListDetailResponse>(
    `/api/v1/practice/lists/${encodeURIComponent(id)}`
  );
}

export async function fetchLeaderboard(
  period: string
): Promise<LeaderboardResponse | null> {
  return apiFetch<LeaderboardResponse>(
    `/api/v1/practice/leaderboard?period=${encodeURIComponent(period)}`
  );
}

export async function fetchUserStats(): Promise<UserStatsResponse | null> {
  return apiFetch<UserStatsResponse>("/api/v1/practice/stats");
}

// ─── Campus ───────────────────────────────────────────────

export async function fetchCampusItems(
  typeFilter?: string,
  categoryFilter?: string
): Promise<CampusItemListResponse | null> {
  const params = new URLSearchParams();
  if (typeFilter) params.set("type", typeFilter);
  if (categoryFilter) params.set("category", categoryFilter);
  const qs = params.toString();
  return apiFetch<CampusItemListResponse>(
    `/api/v1/campus/items${qs ? "?" + qs : ""}`
  );
}

export async function fetchCampusItemDetail(
  id: string
): Promise<CampusItemDetailResponse | null> {
  return apiFetch<CampusItemDetailResponse>(
    `/api/v1/campus/items/${encodeURIComponent(id)}`
  );
}

export async function fetchCategories(): Promise<CategoryListResponse | null> {
  return apiFetch<CategoryListResponse>("/api/v1/campus/categories");
}

// ─── Notices ──────────────────────────────────────────────

export async function fetchNotices(): Promise<NoticeListResponse | null> {
  return apiFetch<NoticeListResponse>("/api/v1/notices");
}
