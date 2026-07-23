/**
 * Practice gateway adapter.
 *
 * Gateway 可用时用 API 数据补充 mock；不可用时完全回退到 mock。
 */

import { hasGateway, fetchPracticeBanks } from "@/lib/api/client";
import type { School } from "@/lib/api/types";

let gatewaySchools: School[] | null = null;

export async function initPracticeGateway(): Promise<void> {
  if (!hasGateway) return;
  try {
    const resp = await fetchPracticeBanks();
    if (resp) {
      gatewaySchools = resp.schools;
    }
  } catch {
    // 静默失败
  }
}

export function getGatewaySchools(): School[] | null {
  return gatewaySchools;
}
