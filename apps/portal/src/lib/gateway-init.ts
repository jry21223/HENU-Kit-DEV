/**
 * Portal Gateway 初始化器。
 *
 * 在客户端首次加载时调用，如果 Gateway 可用，
 * 并行拉取各模块数据并缓存到各 gateway adapter 中。
 */

import { hasGateway, mockAllowed } from "@/lib/api/client";
import { initCampusGateway } from "@/lib/campus/gateway";
import { initFoodGateway } from "@/lib/food/gateway";
import { initGateway } from "@/lib/library/gateway";
import { initPracticeGateway } from "@/lib/practice/gateway";

let initialized = false;
let inflight: Promise<void> | null = null;

/**
 * 初始化所有 Gateway 数据源。
 * 幂等：多次调用只执行一次。
 */
export async function initAllGateways(): Promise<void> {
  if (initialized) return;
  if (typeof window === "undefined") return;
  if (!hasGateway && !mockAllowed) {
    // Still attempt module inits so each records a clear config error.
  }
  if (inflight) return inflight;

  inflight = (async () => {
    await Promise.allSettled([
      initGateway(),
      initFoodGateway(),
      initPracticeGateway(),
      initCampusGateway(),
    ]);
    initialized = true;
    inflight = null;
  })();

  return inflight;
}
