/**
 * Practice gateway adapter.
 *
 * Gateway 可用时用 API 数据补充 mock；不可用时完全回退到 mock。
 */

import { hasGateway, fetchPracticeBanks } from "@/lib/api/client";
import type { BankSummary } from "@/lib/api/types";

let gatewayBanks: BankSummary[] | null = null;

export async function initPracticeGateway(): Promise<void> {
  if (!hasGateway) return;
  try {
    const resp = await fetchPracticeBanks();
    if (resp) {
      gatewayBanks = resp.banks;
    }
  } catch {
    // 静默失败
  }
}

export function getGatewayBanks(): BankSummary[] | null {
  return gatewayBanks;
}
