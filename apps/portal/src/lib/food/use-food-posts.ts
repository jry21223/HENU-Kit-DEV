"use client";

import { useCallback, useEffect, useState } from "react";

import type { FoodPost } from "@/lib/api/types";
import { loadFoodPosts } from "@/lib/food/gateway";

export type FoodLoadState = "loading" | "ready" | "error";

/**
 * 美食榜单共享加载状态机（首页美食榜 + /food 榜单页共用）。
 *
 * 挂载后延迟一拍拉取，返回 load 供「重新加载」按钮重试。
 */
export function useFoodPosts() {
  const [posts, setPosts] = useState<FoodPost[]>([]);
  const [loadState, setLoadState] = useState<FoodLoadState>("loading");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    const { posts: loadedPosts, error: loadError } = await loadFoodPosts();
    if (loadError) {
      setPosts([]);
      setError(loadError);
      setLoadState("error");
      return;
    }
    setPosts(loadedPosts);
    setLoadState("ready");
  }, []);

  useEffect(() => {
    // 挂载后延迟一拍拉取，首次渲染先呈现 loading 状态。
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  return { posts, loadState, error, load };
}
