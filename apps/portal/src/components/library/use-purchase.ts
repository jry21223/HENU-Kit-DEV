"use client";

import { useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { accountStore } from "@/lib/auth/mock";
import { libraryStore, PurchaseResult } from "@/lib/library/mock";

/** 购买动作（详情页与锁定墙共用）：未登录跳登录，返回购买结果 */
export function usePurchase() {
  const router = useRouter();
  const { user } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  const { balance } = useSyncExternalStore(accountStore.subscribe, accountStore.get, accountStore.getServer);

  const buy = (id: string, nextPath: string): PurchaseResult | "login" => {
    if (!user) {
      router.push(`/account/login?next=${encodeURIComponent(nextPath)}`);
      return "login";
    }
    return libraryStore.purchase(id);
  };

  return { buy, balance, user };
}
