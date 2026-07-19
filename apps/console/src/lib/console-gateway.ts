// Code generated from console-gateway.yaml (SHA256 780f86a12079eb465ebdcf4750a0ecbbbdb8ee9c33c65a35b9e7664197b895d1); DO NOT EDIT.
export interface ConsoleScope {
  kind: "platform";
}

export interface ConsoleAccessContext {
  permissions: string[];
  scopes: ConsoleScope[];
  verified_at: string;
}

export interface ConsoleSession {
  user: { id: string };
  access_context: ConsoleAccessContext;
  expires_at: string;
}

interface SuccessEnvelope<T> {
  data: T;
  request_id: string;
}

export type ConsoleSessionResult =
  | { state: "authenticated"; session: ConsoleSession }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchConsoleSession(): Promise<ConsoleSessionResult> {
  try {
    const response = await fetch("/api/v1/session", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope = (await response.json()) as SuccessEnvelope<ConsoleSession>;
    if (!isConsoleSession(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", session: envelope.data };
  } catch {
    return { state: "unavailable" };
  }
}

function isConsoleSession(value: unknown): value is ConsoleSession {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<ConsoleSession>;
  return Boolean(
    candidate.user?.id &&
      candidate.access_context &&
      Array.isArray(candidate.access_context.permissions) &&
      Array.isArray(candidate.access_context.scopes) &&
      candidate.access_context.permissions.every((permission) => typeof permission === "string") &&
      candidate.access_context.scopes.every((scope) => scope?.kind === "platform") &&
      typeof candidate.access_context.verified_at === "string" &&
      typeof candidate.expires_at === "string",
  );
}

export async function logoutConsoleSession(): Promise<void> {
  const response = await fetch("/api/v1/session/logout", { method: "POST", credentials: "same-origin" });
  if (!response.ok) throw new Error("Console logout failed");
}

export function consoleLoginHref(): string {
  const returnTo = window.location.pathname + window.location.search + window.location.hash;
  return "/api/v1/auth/login?return_to=" + encodeURIComponent(returnTo);
}
