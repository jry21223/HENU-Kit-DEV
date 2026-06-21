import type {
  MaterialAccessLevel,
  MaterialStatus,
  MaterialType,
  RecordStatus,
} from "@prisma/client";
import type { AccessLevel, CourseStatus, Material, MaterialType as AppMaterialType } from "@/types";

export function mapRecordStatus(status: RecordStatus): CourseStatus {
  const statusMap: Record<RecordStatus, CourseStatus> = {
    DRAFT: "draft",
    PUBLISHED: "published",
    ARCHIVED: "archived",
  };
  return statusMap[status];
}

export function mapMaterialType(type: MaterialType): AppMaterialType {
  const typeMap: Record<MaterialType, AppMaterialType> = {
    KNOWLEDGE_NOTE: "knowledge_note",
    MOCK_PAPER: "mock_paper",
    ANSWER: "answer",
    QUICK_REVIEW: "quick_review",
    PAST_EXAM: "past_exam",
    OTHER: "other",
  };
  return typeMap[type];
}

export function mapAccessLevel(accessLevel: MaterialAccessLevel): AccessLevel {
  const accessMap: Record<MaterialAccessLevel, AccessLevel> = {
    FREE: "free",
    LOGIN_REQUIRED: "login_required",
    PAID: "paid",
  };
  return accessMap[accessLevel];
}

export function mapMaterialStatus(status: MaterialStatus): Material["status"] {
  const statusMap: Record<MaterialStatus, Material["status"]> = {
    DRAFT: "draft",
    PENDING_REVIEW: "pending_review",
    PUBLISHED: "published",
    ARCHIVED: "archived",
  };
  return statusMap[status];
}

export function formatFileSize(bytes?: number | null) {
  if (!bytes || bytes <= 0) {
    return "未知大小";
  }
  const units = ["B", "KB", "MB", "GB"];
  let size = bytes;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(size >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

