/** Prefer registration display_name; never expose an internal user ID as a label. */
export function publicDisplayName(displayName: string | undefined): string {
  const trimmed = displayName?.trim();
  if (trimmed) return trimmed;
  return "用户";
}
