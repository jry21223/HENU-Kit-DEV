/**
 * 每个路径在本次标签页里最后一次停留的滚动位置（读者的浏览进度）。
 *
 * 键空间与 /food、/library 列表页使用的 use-scroll-restoration 共用：那些页面在
 * 浏览器恢复滚动时还没有渲染出行，需要自己把 offset 补回去；「返回上一级」也读
 * 同一份记录，把读者放回上次离开的位置。
 */
const SCROLL_MEMORY_PREFIX = "henukit.scroll.v1:";

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
 * destination 是不是 pathname 的上层（前缀祖先：上一层、跨级回模块首页或平台首页）。
 * 只有向上导航才恢复位置：横向切换标签、进入更深的页面都应当从顶部开始。
 */
export function isAncestorPath(destination: string, pathname: string): boolean {
  if (destination === pathname) return false;
  if (destination === "/") return pathname !== "/";
  return pathname.startsWith(destination.endsWith("/") ? destination : `${destination}/`);
}

/**
 * 浏览器历史回退落到的那个路径。按落点记录、用掉即清：只用一个全局布尔量的话，
 * 一次回退到别处（首页、题库……）会把标记一直挂着，读者之后**点**进列表页也会被
 * 拽回旧位置——那就成了「普通进入也恢复」。
 */
let historyReturnPath: string | null = null;

export function rememberHistoryReturn(pathname: string): void {
  historyReturnPath = pathname;
}

/** 这条路径是否刚被历史回退落到，因而应当恢复位置。用掉即清。 */
export function claimHistoryReturn(pathname: string): boolean {
  if (historyReturnPath !== pathname) return false;
  historyReturnPath = null;
  return true;
}

/**
 * 落地后该怎么处理滚动：`restore` 放回读者上次离开的位置（向上回退），
 * `top` 从顶部开始（横跳标签、进入更深的页面）。
 */
export type ScrollLanding = "restore" | "top";

type LandingRequest = { pathname: string; landing: ScrollLanding; requestedAt: number };

let landingRequest: LandingRequest | null = null;

/** 向上回退：落点页面把读者放回上次离开的位置。 */
export function requestScrollRestore(pathname: string): void {
  landingRequest = { pathname, landing: "restore", requestedAt: Date.now() };
}

/**
 * 非向上导航：落点页面必须从顶部开始。
 *
 * 这一步不能全指望路由——练习区的横跳走的是过渡动画里的 `router.push`，实测会把
 * 上一页的滚动一并带过去（页面够高时就不再有"从顶部开始"）。契约写在实现决定里，
 * 就由这里保证。
 */
export function requestScrollTop(pathname: string): void {
  landingRequest = { pathname, landing: "top", requestedAt: Date.now() };
}

/**
 * 落点页面该怎么落位。读取不清除：React 严格模式下 effect 会跑两遍，清除会让
 * 第二次丢掉请求。过期请求由 clearStaleScrollRequest 收走。
 */
export function scrollLandingFor(pathname: string): ScrollLanding | null {
  if (!landingRequest) return null;
  if (landingRequest.pathname !== pathname) return null;
  if (Date.now() - landingRequest.requestedAt > REQUEST_TTL_MS) return null;
  return landingRequest.landing;
}

/** 读者没有落到请求的那个路径（点击被取消、跳去了别处）时丢掉请求。 */
export function clearStaleScrollRequest(pathname: string): void {
  if (!landingRequest) return;
  const expired = Date.now() - landingRequest.requestedAt > REQUEST_TTL_MS;
  if (expired || landingRequest.pathname !== pathname) landingRequest = null;
}
