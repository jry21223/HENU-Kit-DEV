/** Fixed Henan University student mailbox domain. */
export const HENU_EMAIL_DOMAIN = "henu.edu.cn";
export const HENU_EMAIL_SUFFIX = `@${HENU_EMAIL_DOMAIN}`;

const LOCAL_RE = /^[a-z0-9][a-z0-9._+-]{0,63}$/i;

/**
 * Normalize user input to local-part only.
 * Pasting `name@henu.edu.cn` or other domains keeps only the left side.
 */
export function toHenuLocalPart(raw: string): string {
  const trimmed = raw.trim().toLowerCase();
  if (!trimmed) return "";
  const at = trimmed.indexOf("@");
  const local = (at >= 0 ? trimmed.slice(0, at) : trimmed)
    // disallow spaces / full-width junk
    .replace(/[^a-z0-9._+-]/gi, "");
  return local.slice(0, 64);
}

export function isValidHenuLocalPart(local: string): boolean {
  return LOCAL_RE.test(local);
}

/** Build full mailbox used by Platform Core. */
export function toHenuEmail(local: string): string {
  const part = toHenuLocalPart(local);
  return part ? `${part}${HENU_EMAIL_SUFFIX}` : "";
}
