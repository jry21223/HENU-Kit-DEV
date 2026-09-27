/**
 * 跳转目标（如登录页的 `next`）来自地址栏，谁都能改：只接受本站路径，否则返回 null，
 * 由调用方换成默认页。只看是否以 / 开头不够——//evil.example、/\evil.example、
 * /<制表符>/evil.example 都以 / 开头，浏览器和路由却会把它们解析到另一个站点。所以先按
 * 当前页面的 origin 解析，要求落在同一个 origin，再只交出路径、查询和片段。
 */
export function sameOriginPath(target: string | null | undefined, origin: string): string | null {
  if (!target?.startsWith("/")) return null;
  let url: URL;
  try {
    url = new URL(target, origin);
  } catch {
    return null;
  }
  if (url.origin !== origin) return null;
  const path = `${url.pathname}${url.search}${url.hash}`;
  // 解析后的路径仍可能以 // 开头（/..//evil.example），再交给路由又成了另一个站点。
  return path.startsWith("//") ? null : path;
}
