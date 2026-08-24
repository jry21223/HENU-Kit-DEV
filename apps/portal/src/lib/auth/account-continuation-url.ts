const CONTINUATION_PARAM = "continuation";

export function continuationHandleFromURL(rawURL: string): string {
  const url = new URL(rawURL);
  const queryHandle = url.searchParams.get(CONTINUATION_PARAM)?.trim();
  if (queryHandle) return queryHandle;
  if (!url.hash) return "";
  return (
    new URLSearchParams(url.hash.slice(1))
      .get(CONTINUATION_PARAM)
      ?.trim() ?? ""
  );
}

export function accountCenterContinuationRedirectURL(
  rawURL: string
): string | null {
  const url = new URL(rawURL);
  const handle = url.searchParams.get(CONTINUATION_PARAM)?.trim();
  if (!handle) return null;
  url.searchParams.delete(CONTINUATION_PARAM);
  const fragment = new URLSearchParams(url.hash.slice(1));
  fragment.set(CONTINUATION_PARAM, handle);
  url.hash = fragment.toString();
  return url.toString();
}

export function accountCenterURLWithoutContinuation(rawURL: string): string {
  const url = new URL(rawURL);
  url.searchParams.delete(CONTINUATION_PARAM);
  if (url.hash) {
    const fragment = new URLSearchParams(url.hash.slice(1));
    if (fragment.has(CONTINUATION_PARAM)) {
      fragment.delete(CONTINUATION_PARAM);
      url.hash = fragment.toString();
    }
  }
  return `${url.pathname}${url.search}${url.hash}`;
}
