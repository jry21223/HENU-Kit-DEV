"use client";

import { useState } from "react";
import { cn } from "@/lib/cn";

/**
 * 统一图片组件：加载失败/无 src 时回退图纸风编号图块。
 * 不用 next/image：免 remotePatterns 配置，且兼容 dataURL。
 *
 * 默认懒加载、异步解码（#548）：列表里首屏以下的图滚动到附近才请求。首屏关键图由调用方
 * 传 loading="eager" fetchPriority="high"。调用方用 className 给出固定宽高或 aspect-ratio，
 * 图片加载前就占好位置，加载时不挤动版面；回退图块沿用同一尺寸。
 */
export default function Img({
  src,
  alt,
  label = "IMG",
  className,
  loading = "lazy",
  fetchPriority,
}: {
  src?: string;
  alt: string;
  label?: string;
  className?: string;
  loading?: "lazy" | "eager";
  fetchPriority?: "high" | "low" | "auto";
}) {
  const [failed, setFailed] = useState(false);

  if (!src || failed) {
    return (
      <div
        className={cn(
          "bg-blueprint flex items-center justify-center border border-line",
          className
        )}
        role="img"
        aria-label={alt}
      >
        <span className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
          {label} / 暂无图片
        </span>
      </div>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={src}
      alt={alt}
      loading={loading}
      decoding="async"
      fetchPriority={fetchPriority}
      onError={() => setFailed(true)}
      className={cn("border border-line object-cover", className)}
    />
  );
}
