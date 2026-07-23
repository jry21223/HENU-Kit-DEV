/**
 * Practice gateway adapter.
 * Fetches from API when available, falls back to mock.
 */

import { hasGateway, fetchBanks } from "@/lib/api/client";
import type { School } from "@/lib/api/types";

let gatewaySchools: School[] | null = null;

export async function initPracticeGateway(): Promise<void> {
  if (!hasGateway) return;
  try {
    const resp = await fetchBanks();
    if (resp) {
      gatewaySchools = resp.schools;
    }
  } catch {
    // silent
  }
}

export function getGatewaySchools(): School[] | null {
  return gatewaySchools;
}
