/**
 * Food gateway adapter.
 *
 * Gateway 可用时用 API 数据补充 mock；不可用时完全回退到 mock。
 */

import { hasGateway, fetchFoodVenues } from "@/lib/api/client";
import type { VenueSummary } from "@/lib/api/types";

let gatewayVenues = new Map<string, VenueSummary[]>();

export async function initFoodGateway(): Promise<void> {
  if (!hasGateway) return;
  for (const campus of ["minglun", "jinming", "longzihu"]) {
    try {
      const resp = await fetchFoodVenues(campus);
      if (resp) {
        gatewayVenues.set(campus, resp.venues);
      }
    } catch {
      // 静默失败
    }
  }
}

export function getVenues(campus: string): VenueSummary[] | null {
  return gatewayVenues.get(campus) ?? null;
}
