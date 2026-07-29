/**
 * Temporary local-only helpers for the legacy credential-form prototype.
 *
 * Account Portfolio fixtures deliberately do not live here: points,
 * memberships, notifications, and tickets are durable owner data and must be
 * read through Portal Gateway.
 */

/** Demonstration verification code, shown only in explicit local mock auth mode. */
export const EMAIL_DEMO_CODE = "427819";

/** Local form-format helper; it neither sends nor records an email code. */
export function sendEmailCode(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}
