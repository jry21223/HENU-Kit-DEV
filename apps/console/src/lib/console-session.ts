// This generation only isolates browser requests. Authorization remains a fresh
// Gateway/Core decision; no permissions or identity are cached here.
let principalGeneration = 0;
const responseGenerations = new WeakMap<Response, number>();
const expiryListeners = new Set<(generation: number) => void>();

export function beginConsolePrincipal(): number {
  return ++principalGeneration;
}

export function isCurrentConsolePrincipal(generation: number): boolean {
  return generation === principalGeneration;
}

export function onConsoleSessionExpired(listener: (generation: number) => void): () => void {
  expiryListeners.add(listener);
  return () => expiryListeners.delete(listener);
}

function assertCurrentPrincipal(generation: number): void {
  if (!isCurrentConsolePrincipal(generation)) throw new Error("Console principal changed during request");
}

export async function consoleRequest(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const generation = principalGeneration;
  const response = await fetch(input, init);
  assertCurrentPrincipal(generation);
  responseGenerations.set(response, generation);
  if (response.status === 401) {
    const expiredGeneration = beginConsolePrincipal();
    for (const listener of expiryListeners) listener(expiredGeneration);
  }
  return response;
}

export async function readConsoleJSON(response: Response): Promise<unknown> {
  const generation = responseGenerations.get(response);
  if (generation === undefined) throw new Error("Console response has no principal boundary");
  assertCurrentPrincipal(generation);
  const data: unknown = await response.json();
  assertCurrentPrincipal(generation);
  return data;
}

/** Only existing non-sensitive navigation context crosses the login redirect. */
export function consoleReturnTo(): string {
  const { pathname, search, hash } = window.location;
  if (!pathname.startsWith("/") || pathname.startsWith("//") || /[\\\r\n]/.test(pathname)) return "/";
  const query = new URLSearchParams();
  for (const [key, value] of new URLSearchParams(search)) {
    if (["page", "accounts_page", "sessions_page", "inbox_page", "audit_page"].includes(key) && /^[1-9]\d{0,5}$/.test(value)) query.set(key, value);
    if (["tab", "state", "status", "campus"].includes(key) && /^[a-zA-Z0-9_-]{1,80}$/.test(value)) query.set(key, value);
    if (["selected", "ticket_id", "user_id"].includes(key) && /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(value)) query.set(key, value);
  }
  const safeHash = /^#[a-z][a-z0-9-]{0,79}$/i.test(hash) ? hash : "";
  return pathname + (query.size ? `?${query}` : "") + safeHash;
}
