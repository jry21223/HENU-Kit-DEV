/**
 * Redirects to the login page, preserving the current location as the
 * `next` return target so a successful login comes back to this surface.
 */
export function redirectToLogin(next?: string): void {
  const target = next ?? `${window.location.pathname}${window.location.search}`;
  window.location.assign(`/account/login?next=${encodeURIComponent(target)}`);
}
