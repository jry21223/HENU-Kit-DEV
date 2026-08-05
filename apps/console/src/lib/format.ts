// Timestamps arrive as UTC strings; render them in the operator's local time
// with one formatter shared across the Console. Invalid values fall back to
// the raw string instead of rendering a broken date.

export function localDateTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}
