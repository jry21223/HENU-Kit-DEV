/**
 * Portal Gateway 初始化器。
 *
 * 在客户端首次加载时调用，如果 Gateway 可用，
 * 并行拉取各模块数据并缓存到各 gateway adapter 中。
 *
 * 页面组件不需要直接调用这个——各 store 内部会使用 gateway adapter。
 */

import { hasGateway } from "@/lib/api/client";
import { initGateway } from "@/lib/library/gateway";
import { initFoodGateway } from "@/lib/food/gateway";
import { initPracticeGateway } from "@/lib/practice/gateway";

let initialized = false;

/**
 * 初始化所有 Gateway 数据源。
 * 幂等：多次调用只执行一次。
 */
export async function initAllGateways(): Promise<void> {
  if (initialized || !hasGateway || typeof window === "undefined") return;
  initialized = true;

  // 并行拉取各模块数据
  await Promise.allSettled([
    initGateway(),
    initFoodGateway(),
    initPracticeGateway(),
  ]);
}
