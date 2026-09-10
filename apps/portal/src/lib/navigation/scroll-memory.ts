/**
 * 每个路径在本次标签页里最后一次停留的滚动位置（读者的浏览进度）。
 *
 * 键空间与 /food、/library 列表页使用的 use-scroll-restoration 共用：那些页面在
 * 浏览器恢复滚动时还没有渲染出行，需要自己把 offset 补回去；「返回上一级」也读
 * 同一份记录，把读者放回上次离开的位置。
 */
export const SCROLL_MEMORY_PREFIX = "henukit.scroll.v1:";

/**
 * 「返回上一级」的恢复窗口：慢网络下列表页的行可能几秒后才到，所以给得宽一些。
 */
export const SCROLL_RESTORE_WINDOW_MS = 1500;

/**
 * 历史前进/后退的补位窗口：浏览器自己已经恢复过一次，这里只需要盖住首屏渲染。
 */
export const SCROLL_REAPPLY_WINDOW_MS = 500;

/**
 * 把读者放回某个偏移。列表页的行是客户端拉取的，文档要过几帧才够高，所以反复落到
 * 同一位置，直到文档撑得下或窗口结束。返回取消函数。
 *
 * 两个调用方共用这一份：返回上一级的恢复，和列表页在历史前进/后退时的补位——它们
 * 是同一条策略，分开写迟早会漂移。
 */
export function reapplyScrollOffset(target: number, windowMs: number): () => void {
  let frame = 0;
  let cancelled = false;
  const deadline = performance.now() + windowMs;

  const apply = () => {
    if (cancelled) return;
    window.scrollTo(0, target);
    const roomy = document.documentElement.scrollHeight - window.innerHeight >= target;
    if (!roomy && performance.now() < deadline) {
      frame = window.requestAnimationFrame(apply);
    }
  };

  frame = window.requestAnimationFrame(apply);
  return () => {
    cancelled = true;
    if (frame) window.cancelAnimationFrame(frame);
  };
}

/**
 * 请求只在「读者刚刚点了一次向上导航」到「落点页面挂载」之间有效。点击被取消
 * （新标签页打开、被其他处理函数拦下）时不能留下一个过期请求，所以带时间戳。
 */
const REQUEST_TTL_MS = 5000;

export function scrollMemoryKey(pathname: string): string {
  return SCROLL_MEMORY_PREFIX + pathname;
}

export function readScrollOffset(pathname: string): number {
  try {
    const stored = Number(window.sessionStorage.getItem(scrollMemoryKey(pathname)) ?? 0);
    if (!Number.isFinite(stored) || stored <= 0) return 0;
    return stored;
  } catch {
    // 存储被禁用时只损失滚动恢复，不影响导航本身。
    return 0;
  }
}

export function writeScrollOffset(pathname: string, offset: number): void {
  try {
    window.sessionStorage.setItem(scrollMemoryKey(pathname), String(Math.round(offset)));
  } catch {
    // 见 readScrollOffset。
  }
}

/**
 * destination 是不是 pathname 的上一层（含跨级回到模块首页、回到平台首页）。
 * 只有向上导航才恢复位置：横向切换标签、进入更深的页面都应当从顶部开始。
 */
export function isAncestorPath(destination: string, pathname: string): boolean {
  if (destination === pathname) return false;
  if (destination === "/") return pathname !== "/";
  return pathname.startsWith(destination.endsWith("/") ? destination : `${destination}/`);
}

type RestoreRequest = { pathname: string; requestedAt: number };

let restoreRequest: RestoreRequest | null = null;

export function requestScrollRestore(pathname: string): void {
  restoreRequest = { pathname, requestedAt: Date.now() };
}

/**
 * 落点页面是否该恢复位置。读取不清除：React 严格模式下 effect 会跑两遍，
 * 清除会让第二次丢掉请求。过期请求由 clearStaleScrollRestore 收走。
 */
export function scrollRestoreRequested(pathname: string): boolean {
  if (!restoreRequest) return false;
  if (restoreRequest.pathname !== pathname) return false;
  return Date.now() - restoreRequest.requestedAt <= REQUEST_TTL_MS;
}

/** 读者没有落到请求的那个路径（点击被取消、跳去了别处）时丢掉请求。 */
export function clearStaleScrollRestore(pathname: string): void {
  if (!restoreRequest) return;
  const expired = Date.now() - restoreRequest.requestedAt > REQUEST_TTL_MS;
  if (expired || restoreRequest.pathname !== pathname) restoreRequest = null;
}
