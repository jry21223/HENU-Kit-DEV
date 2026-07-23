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
  FoodVenuesResponse,
  PracticeBanksResponse,
  NoticeListResponse,
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

// ---- Auth ----

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

// ---- Products (read-only) ----

export async function fetchLibraryCourses(): Promise<LibraryCoursesResponse | null> {
  return apiFetch<LibraryCoursesResponse>("/api/v1/library/materials");
}

export async function fetchFoodVenues(
  campus: string
): Promise<FoodVenuesResponse | null> {
  return apiFetch<FoodVenuesResponse>(
    `/api/v1/food/posts?campus=${encodeURIComponent(campus)}`
  );
}

export async function fetchPracticeBanks(): Promise<PracticeBanksResponse | null> {
  return apiFetch<PracticeBanksResponse>("/api/v1/practice/schools");
}

export async function fetchNotices(): Promise<NoticeListResponse | null> {
  return apiFetch<NoticeListResponse>("/api/v1/notices");
}
