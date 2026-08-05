// Shared presentation labels for accounts resolved by email lookup (#243).
// Both the membership and the points workspaces render the same person and
// status vocabulary; keep it in one place so they cannot drift.

export function accountName(value?: string): string {
  // Legacy rows may carry an empty label, which must never render as a blank
  // confirmation target.
  return value && value.trim().length > 0 ? value : "（未设置姓名）";
}

const statusLabels: Record<string, string> = {
  active: "正常",
  suspended: "已停用",
  deleted: "已删除",
};

export function accountStatusLabel(status: string): string {
  // The gateway contract restricts status to active/suspended/deleted, so the
  // raw-value fallback is defensive only.
  return statusLabels[status] ?? status;
}
