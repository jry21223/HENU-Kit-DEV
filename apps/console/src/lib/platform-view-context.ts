import type { PlatformOperationsPageRequest } from "./console-gateway";

export type PlatformPageSection = "accounts_page" | "sessions_page" | "inbox_page" | "audit_page";
export type PlatformCursorHistory = Record<PlatformPageSection, Array<string | undefined>>;
export type InboxStatusFilter = "all" | "open" | "in_progress" | "blocked" | "resolved" | "archived";
export const platformPageSections: PlatformPageSection[] = ["accounts_page", "sessions_page", "inbox_page", "audit_page"];
export const platformCursorFields = { accounts_page: "accounts_cursor", sessions_page: "sessions_cursor", inbox_page: "inbox_cursor", audit_page: "audit_cursor" } as const;
const storageKey = "henukit.console.platform-view.v1";
const statuses = ["all", "open", "in_progress", "blocked", "resolved", "archived"];
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export interface PlatformViewContext {
  pages: PlatformOperationsPageRequest;
  cursor_history: PlatformCursorHistory;
  inbox_status: InboxStatusFilter;
  selected_inbox_id: string;
  scroll_top: number;
  account_search_cleared: boolean;
}
export function firstPlatformPages(): PlatformOperationsPageRequest { return { accounts_page: 1, sessions_page: 1, inbox_page: 1, audit_page: 1 }; }
export function emptyPlatformCursorHistory(): PlatformCursorHistory { return { accounts_page: [undefined], sessions_page: [undefined], inbox_page: [undefined], audit_page: [undefined] }; }
function record(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value); }
function only(value: Record<string, unknown>, fields: string[]) { return Object.keys(value).every((key) => fields.includes(key)); }
function cursor(value: unknown) { return value === undefined || value === null || (typeof value === "string" && value.length > 0 && value.length <= 512); }
function valid(value: unknown): value is PlatformViewContext {
  if (!record(value) || !only(value, ["pages", "cursor_history", "inbox_status", "selected_inbox_id", "scroll_top", "account_search_cleared"])) return false;
  const pages = value.pages, history = value.cursor_history;
  if (!record(pages) || !record(history) || !only(pages, [...platformPageSections, ...Object.values(platformCursorFields), "snapshot_at"]) || !only(history, platformPageSections)) return false;
  if (typeof pages.snapshot_at !== "string" || !Number.isFinite(Date.parse(pages.snapshot_at))) return false;
  for (const section of platformPageSections) {
    const page = pages[section], token = pages[platformCursorFields[section]], entries = history[section];
    if (typeof page !== "number" || !Number.isSafeInteger(page) || page < 1 || page > 10000 || !cursor(token) || (page > 1) !== (typeof token === "string")) return false;
    if (!Array.isArray(entries) || entries.length < page || entries.length > 10000 || !entries.every(cursor) || entries[0] != null || (page > 1 && entries[page - 1] !== token)) return false;
  }
  return statuses.includes(String(value.inbox_status)) && typeof value.selected_inbox_id === "string" && (value.selected_inbox_id === "" || uuid.test(value.selected_inbox_id))
    && typeof value.scroll_top === "number" && Number.isFinite(value.scroll_top) && value.scroll_top >= 0 && typeof value.account_search_cleared === "boolean";
}
export function readPlatformViewContext(operatorID: string): PlatformViewContext | undefined {
  try {
    const raw = sessionStorage.getItem(storageKey);
    if (!raw) return;
    const value: unknown = raw.length <= 128000 ? JSON.parse(raw) : null;
    if (!record(value) || value.version !== 1 || value.operator_id !== operatorID || !only(value, ["version", "operator_id", "context"]) || !valid(value.context)) { sessionStorage.removeItem(storageKey); return; }
    const context = value.context;
    // JSON represents empty cursor-history positions as null. Keep those empty,
    // and never parse, synthesize, or reuse a cursor in another collection.
    for (const section of platformPageSections) context.cursor_history[section] = context.cursor_history[section].map((entry) => entry ?? undefined);
    return context;
  } catch { clearPlatformViewContext(); return; }
}
export function writePlatformViewContext(operatorID: string, context: PlatformViewContext): boolean {
  if (!uuid.test(operatorID) || !valid(context)) return false;
  try {
    // Deliberately store no identities, grants, rows, query text, search cursors,
    // drafts, confirmation targets, or operation payloads.
    const raw = JSON.stringify({ version: 1, operator_id: operatorID, context });
    if (raw.length > 128000) return false;
    sessionStorage.setItem(storageKey, raw); return true;
  } catch { return false; }
}
export function clearPlatformViewContext() { try { sessionStorage.removeItem(storageKey); } catch { /* Storage is optional for reading. */ } }
