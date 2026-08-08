"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchLibraryMaterialDetail, formatPortalError } from "@/lib/api/client";
import { getMaterialOrFallback } from "@/lib/library/gateway";
import type { Material } from "@/lib/library/mock";

/**
 * 资料详情加载三态（判别联合，组件用单一守卫 `loadState !== "ready"`，
 * ready 后 material 自动收窄为非空）。
 */
export type MaterialDetailState =
  | { loadState: "loading"; material: null; error: null }
  | { loadState: "ready"; material: Material; error: null }
  | { loadState: "error"; material: null; error: string | null };

/**
 * 资料详情统一加载：真实 detail 接口 → gateway 缓存/mock 回退 → error。
 * fallback 只取一次，按分支使用：成功路径 response 优先，失败路径用 fallback。
 */
export function useMaterialDetail(id: string): MaterialDetailState {
  const [state, setState] = useState<MaterialDetailState>({
    loadState: "loading",
    material: null,
    error: null,
  });
  const mounted = useRef(true);

  const load = useCallback(async () => {
    setState({ loadState: "loading", material: null, error: null });
    const fallback = getMaterialOrFallback(id);
    try {
      const response = await fetchLibraryMaterialDetail(id);
      if (!mounted.current) return;
      const found = response?.material ?? fallback;
      if (found) {
        setState({ loadState: "ready", material: found, error: null });
        return;
      }
      // 内容不存在：404 文案由展示组件提供，error 保持 null，避免文案叠加。
      setState({ loadState: "error", material: null, error: null });
    } catch (loadError) {
      if (!mounted.current) return;
      if (fallback) {
        setState({ loadState: "ready", material: fallback, error: null });
        return;
      }
      setState({ loadState: "error", material: null, error: formatPortalError(loadError) });
    }
  }, [id]);

  useEffect(() => {
    mounted.current = true;
    const timer = window.setTimeout(() => void load(), 0);
    return () => {
      mounted.current = false;
      window.clearTimeout(timer);
    };
  }, [load]);

  return state;
}
