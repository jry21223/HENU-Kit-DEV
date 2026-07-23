"use client";

import { useState } from "react";
import { cn } from "@/lib/cn";

/**
 * 统一图片组件：加载失败/无 src 时回退图纸风编号图块（与地图降级同思路）。
 * 不用 next/image：免 remotePatterns 配置，且兼容 dataURL。
 */
export default function Img({
  src,
  alt,
  label = "IMG",
  className,
}: {
  src?: string;
  alt: string;
  label?: string;
  className?: string;
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
      onError={() => setFailed(true)}
      className={cn("border border-line object-cover", className)}
    />
  );
}
