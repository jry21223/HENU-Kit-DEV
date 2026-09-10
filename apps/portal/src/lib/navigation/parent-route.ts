/**
 * 「返回上一级」的落点：模块内的页面回到该模块首页，模块首页回到平台首页。
 *
 * 上一级按路径层级判断，而不是按浏览器历史：读者可能从任何地方进来（首页入口、
 * 搜索结果、外部链接、直接刷新），左上角那个箭头必须总是走到同一层，才算可预期。
 */
export type ParentRoute = {
  href: string;
  /** 箭头后面显示的层级名。 */
  label: string;
  /**
   * 读屏时说的层级名，默认同 `label`。平台首页的可见文案是品牌名：只念品牌没说清
   * 回到哪一层，但也不能丢掉屏幕上的字，否则语音控制念不出这个控件。
   */
  spokenAs?: string;
};

const PLATFORM_HOME: ParentRoute = {
  href: "/",
  label: "henukit",
  spokenAs: "henukit（平台首页）",
};

const RULES: Array<{ matches: (pathname: string) => boolean; parent: ParentRoute }> = [
  {
    matches: (pathname) => pathname.startsWith("/practice/favorites/"),
    // 文件夹页自己的「返回收藏夹概览」用的也是这个词，两层入口对同一层保持同一个叫法。
    parent: { href: "/practice/favorites", label: "收藏夹概览" },
  },
  { matches: (pathname) => pathname.startsWith("/practice/"), parent: { href: "/practice", label: "题库" } },
  { matches: (pathname) => pathname === "/practice", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/library/"), parent: { href: "/library", label: "书库" } },
  { matches: (pathname) => pathname === "/library", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/food/"), parent: { href: "/food", label: "榜单" } },
  { matches: (pathname) => pathname === "/food", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/campus/"), parent: { href: "/campus", label: "市集" } },
  { matches: (pathname) => pathname === "/campus", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/career/"), parent: { href: "/career", label: "求职雷达" } },
  { matches: (pathname) => pathname === "/career", parent: PLATFORM_HOME },
];

export function parentRoute(pathname: string): ParentRoute {
  return RULES.find((rule) => rule.matches(pathname))?.parent ?? PLATFORM_HOME;
}
