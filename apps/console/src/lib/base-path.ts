/** Resolve Vite `base` for subpath deploys such as `/console/`. */
export function consoleBasePath(): string {
  const base = import.meta.env.BASE_URL || "/";
  return base.endsWith("/") ? base.slice(0, -1) : base;
}

/** Join an app-relative path with the configured Vite base. */
export function consolePath(path: string): string {
  const base = consoleBasePath();
  const normalized = path.startsWith("/") ? path : `/${path}`;
  return `${base}${normalized}` || "/";
}

/** True when the current location matches an app-relative Console route. */
export function isConsolePath(path: string): boolean {
  const current = window.location.pathname.replace(/\/+$/, "") || "/";
  const target = consolePath(path).replace(/\/+$/, "") || "/";
  return current === target;
}
