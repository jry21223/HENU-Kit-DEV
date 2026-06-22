import path from "node:path";

export function getDownloadContentType(fileName?: string | null) {
  if (fileName?.toLowerCase().endsWith(".pdf")) {
    return "application/pdf";
  }
  if (fileName?.toLowerCase().endsWith(".txt")) {
    return "text/plain; charset=utf-8";
  }
  return "application/octet-stream";
}

export function resolveLocalUploadPath(fileUrl: string, uploadRoot = path.resolve(process.cwd(), "uploads")) {
  const normalized = fileUrl.replaceAll("\\", "/");
  if (!normalized.startsWith("/uploads/")) {
    return null;
  }

  const relativePath = normalized.replace(/^\/uploads\//, "");
  const resolvedPath = path.resolve(uploadRoot, relativePath);
  const relativeToRoot = path.relative(uploadRoot, resolvedPath);

  if (!relativeToRoot || relativeToRoot.startsWith("..") || path.isAbsolute(relativeToRoot)) {
    return null;
  }

  return resolvedPath;
}
