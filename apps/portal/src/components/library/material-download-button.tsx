"use client";

import { libraryMaterialDownloadURL } from "@/lib/api/client";

export default function MaterialDownloadButton({
  materialId,
  label = "下载资料 ↓",
  className,
}: {
  materialId: string;
  label?: string;
  className?: string;
}) {
  return (
    <a
      href={libraryMaterialDownloadURL(materialId)}
      target="_blank"
      rel="noreferrer"
      className={className}
    >
      {label}
    </a>
  );
}
