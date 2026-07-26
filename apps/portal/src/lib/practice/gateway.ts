/**
 * Practice gateway adapter.
 *
 * 生产 / require-gateway：必须走 API；失败不静默回退 mock。
 */

import {
  fetchPracticeBanks,
  fetchPracticeSchools,
  hasGateway,
  mockAllowed,
  PortalApiError,
} from "@/lib/api/client";
import type { BankSummary, PracticeSchool } from "@/lib/api/types";

let gatewayBanks: BankSummary[] | null = null;
let gatewaySchools: PracticeSchool[] | null = null;
let lastError: string | null = null;
let loaded = false;

export function getPracticeGatewayError(): string | null {
  return lastError;
}

export async function initPracticeGateway(): Promise<void> {
  if (loaded) return;

  if (!hasGateway) {
    if (mockAllowed) {
      loaded = true;
      lastError = null;
      return;
    }
    lastError =
      "Gateway 未配置。生产环境禁止 mock；请设置 NEXT_PUBLIC_PORTAL_GATEWAY_URL。";
    return;
  }

  try {
    // Prefer full school hierarchy (portal-api); banks as secondary signal.
    try {
      const schools = await fetchPracticeSchools();
      gatewaySchools = schools.schools;
    } catch {
      gatewaySchools = null;
    }

    try {
      const banks = await fetchPracticeBanks();
      gatewayBanks = banks?.banks ?? null;
    } catch {
      gatewayBanks = null;
    }

    if (!gatewaySchools && !gatewayBanks) {
      throw new PortalApiError("practice schools/banks both unavailable", {
        code: "PORTAL_EMPTY_RESPONSE",
        path: "/api/v1/practice/schools",
      });
    }

    loaded = true;
    lastError = null;
  } catch (e) {
    lastError =
      e instanceof PortalApiError
        ? e.message
        : e instanceof Error
          ? e.message
          : "加载刷题数据失败";
    if (!mockAllowed) {
      gatewaySchools = null;
      gatewayBanks = null;
      loaded = false;
    } else {
      loaded = true;
    }
  }
}

export function getGatewayBanks(): BankSummary[] | null {
  return gatewayBanks;
}

export function getGatewaySchools(): PracticeSchool[] | null {
  return gatewaySchools;
}

export function isPracticeReady(): boolean {
  return loaded || mockAllowed;
}
