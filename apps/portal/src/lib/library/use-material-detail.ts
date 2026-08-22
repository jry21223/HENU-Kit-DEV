"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchLibraryMaterialDetail, formatPortalError, PortalHttpError } from "@/lib/api/client";
import type { Material } from "@/lib/library/mock";

/**
 * 资料详情加载三态（判别联合，组件用单一守卫 `loadState !== "ready"`，
 * ready 后 material 自动收窄为非空）。
 */
export type MaterialDetailState =
  | { loadState: "loading"; material: null; error: null }
  | { loadState: "ready"; material: Material; error: null }
  | { loadState: "not-found"; material: null; error: null }
  | { loadState: "error"; material: null; error: string; retry: () => void };

/**
 * 资料详情统一从 Library owner 加载。owner 失败必须保持失败态，不能用缓存或 mock
 * 伪装成一次成功读取。
 */
export function useMaterialDetail(id: string): MaterialDetailState {
  const [state, setState] = useState<MaterialDetailState>({
    loadState: "loading",
    material: null,
    error: null,
  });
  const requestVersion = useRef(0);
  const [attempt, setAttempt] = useState(0);
  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  const load = useCallback(async () => {
    const version = ++requestVersion.current;
    setState({ loadState: "loading", material: null, error: null });
    try {
      const response = await fetchLibraryMaterialDetail(id);
      if (version !== requestVersion.current) return;
      const found = response?.material;
      if (found) {
        setState({ loadState: "ready", material: found, error: null });
        return;
      }
      // 内容不存在：404 文案由展示组件提供，error 保持 null，避免文案叠加。
      setState({ loadState: "not-found", material: null, error: null });
    } catch (loadError) {
      if (version !== requestVersion.current) return;
      if (loadError instanceof PortalHttpError && loadError.status === 404) {
        setState({ loadState: "not-found", material: null, error: null });
        return;
      }
      setState({
        loadState: "error",
        material: null,
        error: loadError instanceof PortalHttpError
          ? "资料详情暂时无法加载，请稍后重试。"
          : formatPortalError(loadError),
        retry,
      });
    }
  }, [id, retry]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => {
      requestVersion.current += 1;
      window.clearTimeout(timer);
    };
  }, [attempt, load]);

  return state;
}
